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
RUN cd apps/dashboard && PUBLIC_BUILD_TARGET=selfhosted bun run build

# ── Stage 3: Build embed ──────────────────────────────────────────────────────
FROM deps AS embed-build
COPY embed embed/
RUN cd embed && bun run build

# ── Stage 4: Build Go binary ──────────────────────────────────────────────────
FROM golang:1.26-alpine AS go-build
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
# Copy built assets into static/ so //go:embed picks them up.
COPY --from=dashboard-build /app/apps/dashboard/dist ./static/dashboard/
COPY --from=embed-build /app/embed/dist/embed.iife.js ./static/embed.js
RUN CGO_ENABLED=0 GOOS=linux go build -tags=selfhosted,production -ldflags="-s -w" -o /quipthread .

# ── Stage 5: Runtime ──────────────────────────────────────────────────────────
FROM litestream/litestream:0.3
COPY --from=go-build /quipthread /usr/local/bin/quipthread
COPY deploy/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 8080
ENTRYPOINT ["/entrypoint.sh"]
