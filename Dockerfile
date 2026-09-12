# ── Stage 1: Install workspace dependencies ───────────────────────────────────
FROM oven/bun:1-alpine AS deps
WORKDIR /app
COPY package.json bun.lock ./
COPY apps/dashboard/package.json apps/dashboard/
COPY apps/website/package.json apps/website/
COPY embed/package.json embed/
RUN bun install

# ── Stage 2: Build dashboard ──────────────────────────────────────────────────
FROM deps AS dashboard-build
COPY apps/dashboard apps/dashboard/
COPY embed/src/editor/prose.css embed/src/editor/prose.css
RUN cd apps/dashboard && PUBLIC_BUILD_TARGET=selfhosted bun run build

# ── Stage 3: Build embed ──────────────────────────────────────────────────────
FROM deps AS embed-build
COPY embed embed/
RUN cd embed && bun run build

# ── Stage 4: Package the pinned Atlas CLI ─────────────────────────────────────
FROM alpine:3.24 AS atlas
ARG TARGETARCH
ARG ATLAS_VERSION=v1.3.0
WORKDIR /tmp/atlas
RUN apk add --no-cache ca-certificates curl
COPY backend/migration/atlas_cli.sha256 .
RUN set -eu; \
    case "${TARGETARCH}" in \
      amd64|arm64) ;; \
      *) echo "unsupported Atlas architecture: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    artifact="atlas-linux-${TARGETARCH}-${ATLAS_VERSION}"; \
    checksum="$(awk -v key="linux/${TARGETARCH}/${artifact}" '$2 == key { print $1 }' atlas_cli.sha256)"; \
    test -n "${checksum}"; \
    curl --fail --silent --show-error --location \
      "https://release.ariga.io/atlas/${artifact}" --output atlas; \
    printf '%s  %s\n' "${checksum}" atlas | sha256sum -c -s; \
    chmod 0555 atlas; \
    ./atlas version | grep -F "atlas version ${ATLAS_VERSION}" >/dev/null; \
    mkdir -p /out; \
    cp atlas /out/atlas

# ── Stage 5: Build Go binary ──────────────────────────────────────────────────
FROM golang:1.26-alpine AS go-build
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
# Copy built assets into static/ so //go:embed picks them up.
COPY --from=dashboard-build /app/apps/dashboard/dist ./static/dashboard/
COPY --from=embed-build /app/embed/dist/embed.iife.js ./static/embed.js
RUN CGO_ENABLED=0 GOOS=linux go build -tags=selfhosted,production -ldflags="-s -w" -o /quipthread .

# ── Stage 6: Runtime ──────────────────────────────────────────────────────────
FROM litestream/litestream:0.3@sha256:c5a1e1b01916b3a110f6600820ef176d048d9b9c2411ef0a680de0caa68934a5 AS litestream

FROM alpine:3.24
RUN apk add --no-cache ca-certificates sqlite
COPY --from=litestream /usr/local/bin/litestream /usr/local/bin/litestream
COPY --from=go-build /quipthread /usr/local/bin/quipthread
COPY --from=atlas /out/atlas /usr/local/bin/atlas
COPY deploy/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 8080
ENTRYPOINT ["/entrypoint.sh"]
