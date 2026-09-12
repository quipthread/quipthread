#!/bin/sh
set -eu

IMAGE="${1:-quipthread-selfhosted-validation}"
WORK_DIR="$(mktemp -d)"
CONTAINER="quipthread-selfhosted-test-$$"
VOLUME="quipthread-selfhosted-volume-$$"
PORT=""
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"

cleanup() {
    docker rm -f "${CONTAINER}" >/dev/null 2>&1 || true
    docker volume rm "${VOLUME}" >/dev/null 2>&1 || true
    rm -rf "${WORK_DIR}"
}
trap cleanup EXIT INT TERM

"${SCRIPT_DIR}/test-entrypoint-restore.sh"
command -v docker >/dev/null
command -v curl >/dev/null
command -v sqlite3 >/dev/null
docker image inspect "${IMAGE}" >/dev/null
litestream_version="$(docker run --rm --entrypoint litestream "${IMAGE}" version 2>&1)"
[ "${litestream_version}" = 'v0.3.14' ]
sqlite_integrity="$(docker run --rm --entrypoint sqlite3 "${IMAGE}" ':memory:' 'PRAGMA integrity_check;' 2>&1)"
[ "${sqlite_integrity}" = 'ok' ]
if restore_help="$(docker run --rm --entrypoint litestream "${IMAGE}" restore --help 2>&1)"; then
    restore_help_status=0
else
    restore_help_status=$?
fi
[ -n "${restore_help}" ]
[ "${restore_help_status}" -ne 0 ]
printf '%s\n' "${restore_help}" | grep -F -- '-config' >/dev/null
printf '%s\n' "${restore_help}" | grep -F -- '-if-db-not-exists' >/dev/null
printf '%s\n' "${restore_help}" | grep -F -- '-if-replica-exists' >/dev/null
if printf '%s\n' "${restore_help}" | grep -F -- '-integrity-check' >/dev/null; then
    echo 'built image exposes unsupported restore integrity flag' >&2
    exit 1
fi
docker volume create "${VOLUME}" >/dev/null

wait_for_health() {
    i=0
    while [ "${i}" -lt 60 ]; do
        if curl --fail --silent "http://127.0.0.1:${PORT}/health" >/dev/null 2>&1; then
            return 0
        fi
        state="$(docker inspect --format '{{.State.Status}}' "${CONTAINER}" 2>/dev/null || true)"
        if [ "${state}" = exited ] || [ "${state}" = dead ]; then
            docker logs "${CONTAINER}" >&2 || true
            return 1
        fi
        i=$((i + 1))
        sleep 1
    done
    docker logs "${CONTAINER}" >&2 || true
    return 1
}

docker run --detach --name "${CONTAINER}" \
    --mount "type=volume,src=${VOLUME},dst=/data" \
    --publish 127.0.0.1::8080 \
    --env JWT_SECRET=selfhosted-container-test-secret \
    --env BASE_URL=https://comments.example.com \
    --env ALLOWED_ORIGINS=https://publisher.example.com \
    --env GITHUB_CLIENT_ID=container-test-client \
    --env GITHUB_CLIENT_SECRET=container-test-secret \
    --env DATABASE_URL=/data/db.sqlite \
    "${IMAGE}" >/dev/null
PORT="$(docker port "${CONTAINER}" 8080/tcp)"
PORT="${PORT##*:}"
wait_for_health
docker stop "${CONTAINER}" >/dev/null
docker cp "${CONTAINER}:/data/db.sqlite" "${WORK_DIR}/first.db"

first_revisions="$(sqlite3 "${WORK_DIR}/first.db" 'SELECT version FROM atlas_schema_revisions ORDER BY version;')"
first_count="$(printf '%s\n' "${first_revisions}" | awk 'NF { count++ } END { print count + 0 }')"
[ "${first_count}" = 10 ]
generation_columns="$(sqlite3 "${WORK_DIR}/first.db" "SELECT name FROM pragma_table_info('users') WHERE name IN ('dashboard_session_generation', 'embed_session_generation') ORDER BY name;")"
[ "${generation_columns}" = "dashboard_session_generation
embed_session_generation" ]

docker start "${CONTAINER}" >/dev/null
PORT="$(docker port "${CONTAINER}" 8080/tcp)"
PORT="${PORT##*:}"
wait_for_health
docker stop "${CONTAINER}" >/dev/null
docker cp "${CONTAINER}:/data/db.sqlite" "${WORK_DIR}/second.db"
second_revisions="$(sqlite3 "${WORK_DIR}/second.db" 'SELECT version FROM atlas_schema_revisions ORDER BY version;')"
[ "${second_revisions}" = "${first_revisions}" ]

docker rm "${CONTAINER}" >/dev/null

set +e
invalid_config="$(docker run --rm \
    --env JWT_SECRET=selfhosted-container-test-secret \
    --env BASE_URL=https://comments.example.com \
    --env DATABASE_URL=/data/db.sqlite \
    "${IMAGE}" 2>&1)"
invalid_status=$?
set -e
[ "${invalid_status}" -ne 0 ]
printf '%s\n' "${invalid_config}" | grep -F 'ALLOWED_ORIGINS must contain at least one exact origin' >/dev/null
if printf '%s\n' "${invalid_config}" | grep -E 'migrate database|open database|atlas' >/dev/null; then
    echo 'invalid production configuration reached migration or database opening' >&2
    exit 1
fi

set +e
rejection="$(docker run --rm \
    --env JWT_SECRET=selfhosted-container-test-secret \
    --env BASE_URL=https://comments.example.com \
    --env ALLOWED_ORIGINS=https://publisher.example.com \
    --env GITHUB_CLIENT_ID=container-test-client \
    --env GITHUB_CLIENT_SECRET=container-test-secret \
    --env DATABASE_URL=libsql://remote.example.test \
    "${IMAGE}" 2>&1)"
status=$?
set -e
[ "${status}" -ne 0 ]
printf '%s\n' "${rejection}" | grep -F 'self-hosted mode requires a local SQLite DATABASE_URL' >/dev/null
if printf '%s\n' "${rejection}" | grep -E 'open database|migrate database|atlas' >/dev/null; then
    echo 'remote DATABASE_URL reached driver opening' >&2
    exit 1
fi

printf '%s\n' 'self-hosted container startup, restart/no-op migration, and remote URL rejection passed'
