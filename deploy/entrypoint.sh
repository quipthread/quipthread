#!/bin/sh
set -e

DB_PATH="${DATABASE_URL:-/data/db.sqlite}"
DB_WAS_ABSENT=0

quarantine_failed_restore() {
    quarantine_path="${DB_PATH}.integrity-failed-$(date -u +%Y%m%dT%H%M%SZ)-$$"

    if [ -e "${DB_PATH}" ]; then
        if ! mv "${DB_PATH}" "${quarantine_path}"; then
            rm -f "${DB_PATH}"
        fi
    fi
    for suffix in -wal -shm; do
        if [ -e "${DB_PATH}${suffix}" ]; then
            if ! mv "${DB_PATH}${suffix}" "${quarantine_path}${suffix}"; then
                rm -f "${DB_PATH}${suffix}"
            fi
        fi
    done
    printf '%s\n' "Litestream restore integrity check failed; quarantined database at ${quarantine_path}" >&2
}

# Ensure data directory exists.
mkdir -p "$(dirname "${DB_PATH}")"

if [ -n "${LITESTREAM_REPLICA_URL}" ]; then
    if [ ! -e "${DB_PATH}" ]; then
        DB_WAS_ABSENT=1
    fi

    # Generate litestream config from environment variables.
    LITESTREAM_CONFIG="/tmp/litestream.yml"
    cat > "${LITESTREAM_CONFIG}" << EOF
exec: /usr/local/bin/quipthread

dbs:
  - path: ${DB_PATH}
    replicas:
      - url: ${LITESTREAM_REPLICA_URL}
        access-key-id: ${LITESTREAM_ACCESS_KEY_ID:-}
        secret-access-key: ${LITESTREAM_SECRET_ACCESS_KEY:-}
EOF

    # Restore only when the local database does not exist. Litestream treats an
    # absent replica as a successful no-op, but every other restore error must
    # prevent the application from starting with an unverified database.
    litestream restore \
        -config "${LITESTREAM_CONFIG}" \
        -if-db-not-exists \
        -if-replica-exists \
        "${DB_PATH}"
    if [ "${DB_WAS_ABSENT}" -eq 1 ] && [ -e "${DB_PATH}" ]; then
        integrity_status=0
        if integrity_result="$(sqlite3 "${DB_PATH}" 'PRAGMA integrity_check;')"; then
            :
        else
            integrity_status=$?
        fi
        if [ "${integrity_status}" -ne 0 ] || [ "${integrity_result}" != ok ]; then
            quarantine_failed_restore
            exit 1
        fi
    fi
    # Hand off to litestream, which starts the app and replicates continuously.
    exec litestream replicate -config "${LITESTREAM_CONFIG}"
fi

exec /usr/local/bin/quipthread
