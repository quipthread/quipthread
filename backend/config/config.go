package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port             string
	GitHubClientID   string
	GitHubSecret     string
	GoogleClientID   string
	GoogleSecret     string
	DatabaseURL      string
	JWTSecret        string
	AllowedOrigins   []string
	EmailAuthEnabled bool
	SMTPHost         string
	SMTPPort         string
	SMTPUser         string
	SMTPPass         string
	SMTPFrom         string
	WebhookURL       string
	BaseURL          string

	// Notification batching
	NotifyBatchSize   int    // NOTIFY_BATCH_SIZE, default 5
	NotifyCooldownHrs int    // NOTIFY_COOLDOWN_HOURS, default 24
	NotifyEmailTo     string // NOTIFY_EMAIL_TO — fallback recipient if owner has no email

	// Notification channels
	TelegramBotToken  string // TELEGRAM_BOT_TOKEN
	TelegramChatID    string // TELEGRAM_CHAT_ID
	SlackWebhookURL   string // SLACK_WEBHOOK_URL
	DiscordWebhookURL string // DISCORD_WEBHOOK_URL
	EmailProvider     string // EMAIL_PROVIDER: resend | postmark | sendgrid | ses
	EmailAPIKey       string // EMAIL_API_KEY

	// Cloudflare Turnstile (optional bot protection)
	TurnstileSiteKey   string // TURNSTILE_SITE_KEY — served to the embed widget
	TurnstileSecretKey string // TURNSTILE_SECRET_KEY — used for server-side verification

	// Rate limiting
	RateLimitComments string // RATE_LIMIT_COMMENTS — e.g. "5/10m" (default)
	RateLimitAuth     string // RATE_LIMIT_AUTH — e.g. "10/5m" (default)
	TrustProxy        bool   // TRUST_PROXY — trust X-Forwarded-For/X-Real-IP headers (set true when behind nginx/Caddy)

	// Spam filtering
	SpamMaxLinks int // SPAM_MAX_LINKS — max hrefs/URLs before auto-reject (default 3)

	// Turso / libSQL
	TursoAuthToken string // TURSO_AUTH_TOKEN — appended to libsql:// DSN when set

	// Cloud / billing
	CloudMode           bool   // CLOUD_MODE — enables Stripe, plan enforcement, Turso provisioning
	StripeSecretKey     string // STRIPE_SECRET_KEY
	StripeWebhookSecret string // STRIPE_WEBHOOK_SECRET
	StripePrices        StripePrices

	// Cloud multi-tenant
	MasterDatabaseURL string // MASTER_DATABASE_URL — Turso URL for cloud master DB; empty = local SQLite
	TenantDataDir     string // TENANT_DATA_DIR — directory for per-tenant SQLite files; default "data/tenants"

	// Turso provisioning (cloud Pro/Business)
	TursoAPIToken     string // TURSO_API_TOKEN — Turso Platform API management token
	TursoOrganization string // TURSO_ORGANIZATION — Turso org slug for DB provisioning
}

type StripePrices struct {
	StarterMonthly    string // STRIPE_PRICE_STARTER_MONTHLY
	StarterYearly     string // STRIPE_PRICE_STARTER_YEARLY
	ProMonthly        string // STRIPE_PRICE_PRO_MONTHLY
	ProYearly         string // STRIPE_PRICE_PRO_YEARLY
	BusinessMonthly   string // STRIPE_PRICE_BUSINESS_MONTHLY
	BusinessYearly    string // STRIPE_PRICE_BUSINESS_YEARLY
	EnterpriseMonthly string // STRIPE_PRICE_ENTERPRISE_MONTHLY
	EnterpriseYearly  string // STRIPE_PRICE_ENTERPRISE_YEARLY
}

