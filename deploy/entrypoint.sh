#!/bin/sh
set -e

DB_PATH="${DATABASE_URL:-/data/db.sqlite}"

# Ensure data directory exists.
mkdir -p "$(dirname "${DB_PATH}")"

if [ -n "${LITESTREAM_REPLICA_URL}" ]; then
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

    # Restore from replica if one exists (no-op on first run).
    litestream restore -if-replica-exists "${DB_PATH}" "${LITESTREAM_REPLICA_URL}" || true
    # Hand off to litestream, which starts the app and replicates continuously.
    exec litestream replicate -config "${LITESTREAM_CONFIG}"
fi

exec /usr/local/bin/quipthread
