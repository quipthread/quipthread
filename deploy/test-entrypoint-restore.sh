#!/bin/sh
set -eu

ENTRYPOINT="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/entrypoint.sh"
WORK_DIR="$(mktemp -d)"
MOCK_BIN="${WORK_DIR}/bin"
LOG="${WORK_DIR}/litestream.log"

cleanup() {
    rm -rf "${WORK_DIR}"
}
trap cleanup EXIT INT TERM

mkdir -p "${MOCK_BIN}"
cat > "${MOCK_BIN}/litestream" <<'EOF'
#!/bin/sh
set -eu

printf '%s\n' "$*" >> "${LITESTREAM_TEST_LOG}"
case "$1" in
    restore)
        [ "$#" = 6 ]
        [ "$2" = -config ]
        [ "$3" = /tmp/litestream.yml ]
        [ "$4" = -if-db-not-exists ]
        [ "$5" = -if-replica-exists ]
        [ "$6" = "${DATABASE_URL}" ]
        if [ "${LITESTREAM_TEST_CREATE_DB:-0}" = 1 ]; then
            : > "${DATABASE_URL}"
            if [ "${LITESTREAM_TEST_CREATE_SIDECARS:-0}" = 1 ]; then
                : > "${DATABASE_URL}-wal"
                : > "${DATABASE_URL}-shm"
            fi
        fi
        exit "${LITESTREAM_TEST_RESTORE_STATUS:-0}"
        ;;
    replicate)
        exit 23
        ;;
    *)
        exit 64
        ;;
esac
EOF
chmod 0555 "${MOCK_BIN}/litestream"
cat > "${MOCK_BIN}/sqlite3" <<'EOF'
#!/bin/sh
set -eu

printf 'sqlite3 %s\n' "$*" >> "${LITESTREAM_TEST_LOG}"
printf '%s\n' "${LITESTREAM_TEST_INTEGRITY_RESULT:-ok}"
EOF
chmod 0555 "${MOCK_BIN}/sqlite3"

DB_PATH="${WORK_DIR}/db.sqlite"

run_entrypoint() {
    set +e
    restore_status="${1}"
    create_db="${2}"
    integrity_result="${3}"
    existing_db="${4}"
    reset_db="${5}"
    create_sidecars="${6}"
    if [ "${reset_db}" = 1 ]; then
        rm -f "${DB_PATH}" "${DB_PATH}-wal" "${DB_PATH}-shm"
    fi
    if [ "${existing_db}" = 1 ]; then
        : > "${DB_PATH}"
    fi
    PATH="${MOCK_BIN}:${PATH}" \
        DATABASE_URL="${DB_PATH}" \
        LITESTREAM_REPLICA_URL=s3://example.invalid/quipthread/db.sqlite \
        LITESTREAM_TEST_LOG="${LOG}" \
        LITESTREAM_TEST_RESTORE_STATUS="${restore_status}" \
        LITESTREAM_TEST_CREATE_DB="${create_db}" \
        LITESTREAM_TEST_CREATE_SIDECARS="${create_sidecars}" \
        LITESTREAM_TEST_INTEGRITY_RESULT="${integrity_result}" \
        sh "${ENTRYPOINT}" >"${WORK_DIR}/entrypoint.log" 2>&1
    status=$?
    set -e
    return "${status}"
}

# A successful restore command represents Litestream's no-replica,
# -if-replica-exists bootstrap path. Replication must still be started.
if run_entrypoint 0 0 ok 0 1 0; then
    echo 'entrypoint unexpectedly exited successfully' >&2
    exit 1
else
    status=$?
fi
[ "${status}" = 23 ]
grep -F -- 'restore -config /tmp/litestream.yml -if-db-not-exists -if-replica-exists ' "${LOG}" >/dev/null
grep -F -- 'replicate -config ' "${LOG}" >/dev/null
if grep -F -- 'sqlite3 ' "${LOG}" >/dev/null; then
    echo 'entrypoint ran integrity check for a successful no-replica bootstrap' >&2
    exit 1
fi

: > "${LOG}"
# A restore failure must be returned by the entrypoint, before replicate runs.
if run_entrypoint 42 0 ok 0 1 0; then
    echo 'entrypoint unexpectedly continued after restore failure' >&2
    exit 1
else
    status=$?
fi
[ "${status}" = 42 ]
grep -F -- 'restore -config /tmp/litestream.yml -if-db-not-exists -if-replica-exists ' "${LOG}" >/dev/null
if grep -F -- 'replicate ' "${LOG}" >/dev/null; then
    echo 'entrypoint started replication after restore failure' >&2
    exit 1
fi
if grep -F -- 'sqlite3 ' "${LOG}" >/dev/null; then
    echo 'entrypoint ran integrity check after restore failure' >&2
    exit 1
fi

: > "${LOG}"
# A restored database must pass an actual integrity check before replication.
if run_entrypoint 0 1 ok 0 1 0; then
    echo 'entrypoint unexpectedly exited successfully' >&2
    exit 1
else
    status=$?
fi
[ "${status}" = 23 ]
grep -F -- "sqlite3 ${DB_PATH} PRAGMA integrity_check;" "${LOG}" >/dev/null
grep -F -- 'replicate -config ' "${LOG}" >/dev/null

: > "${LOG}"
# A non-ok integrity result must abort startup before replication.
if run_entrypoint 0 1 'not ok' 0 1 1; then
    echo 'entrypoint continued after integrity failure' >&2
    exit 1
else
    status=$?
fi
[ "${status}" = 1 ]
grep -F -- "sqlite3 ${DB_PATH} PRAGMA integrity_check;" "${LOG}" >/dev/null
if grep -F -- 'replicate ' "${LOG}" >/dev/null; then
    echo 'entrypoint started replication after integrity failure' >&2
    exit 1
fi
[ ! -e "${DB_PATH}" ]
[ ! -e "${DB_PATH}-wal" ]
[ ! -e "${DB_PATH}-shm" ]
FAILED_RESTORE=""
for candidate in "${DB_PATH}".integrity-failed-*; do
    case "${candidate}" in
        *-wal|*-shm) continue ;;
    esac
    if [ -e "${candidate}" ]; then
        FAILED_RESTORE="${candidate}"
        break
    fi
done
[ -n "${FAILED_RESTORE}" ]
[ -e "${FAILED_RESTORE}-wal" ]
[ -e "${FAILED_RESTORE}-shm" ]

: > "${LOG}"
# A restart using the same path must recover again after the failed restore.
if run_entrypoint 0 1 ok 0 0 0; then
    echo 'entrypoint unexpectedly exited successfully' >&2
    exit 1
else
    status=$?
fi
[ "${status}" = 23 ]
grep -F -- "sqlite3 ${DB_PATH} PRAGMA integrity_check;" "${LOG}" >/dev/null
grep -F -- 'replicate -config ' "${LOG}" >/dev/null

: > "${LOG}"
# An existing database must not be integrity-checked by the restore bootstrap.
if run_entrypoint 0 0 ok 1 1 0; then
    echo 'entrypoint unexpectedly exited successfully' >&2
    exit 1
else
    status=$?
fi
[ "${status}" = 23 ]
if grep -F -- 'sqlite3 ' "${LOG}" >/dev/null; then
    echo 'entrypoint ran integrity check for an existing database' >&2
    exit 1
fi

printf '%s\n' 'entrypoint restore, integrity, no-replica, and failure behavior passed'
