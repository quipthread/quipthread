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

The fastest way to get running:

```bash
curl -O https://raw.githubusercontent.com/quipthread/quipthread/main/docker-compose.yml
curl -O https://raw.githubusercontent.com/quipthread/quipthread/main/.env.docker.example

cp .env.docker.example .env
# Edit .env — at minimum set JWT_SECRET and at least one OAuth provider
docker compose up -d
```

The admin dashboard is available at `http://localhost:8080/dashboard`.

## Configuration

Copy `.env.docker.example` to `.env` and fill in the values. Required fields are marked below.

### Core

| Variable | Required | Default | Description |
|---|---|---|---|
| `JWT_SECRET` | Yes | — | Random secret for signing session tokens. Generate with `openssl rand -base64 32` |
| `BASE_URL` | Yes | `http://localhost:8080` | Public URL of your instance |
| `PORT` | No | `8080` | Port the server listens on |
| `DATABASE_URL` | No | `./data/comments.db` | SQLite database path |
| `ALLOWED_ORIGINS` | No | _(allow all)_ | Comma-separated list of domains that can embed the widget |

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
| `EMAIL_PROVIDER` | `smtp`, `ses`, `resend`, `postmark`, or `sendgrid` |
| `SMTP_HOST` | SMTP server hostname (e.g. `email-smtp.us-east-1.amazonaws.com` for SES) |
| `SMTP_PORT` | SMTP port (typically `587`) |
| `SMTP_FROM` | Sender address (e.g. `noreply@yourdomain.com`) |
| `SMTP_USER` / `SMTP_PASS` | SMTP credentials |
| `EMAIL_API_KEY` | API key for Resend, Postmark, or SendGrid |

### Rate Limiting

| Variable | Default | Description |
|---|---|---|
| `RATE_LIMIT_COMMENTS` | `5/10m` | Max comment submissions per IP per window |
| `RATE_LIMIT_AUTH` | `10/5m` | Max auth attempts per IP per window |
| `TRUST_PROXY` | `false` | Set `true` if behind a reverse proxy (enables `X-Forwarded-For` for real IP) |

### Notifications

| Variable | Description |
|---|---|
| `NOTIFY_EMAIL_TO` | Email address to receive moderation digests |
| `NOTIFY_BATCH_SIZE` | Send digest when this many comments are pending (default: `5`) |
| `NOTIFY_COOLDOWN_HOURS` | Also send if any pending and this many hours have passed (default: `24`) |
| `TELEGRAM_BOT_TOKEN` / `TELEGRAM_CHAT_ID` | Telegram notification channel |
| `SLACK_WEBHOOK_URL` | Slack incoming webhook |
| `DISCORD_WEBHOOK_URL` | Discord webhook URL |
| `NOTIFY_WEBHOOK_URL` | Generic HTTP webhook for custom integrations |

## Embedding the Widget

After logging in and creating a site in the dashboard, add this to any page:

```html
<script
  src="http://your-instance.com/embed.js"
  data-site-id="your-site-id"
  async
></script>
```

The widget automatically inherits your page's light/dark preference, or you can pin a specific theme:

```html
<script
  src="http://your-instance.com/embed.js"
  data-site-id="your-site-id"
  data-theme="dark"
  async
></script>
```

Available themes: `auto`, `light`, `dark`, `editorial-light`, `editorial-dark`, `warm-paper`, `midnight`, `ocean`, `forest`, `rose`, `slate`, `sepia`, `high-contrast-light`, `high-contrast-dark`.

## Upgrading

```bash
docker compose pull
docker compose up -d
```

The database schema is managed automatically — migrations run on startup and are idempotent.

## Development

```bash
# Install dependencies
bun install

# Terminal 1 — Go backend
cd backend
cp ../.env.example .env   # fill in JWT_SECRET + auth provider keys
echo "DEV_DASHBOARD_URL=http://localhost:4321" >> .env
go run .

# Terminal 2 — Astro dashboard (with HMR)
bun run dev:dashboard
```

Everything is available at `http://localhost:8080`. The Go backend proxies `/dashboard/*` to the Astro dev server so hot module replacement works.

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

## Building from Source

```bash
# Build frontend assets
bun run build:assets:selfhosted

# Build the Go binary
cd backend && go build -tags=selfhosted,production -o quipthread .

# Run it
./quipthread
```

## License

[AGPL-3.0](LICENSE)
