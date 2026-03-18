#!/bin/sh
set -e

DB_PATH="${DATABASE_URL:-/data/db.sqlite}"

# Ensure data directory exists.
mkdir -p "$(dirname "${DB_PATH}")"

if [ -n "${LITESTREAM_REPLICA_URL}" ]; then
    # Restore from replica if one exists (no-op on first run).
    litestream restore -if-replica-exists "${DB_PATH}" "${LITESTREAM_REPLICA_URL}" || true
    # Hand off to litestream, which starts the app and replicates continuously.
    exec litestream replicate -config /etc/litestream.yml
fi

exec /usr/local/bin/quipthread
