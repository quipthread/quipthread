# quipthread/cloud

Private cloud deployment of [Quipthread](https://github.com/quipthread/quipthread) — a self-hostable comment system. This repo contains the cloud-specific additions (Stripe billing, plan enforcement, Turso multi-tenancy) on top of the open-source core.

The public open-source repo lives at [quipthread/quipthread](https://github.com/quipthread/quipthread). Changes to shared code flow through `scripts/sync-public.sh`.

## What's different from the public repo

Cloud-gated code uses `//go:build cloud`:

- `backend/billing.go` — Stripe checkout, customer portal, webhook handling
- `backend/middleware/plan.go` — plan enforcement, comment/site quotas
- `backend/cloud/` — Turso provisioning, per-tenant SQLite
- `backend/cloud_init.go` — tenant store initialization

Self-hosted stubs (`billing_stub.go`, `plan_stub.go`, `cloud_init_stub.go`) keep the public build clean — no billing surface, business-tier defaults for all limits.

## Repo layout

```
/
├── apps/
│   ├── dashboard/       Astro + Preact admin panel
│   └── website/         Astro + Starlight landing page + docs
├── backend/             Go API (chi, SQLite, JWT)
├── embed/               React 19 IIFE comment widget (Vite)
├── deploy/              Dockerfile.fly, fly.toml, Caddyfile, entrypoint
├── scripts/
│   └── sync-public.sh   Sync shared code to quipthread/quipthread
└── packages/
    └── create-quipthread/  CLI scaffolding (pending)
```

## Development

```bash
# Install Node dependencies
bun install

# Terminal 1 — Go backend (proxies /dashboard/* to Astro dev server)
cd backend
cp ../.env.example .env   # fill in JWT_SECRET + auth provider keys
echo "DEV_DASHBOARD_URL=http://localhost:4321" >> .env
go run .

# Terminal 2 — Astro dashboard (HMR)
bun run dev:dashboard
```

Everything is available at `http://localhost:8080`. The Go backend proxies `/dashboard/*` to the Astro dev server.

## Building

```bash
# Build frontend assets (copies to backend/static/)
bun run build:assets

# Self-hosted binary
cd backend && go build -tags=production -o quipthread .

# Cloud binary (includes Stripe + Turso)
cd backend && go build -tags=production,cloud -o quipthread .
```

Verify both tag sets stay clean:

```bash
cd backend
go build .
go build -tags=cloud .
go test ./...
go test -tags=cloud ./...
```

## Deployment (cloud)

Target: `app.quipthread.com` on Fly.io, behind Cloudflare proxy.

```bash
# Build and deploy
fly deploy -f deploy/Dockerfile.fly --build-arg TAGS="production,cloud"

# Config (set via fly secrets or fly.toml [env])
CLOUD_MODE=true
TRUST_PROXY=true
JWT_SECRET=...
DATABASE_URL=...              # Turso master DB
MASTER_DATABASE_URL=...
TURSO_API_TOKEN=...
TURSO_ORGANIZATION=...
STRIPE_SECRET_KEY=...
STRIPE_WEBHOOK_SECRET=...
STRIPE_PRICE_STARTER_MONTHLY=...
# ... remaining Stripe price IDs
```

Website (`quipthread.com`) deploys to Cloudflare Pages from `apps/website/`.

See `.claude/quipthread-ops.md` for the full deployment runbook.

## Syncing to the public repo

```bash
# Dry run (default) — review what would change
./scripts/sync-public.sh

# Live sync
DRY_RUN=false ./scripts/sync-public.sh
```

The script uses `rsync` with exclusion rules to strip cloud-specific files before copying to `../quipthread-public/`. Review the diff before committing.

## Environment reference

See `.env.example` for all supported variables. Cloud-specific keys:

| Variable | Description |
|---|---|
| `CLOUD_MODE` | Set `true` to enable billing and plan enforcement |
| `STRIPE_SECRET_KEY` | Stripe secret key |
| `STRIPE_WEBHOOK_SECRET` | Stripe webhook signing secret |
| `STRIPE_PRICE_*` | Six price IDs (3 plans × monthly/yearly) |
| `MASTER_DATABASE_URL` | Cloud account DB (Turso or local SQLite) |
| `TURSO_API_TOKEN` | Platform API management token |
| `TURSO_ORGANIZATION` | Turso organization slug |

## Tests

```bash
bun run test:backend
# or directly:
cd backend && go test ./...
```
