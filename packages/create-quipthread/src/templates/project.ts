export interface ProjectConfig {
  baseUrl: string
  allowedOrigins: string
  jwtSecret: string
  githubClientId: string
  githubClientSecret: string
  googleClientId: string
  googleClientSecret: string
  emailAuthEnabled: boolean
  smtpHost: string
  smtpPort: string
  smtpUser: string
  smtpPass: string
  smtpFrom: string
}

export function dockerCompose(baseUrl: string): string {
  const url = new URL(baseUrl)
  const tls = url.protocol === 'https:'
  return `services:
  app:
    image: ghcr.io/quipthread/quipthread:latest
${tls ? '    expose:\n      - "8080"' : `    ports:\n      - "127.0.0.1:${url.port || '80'}:8080"`}
    volumes:
      - ./data:/data
    env_file:
      - .env
    restart: unless-stopped
${
  tls
    ? `
  caddy:
    image: caddy:2-alpine
    ports:
      - "80:80"
      - "443:443"
      - "443:443/udp"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy-data:/data
      - caddy-config:/config
    restart: unless-stopped

volumes:
  caddy-data:
  caddy-config:
`
    : ''
}`
}

export function caddyfile(baseUrl: string): string {
  return `${new URL(baseUrl).host} {\n  reverse_proxy app:8080\n}\n`
}

export function dockerfile(): string {
  return 'FROM ghcr.io/quipthread/quipthread:latest\n'
}

function envValue(value: string): string {
  return `'${value.replaceAll("'", "\\'")}'`
}

export function dotEnv(cfg: ProjectConfig): string {
  const lines: string[] = [
    '# Server',
    'PORT=8080',
    `BASE_URL=${envValue(cfg.baseUrl)}`,
    `ALLOWED_ORIGINS=${envValue(cfg.allowedOrigins)}`,
    '',
    '# Database',
    'DATABASE_URL=/data/db.sqlite',
    '',
    '# Security — do not share or commit this value',
    `JWT_SECRET=${envValue(cfg.jwtSecret)}`,
    '',
  ]

  lines.push('# Auth — GitHub OAuth')
  lines.push('# Register at: https://github.com/settings/developers')
  if (cfg.githubClientId) {
    lines.push(`GITHUB_CLIENT_ID=${envValue(cfg.githubClientId)}`)
    lines.push(`GITHUB_CLIENT_SECRET=${envValue(cfg.githubClientSecret)}`)
  } else {
    lines.push('# GITHUB_CLIENT_ID=')
    lines.push('# GITHUB_CLIENT_SECRET=')
  }
  lines.push('')

  lines.push('# Auth — Google OAuth')
  lines.push('# Register at: https://console.cloud.google.com/apis/credentials')
  if (cfg.googleClientId) {
    lines.push(`GOOGLE_CLIENT_ID=${envValue(cfg.googleClientId)}`)
    lines.push(`GOOGLE_CLIENT_SECRET=${envValue(cfg.googleClientSecret)}`)
  } else {
    lines.push('# GOOGLE_CLIENT_ID=')
    lines.push('# GOOGLE_CLIENT_SECRET=')
  }
  lines.push('')

  lines.push('# Auth — Email / Password')
  lines.push(`EMAIL_AUTH_ENABLED=${cfg.emailAuthEnabled}`)
  lines.push('')

  lines.push('# SMTP — required for email auth verification and notifications')
  if (cfg.smtpHost) {
    lines.push(`SMTP_HOST=${envValue(cfg.smtpHost)}`)
    lines.push(`SMTP_PORT=${envValue(cfg.smtpPort)}`)
    lines.push(`SMTP_USER=${envValue(cfg.smtpUser)}`)
    lines.push(`SMTP_PASS=${envValue(cfg.smtpPass)}`)
    lines.push(`SMTP_FROM=${envValue(cfg.smtpFrom)}`)
  } else {
    lines.push('# SMTP_HOST=')
    lines.push('# SMTP_PORT=587')
    lines.push('# SMTP_USER=')
    lines.push('# SMTP_PASS=')
    lines.push('# SMTP_FROM=')
  }
  lines.push('')

  lines.push('# Notifications (optional — all channels are opt-in)')
  lines.push('# NOTIFY_BATCH_SIZE=5')
  lines.push('# NOTIFY_COOLDOWN_HOURS=24')
  lines.push('# NOTIFY_EMAIL_TO=')
  lines.push('# TELEGRAM_BOT_TOKEN=')
  lines.push('# TELEGRAM_CHAT_ID=')
  lines.push('# SLACK_WEBHOOK_URL=')
  lines.push('# DISCORD_WEBHOOK_URL=')
  lines.push('')

  lines.push('# Spam filtering — Cloudflare Turnstile (optional)')
  lines.push('# TURNSTILE_SITE_KEY=')
  lines.push('# TURNSTILE_SECRET_KEY=')
  lines.push('')

  lines.push('# Rate limiting (defaults shown)')
  lines.push('# RATE_LIMIT_COMMENTS=5/10m')
  lines.push('# RATE_LIMIT_AUTH=10/5m')
  lines.push('')

  lines.push('# Trust forwarded client IPs only behind the generated Caddy proxy')
  lines.push(`TRUST_PROXY=${new URL(cfg.baseUrl).protocol === 'https:'}`)

  return `${lines.join('\n')}\n`
}

export function gitignore(): string {
  return `.env
data/
`
}
