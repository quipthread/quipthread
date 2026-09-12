# Self-hosted SQLite restore

This procedure restores the SQLite database from the Litestream replica used by
the self-hosted Docker deployment. It deliberately restores to a separate path
first. Do not point a restore at the live database until the validation below
has passed.

## Preconditions

- Confirm the outage and record the current image and deployment configuration.
- Have the real `LITESTREAM_REPLICA_URL`,
  `LITESTREAM_ACCESS_KEY_ID`, and `LITESTREAM_SECRET_ACCESS_KEY` available from
  the secret store. Never use example credentials or a made-up bucket as
  restore evidence.
- Confirm the app image was built from the versioned Dockerfile using
  `litestream/litestream:0.3@sha256:c5a1e1b01916b3a110f6600820ef176d048d9b9c2411ef0a680de0caa68934a5`.
  Confirm the image's `litestream version` output is `v0.3.14`; do not use an
  older recovery image.
- Ensure the host has enough free space for the restored database and a copy of
  the existing database.

The commands below use the `app` service from the repository's
`docker-compose.yml`, which mounts the persistent volume at `/data`. Set a
unique target name for this attempt:

```bash
export LIVE_DB="${DATABASE_URL:-/data/db.sqlite}"
export RESTORE_TARGET="/data/db.sqlite.restore-$(date -u +%Y%m%dT%H%M%SZ)"
export LITESTREAM_REPLICA_URL='s3://REAL-BUCKET/quipthread/db.sqlite'
export LITESTREAM_ACCESS_KEY_ID='REAL-ACCESS-KEY'
export LITESTREAM_SECRET_ACCESS_KEY='REAL-SECRET-KEY'
```

Keep the credentials in the environment or secret manager; do not put them in
this document, shell history, or a checked-in `.env` file.

## Stop and dry-run

Stop the application before touching its volume. Leave the reverse proxy up if
you need to serve a maintenance page, but it must not route requests to the
stopped app.

```bash
docker compose stop app
docker compose ps app
```

The Litestream v0.3.14 image does not provide a restore preview that writes no
files. The dry-run portion of this procedure therefore performs only
read-only replica discovery and local preflight checks; it does not claim that
a restore succeeded:

```bash
test -n "${LITESTREAM_REPLICA_URL:?set the real replica URL first}"
test -n "${LITESTREAM_ACCESS_KEY_ID:?set the real access key first}"
test -n "${LITESTREAM_SECRET_ACCESS_KEY:?set the real secret key first}"
case "${RESTORE_TARGET}" in
  /data/db.sqlite.restore-*) ;;
  *) echo 'RESTORE_TARGET must be a separate /data/db.sqlite.restore-* path' >&2; exit 1 ;;
esac
docker compose run --rm --no-deps --entrypoint sh \
  --env LITESTREAM_REPLICA_URL \
  --env LITESTREAM_ACCESS_KEY_ID \
  --env LITESTREAM_SECRET_ACCESS_KEY \
  --env LIVE_DB \
  --env RESTORE_TARGET \
  app -ec '
    set -eu
    case "${RESTORE_TARGET}" in
      /data/db.sqlite.restore-*) ;;
      *) echo "RESTORE_TARGET must be a separate /data/db.sqlite.restore-* path" >&2; exit 1 ;;
    esac
    test ! -e "${RESTORE_TARGET}"
    cat > /tmp/litestream-restore.yml <<EOF
dbs:
  - path: ${LIVE_DB}
    replicas:
      - url: ${LITESTREAM_REPLICA_URL}
        access-key-id: ${LITESTREAM_ACCESS_KEY_ID}
        secret-access-key: ${LITESTREAM_SECRET_ACCESS_KEY}
EOF
    litestream generations -config /tmp/litestream-restore.yml "${LIVE_DB}"
    litestream snapshots -config /tmp/litestream-restore.yml "${LIVE_DB}"
  '
```

Choose the generation and snapshot to recover from based on the output and the
incident timeline. If replica discovery fails, stop here: do not start the app
and do not replace its database.

## Restore and integrity-check a separate target

Restore into the target selected above. `-if-db-not-exists` prevents an
accidental overwrite of the target, and `-if-replica-exists` makes an absent
replica a successful no-op; network, credential, corruption, and all other
restore errors remain fatal. Litestream v0.3.14 reads the replica URL and
credentials from the config file in config mode; the database path is the only
positional argument.

