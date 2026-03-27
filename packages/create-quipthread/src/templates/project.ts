export interface ProjectConfig {
  baseUrl: string
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

export function dockerCompose(): string {
  return `services:
  app:
    image: ghcr.io/quipthread/quipthread:latest
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
    env_file:
      - .env
    restart: unless-stopped
`
}

export function dotEnv(cfg: ProjectConfig): string {
  const lines: string[] = [
    '# Server',
    'PORT=8080',
    `BASE_URL=${cfg.baseUrl}`,
    `ALLOWED_ORIGINS=${cfg.baseUrl}`,
    '',
    '# Database',
    'DATABASE_URL=./data/comments.db',
    '',
    '# Security — do not share or commit this value',
    `JWT_SECRET=${cfg.jwtSecret}`,
    '',
  ]

  lines.push('# Auth — GitHub OAuth')
  lines.push('# Register at: https://github.com/settings/developers')
  if (cfg.githubClientId) {
    lines.push(`GITHUB_CLIENT_ID=${cfg.githubClientId}`)
    lines.push(`GITHUB_CLIENT_SECRET=${cfg.githubClientSecret}`)
  } else {
    lines.push('# GITHUB_CLIENT_ID=')
    lines.push('# GITHUB_CLIENT_SECRET=')
  }
  lines.push('')

  lines.push('# Auth — Google OAuth')
  lines.push('# Register at: https://console.cloud.google.com/apis/credentials')
  if (cfg.googleClientId) {
    lines.push(`GOOGLE_CLIENT_ID=${cfg.googleClientId}`)
    lines.push(`GOOGLE_CLIENT_SECRET=${cfg.googleClientSecret}`)
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
    lines.push(`SMTP_HOST=${cfg.smtpHost}`)
    lines.push(`SMTP_PORT=${cfg.smtpPort}`)
    lines.push(`SMTP_USER=${cfg.smtpUser}`)
    lines.push(`SMTP_PASS=${cfg.smtpPass}`)
    lines.push(`SMTP_FROM=${cfg.smtpFrom}`)
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

  lines.push('# Set to true if running behind a reverse proxy (Caddy, nginx, etc.)')
  lines.push('# TRUST_PROXY=true')

  return `${lines.join('\n')}\n`
}

export function gitignore(): string {
  return `.env
data/
`
}
