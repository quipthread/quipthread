# Quipthread

A self-hostable comment system you can drop into any website. No third-party tracking, no ads, no lock-in — just comments on your own infrastructure.

**Don't want to manage your own server?** [Quipthread Cloud](https://quipthread.com) is the hosted version — same software, managed for you, with a free tier to get started.

## Features

- **Embed anywhere** — drop a single `<script>` tag into any HTML page
- **Moderation tools** — approve/reject queue, bulk actions, inline reply, spam blocklist
- **Multiple auth providers** — GitHub OAuth, Google OAuth, and email/password
- **Email notifications** — moderation digests via SMTP (including AWS SES)
- **Export your data** — full JSON and CSV export with date/status filters
- **14 themes** — or inherit your site's light/dark preference automatically
- **Import from other platforms** — Disqus, WordPress, Remark42
- **Single binary** — Go backend embeds the dashboard and widget; no separate server needed

## Quick Start (Docker)

Clone the repository so the Compose file and Caddy configuration are both available:

```bash
git clone https://github.com/quipthread/quipthread
cd quipthread
cp .env.docker.example .env
chmod 600 .env
```

Edit `.env`: set `JWT_SECRET`, `BASE_URL`, `ALLOWED_ORIGINS`, and a complete authentication provider. Use the Quipthread origin for `BASE_URL` and the websites that embed comments for `ALLOWED_ORIGINS`. Email authentication requires SMTP.

Replace the example domain and email in `deploy/Caddyfile`. Point DNS to your server and allow inbound TCP ports 80 and 443. Then run:

```bash
docker compose pull
docker compose up -d
```

Open `https://your-comments-domain/login` and create the first admin account. The app port is private by default; Caddy serves HTTPS.

The image tag `latest` follows public version-tag releases, not merges to `main`. See the [Docker guide](https://quipthread.com/docs/guides/docker/) for the current-source build option, local testing, and backups.

## Configuration

Copy `.env.docker.example` to `.env` and fill in the values. Required fields are marked below.

### Core

| Variable | Required | Default | Description |
|---|---|---|---|
| `JWT_SECRET` | Yes | — | Random secret for signing session tokens. Generate with `openssl rand -base64 32` |
| `BASE_URL` | Yes | `http://localhost:8080` | Public URL of your instance |
| `PORT` | No | `8080` | Port the server listens on |
| `DATABASE_URL` | No | `./data/comments.db` | SQLite database path |
| `ALLOWED_ORIGINS` | Yes in production | — | Comma-separated exact publisher origins, including scheme and port where needed. No paths or wildcards. |

### Authentication

At least one auth provider is required for your users to log in.

| Variable | Description |
|---|---|
| `GITHUB_CLIENT_ID` / `GITHUB_CLIENT_SECRET` | GitHub OAuth app credentials |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | Google OAuth app credentials |
| `EMAIL_AUTH_ENABLED` | Set `true` to enable email + password login |

**GitHub OAuth setup:**
1. Go to GitHub → Settings → Developer settings → OAuth Apps → New OAuth App
2. Homepage URL: your `BASE_URL`
3. Authorization callback URL: `{BASE_URL}/auth/github/callback`

**Google OAuth setup:**
1. Go to [Google Cloud Console](https://console.cloud.google.com) → APIs & Services → Credentials → Create OAuth client
2. Authorized redirect URI: `{BASE_URL}/auth/google/callback`

### Email (optional)

Used for moderation digest notifications and email auth flows.

| Variable | Description |
|---|---|
| `SMTP_HOST` | SMTP server hostname (e.g. `email-smtp.us-east-1.amazonaws.com` for SES) |
| `SMTP_PORT` | SMTP port (typically `587`) |
| `SMTP_FROM` | Sender address (e.g. `noreply@yourdomain.com`) |
| `SMTP_USER` / `SMTP_PASS` | SMTP credentials |

### Rate Limiting

| Variable | Default | Description |
|---|---|---|
| `RATE_LIMIT_COMMENTS` | `5/10m` | Max comment submissions per IP per window |
| `RATE_LIMIT_AUTH` | `10/5m` | Max auth attempts per IP per window |
| `TRUST_PROXY` | `false` | Set `true` if behind a reverse proxy (enables `X-Forwarded-For` for real IP) |

### Notifications

Self-hosted builds support SMTP email notifications. Configure the `SMTP_*` settings above.

| Variable | Description |
|---|---|
| `NOTIFY_EMAIL_TO` | Email address to receive moderation digests |
| `NOTIFY_BATCH_SIZE` | Send digest when this many comments are pending (default: `5`) |
| `NOTIFY_COOLDOWN_HOURS` | Also send if any pending and this many hours have passed (default: `24`) |

## Embedding the Widget

After creating a site in the dashboard, put the mount element before the script:

```html
<div
  id="comments"
  data-site-id="YOUR_SITE_ID"
  data-page-id="/your-page"
  data-theme="auto"
></div>
<script src="https://your-comments-domain/embed.js" async></script>
```

Use a stable, unique `data-page-id` for each page. The attributes belong on the `div`, not the script. `auto` uses the site theme with a system-preference fallback. See the [embed reference](https://quipthread.com/docs/embed/reference/) for supported options.

## Upgrading

```bash
docker compose pull
docker compose up -d
```

Back up the database before upgrading. Startup runs migrations automatically and stops on migration errors. Container replacement preserves the data volume; `docker compose down -v` deletes it. Keep a pre-upgrade backup if you need to roll back.

## Development

Use Go 1.26.4 or newer and Bun. The backend also requires the pinned Linux Atlas CLI; follow the [source-build guide](https://quipthread.com/docs/guides/self-hosting/) to install it before starting the backend. On macOS or Windows, use the Linux Docker image for the backend.

```bash
bun install --frozen-lockfile
bun run build:assets:selfhosted
cp .env.docker.example .env
# Edit .env with your local backend origin, publisher origins, database path, and auth settings.
```

Run the backend from `backend/` with `go run .`. To use dashboard HMR, set `DEV_DASHBOARD_URL` to the URL printed by `bun run dev:dashboard` before starting the backend.

## Repo Layout

```
/
├── apps/
│   ├── dashboard/       Astro + Preact admin panel
│   └── website/         Astro + Starlight landing page + docs
├── backend/             Go API (chi router, SQLite, JWT sessions)
├── embed/               React 19 IIFE comment widget (Vite)
└── deploy/              Docker entrypoint and supporting scripts
```

## Building from source

The [self-hosting guide](https://quipthread.com/docs/guides/self-hosting/) covers the Go/Bun build, checksum-verified Atlas installation, production configuration, and HTTPS setup. The [Docker guide](https://quipthread.com/docs/guides/docker/) builds all of these inside a container.

## License

[AGPL-3.0](LICENSE)