```bash
docker compose run --rm --no-deps --entrypoint sh \
  --env LITESTREAM_REPLICA_URL \
  --env LITESTREAM_ACCESS_KEY_ID \
  --env LITESTREAM_SECRET_ACCESS_KEY \
  --env LIVE_DB \
  --env RESTORE_TARGET \
  app -ec '
    set -eu
cat > /tmp/litestream-restore.yml <<EOF
dbs:
  - path: ${LIVE_DB}
    replicas:
      - url: ${LITESTREAM_REPLICA_URL}
        access-key-id: ${LITESTREAM_ACCESS_KEY_ID}
        secret-access-key: ${LITESTREAM_SECRET_ACCESS_KEY}
EOF
    test ! -e "${RESTORE_TARGET}"
    litestream restore \
      -config /tmp/litestream-restore.yml \
      -if-db-not-exists \
      -if-replica-exists \
      -o "${RESTORE_TARGET}" \
      "${LIVE_DB}"
    test -s "${RESTORE_TARGET}"
    integrity_result="$(sqlite3 "${RESTORE_TARGET}" 'PRAGMA integrity_check;')"
    test "${integrity_result}" = ok
  '
```

The `test ! -e` check runs against the mounted Docker volume before restore, and
`test -s` and the `sqlite3` `PRAGMA integrity_check` run against the restored
target in that same container. The integrity command must return exactly `ok`;
otherwise startup must not proceed. A successful no-replica bootstrap leaves
the target absent and therefore fails the non-empty check, so it is not treated
as a completed restore.

If the entrypoint's integrity check fails for a newly restored live database,
it moves that database and its `-wal`/`-shm` sidecars to a
`<DB_PATH>.integrity-failed-<timestamp>-<pid>` quarantine and exits before
replication or application startup. Preserve that quarantine for investigation;
the live path remains absent so a restart cannot mistake the failed restore for
an existing database. If the separate target check above fails, quarantine that
target and its sidecars before attempting another restore:

```bash
docker compose run --rm --no-deps --entrypoint sh \
  --env RESTORE_TARGET \
  app -ec '
    quarantine="${RESTORE_TARGET}.integrity-failed-$(date -u +%Y%m%dT%H%M%SZ)-$$"
    if [ -e "${RESTORE_TARGET}" ]; then mv "${RESTORE_TARGET}" "${quarantine}"; fi
    for suffix in -wal -shm; do
      if [ -e "${RESTORE_TARGET}${suffix}" ]; then
        mv "${RESTORE_TARGET}${suffix}" "${quarantine}${suffix}"
      fi
    done
  '
```

Inspect the schema and a few known records as appropriate for the incident.
Do not start the application if the integrity check fails or if the target is
empty or not the expected database.

## Deliberate replacement of the live database

Replacement is a separate, explicitly approved operation. Keep the app
stopped, make a rollback copy, and use the same filesystem so the final rename
is atomic:

```bash
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
BACKUP_DB="${LIVE_DB}.pre-restore-${STAMP}"

docker compose run --rm --no-deps --entrypoint sh app -ec '
  cp -p "$1" "$2"
  for suffix in -wal -shm; do
    if [ -e "${1}${suffix}" ]; then
      mv "${1}${suffix}" "${2}${suffix}"
    fi
  done
  mv "$3" "$1"
' sh "${LIVE_DB}" "${BACKUP_DB}" "${RESTORE_TARGET}"
```

The `-wal` and `-shm` files belong to the database that created them. Preserve
the live database's sidecars with its rollback copy as shown; never leave those
old sidecars next to the restored database. If the restored target has sidecars
of its own, inspect them and move or remove them only while the app is stopped.
Do not copy a sidecar from one database to another.

If replacement is not approved, leave the restored file beside the live file
and start the app normally. If replacement is approved, retain the pre-restore
copy until the deployment has been verified and the rollback window has
expired.

## Start and verify

```bash
docker compose up -d app
docker compose logs --since=2m app
curl --fail --silent http://127.0.0.1:8080/health
```

Review application logs, dashboard access, and representative comment reads
before returning traffic. If startup reports a Litestream restore error,
investigate the replica or credentials; do not bypass it with `|| true` and do
not treat a no-replica bootstrap as a successful data restore.