func Load() *Config {
	_ = godotenv.Load("../.env", ".env")

	var origins []string
	if raw := os.Getenv("ALLOWED_ORIGINS"); raw != "" {
		for _, o := range strings.Split(raw, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				origins = append(origins, trimmed)
			}
		}
	}

	return &Config{
		Port:             getEnv("PORT", "8080"),
		GitHubClientID:   os.Getenv("GITHUB_CLIENT_ID"),
		GitHubSecret:     os.Getenv("GITHUB_CLIENT_SECRET"),
		GoogleClientID:   os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleSecret:     os.Getenv("GOOGLE_CLIENT_SECRET"),
		DatabaseURL:      getEnv("DATABASE_URL", "./data/comments.db"),
		JWTSecret:        os.Getenv("JWT_SECRET"),
		AllowedOrigins:   origins,
		EmailAuthEnabled: os.Getenv("EMAIL_AUTH_ENABLED") == "true",
		SMTPHost:         os.Getenv("SMTP_HOST"),
		SMTPPort:         getEnv("SMTP_PORT", "587"),
		SMTPUser:         os.Getenv("SMTP_USER"),
		SMTPPass:         os.Getenv("SMTP_PASS"),
		SMTPFrom:         os.Getenv("SMTP_FROM"),
		WebhookURL:       os.Getenv("WEBHOOK_URL"),
		BaseURL:          getEnv("BASE_URL", "http://localhost:8080"),

		NotifyBatchSize:   getEnvInt("NOTIFY_BATCH_SIZE", 5),
		NotifyCooldownHrs: getEnvInt("NOTIFY_COOLDOWN_HOURS", 24),
		NotifyEmailTo:     os.Getenv("NOTIFY_EMAIL_TO"),

		TelegramBotToken:  os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:    os.Getenv("TELEGRAM_CHAT_ID"),
		SlackWebhookURL:   os.Getenv("SLACK_WEBHOOK_URL"),
		DiscordWebhookURL: os.Getenv("DISCORD_WEBHOOK_URL"),
		EmailProvider:     os.Getenv("EMAIL_PROVIDER"),
		EmailAPIKey:       os.Getenv("EMAIL_API_KEY"),

		TurnstileSiteKey:   os.Getenv("TURNSTILE_SITE_KEY"),
		TurnstileSecretKey: os.Getenv("TURNSTILE_SECRET_KEY"),

		RateLimitComments: getEnv("RATE_LIMIT_COMMENTS", "5/10m"),
		RateLimitAuth:     getEnv("RATE_LIMIT_AUTH", "10/5m"),
		TrustProxy:        os.Getenv("TRUST_PROXY") == "true",

		SpamMaxLinks: getEnvInt("SPAM_MAX_LINKS", 3),

		TursoAuthToken: os.Getenv("TURSO_AUTH_TOKEN"),

		CloudMode:           os.Getenv("CLOUD_MODE") == "true",
		StripeSecretKey:     os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		StripePrices: StripePrices{
			StarterMonthly:    os.Getenv("STRIPE_PRICE_STARTER_MONTHLY"),
			StarterYearly:     os.Getenv("STRIPE_PRICE_STARTER_YEARLY"),
			ProMonthly:        os.Getenv("STRIPE_PRICE_PRO_MONTHLY"),
			ProYearly:         os.Getenv("STRIPE_PRICE_PRO_YEARLY"),
			BusinessMonthly:   os.Getenv("STRIPE_PRICE_BUSINESS_MONTHLY"),
			BusinessYearly:    os.Getenv("STRIPE_PRICE_BUSINESS_YEARLY"),
			EnterpriseMonthly: os.Getenv("STRIPE_PRICE_ENTERPRISE_MONTHLY"),
			EnterpriseYearly:  os.Getenv("STRIPE_PRICE_ENTERPRISE_YEARLY"),
		},

		MasterDatabaseURL: os.Getenv("MASTER_DATABASE_URL"),
		TenantDataDir:     getEnv("TENANT_DATA_DIR", "data/tenants"),

		TursoAPIToken:     os.Getenv("TURSO_API_TOKEN"),
		TursoOrganization: os.Getenv("TURSO_ORGANIZATION"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
