package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

var ErrSelfHostedRemoteDatabase = errors.New("self-hosted mode requires a local SQLite DATABASE_URL")

// ValidateSelfHostedDatabaseURL rejects remote database schemes before any
// driver is opened. Self-hosted deployments are intentionally SQLite-only.
func ValidateSelfHostedDatabaseURL(databaseURL string) error {
	u, err := url.Parse(strings.TrimSpace(databaseURL))
	if err != nil {
		return ErrSelfHostedRemoteDatabase
	}
	switch strings.ToLower(u.Scheme) {
	case "libsql", "http", "https":
		return ErrSelfHostedRemoteDatabase
	default:
		return nil
	}
}

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
	MasterDatabaseURL                string // MASTER_DATABASE_URL — Turso URL for cloud master DB; empty = local SQLite
	TenantDataDir                    string // TENANT_DATA_DIR — directory for per-tenant SQLite files; default "data/tenants"
	CloudNotificationDeliveryEnabled bool   // CLOUD_NOTIFICATION_DELIVERY_ENABLED — dedicated opt-in; disabled by default

	// Tenant store cache (cloud mode). Optional overrides of the safe defaults
	// (capacity 128, idle TTL 15m); invalid or non-positive values fall back to
	// the defaults enforced by middleware.NewStoreCacheWithLimits.
	TenantStoreCacheCapacity int           // TENANT_STORE_CACHE_CAPACITY — max cached tenant stores (LRU-bounded)
	TenantStoreCacheTTL      time.Duration // TENANT_STORE_CACHE_TTL — idle eviction window, e.g. "20m"

	// Public site resolver (cloud-only, disabled by default). When enabled,
	// GET /api/config and GET /api/comments resolve their tenant from the site
	// registry (siteId query param) instead of session cookies.
	PublicSiteResolverEnabled bool // PUBLIC_SITE_RESOLVER_ENABLED

	// Approval-token HMAC key (cloud mode). Dedicated secret used to derive
	// HMAC-SHA-256 hashes of comment-approval bearer tokens for the cloud
	// control-plane locator table. Cloud startup fails closed when absent or
	// shorter than 32 bytes; self-hosted deployments never read it.
	ApprovalTokenHMACKey string // APPROVAL_TOKEN_HMAC_KEY

	// Turso provisioning (cloud Pro/Business)
	TursoAPIToken     string // TURSO_API_TOKEN — Turso Platform API management token
	TursoOrganization string // TURSO_ORGANIZATION — Turso org slug for DB provisioning
	TursoGroup        string // TURSO_GROUP — database group covered by TURSO_AUTH_TOKEN

	// Cross-origin login indicator cookie
	CookieDomain string // COOKIE_DOMAIN — e.g. ".quipthread.com" in production; empty in self-hosted
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

		MasterDatabaseURL:                os.Getenv("MASTER_DATABASE_URL"),
		TenantDataDir:                    getEnv("TENANT_DATA_DIR", "data/tenants"),
		CloudNotificationDeliveryEnabled: os.Getenv("CLOUD_NOTIFICATION_DELIVERY_ENABLED") == "true",

		TenantStoreCacheCapacity: getEnvPositiveInt("TENANT_STORE_CACHE_CAPACITY"),
		TenantStoreCacheTTL:      getEnvPositiveDuration("TENANT_STORE_CACHE_TTL"),

		PublicSiteResolverEnabled: os.Getenv("PUBLIC_SITE_RESOLVER_ENABLED") == "true",

		ApprovalTokenHMACKey: os.Getenv("APPROVAL_TOKEN_HMAC_KEY"),

		TursoAPIToken:     os.Getenv("TURSO_API_TOKEN"),
		TursoOrganization: os.Getenv("TURSO_ORGANIZATION"),
		TursoGroup:        getEnv("TURSO_GROUP", "default"),

		CookieDomain: os.Getenv("COOKIE_DOMAIN"),
	}
}

// ValidateProductionConfig validates values that must never be guessed or
// silently defaulted in a deployed instance. It is intentionally independent
// of database opening so callers can run it as the first startup check.
func ValidateProductionConfig(cfg *Config) error {
	if cfg == nil {
		return errors.New("configuration is nil")
	}
	if strings.TrimSpace(cfg.JWTSecret) == "" {
		return errors.New("JWT_SECRET is required")
	}
	if !validConfigOrigin(cfg.BaseURL) {
		return fmt.Errorf("BASE_URL must be an absolute http or https origin")
	}
	if len(cfg.AllowedOrigins) == 0 {
		return errors.New("ALLOWED_ORIGINS must contain at least one exact origin")
	}
	for _, origin := range cfg.AllowedOrigins {
		if !validConfigOrigin(origin) {
			return fmt.Errorf("ALLOWED_ORIGINS contains an invalid origin")
		}
	}

	githubConfigured := strings.TrimSpace(cfg.GitHubClientID) != "" || strings.TrimSpace(cfg.GitHubSecret) != ""
	if githubConfigured && (strings.TrimSpace(cfg.GitHubClientID) == "" || strings.TrimSpace(cfg.GitHubSecret) == "") {
		return errors.New("GitHub OAuth requires both GITHUB_CLIENT_ID and GITHUB_CLIENT_SECRET")
	}
	googleConfigured := strings.TrimSpace(cfg.GoogleClientID) != "" || strings.TrimSpace(cfg.GoogleSecret) != ""
	if googleConfigured && (strings.TrimSpace(cfg.GoogleClientID) == "" || strings.TrimSpace(cfg.GoogleSecret) == "") {
		return errors.New("google OAuth requires both GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET")
	}
	if !cfg.EmailAuthEnabled && !githubConfigured && !googleConfigured {
		return errors.New("at least one authentication provider must be configured")
	}
	if cfg.EmailAuthEnabled && (strings.TrimSpace(cfg.SMTPHost) == "" || strings.TrimSpace(cfg.SMTPFrom) == "") {
		return errors.New("email authentication requires SMTP_HOST and SMTP_FROM")
	}

	if err := validateRateLimit(cfg.RateLimitComments); err != nil {
		return fmt.Errorf("RATE_LIMIT_COMMENTS: %w", err)
	}
	if err := validateRateLimit(cfg.RateLimitAuth); err != nil {
		return fmt.Errorf("RATE_LIMIT_AUTH: %w", err)
	}
	port, err := strconv.Atoi(strings.TrimSpace(cfg.Port))
	if err != nil || port < 1 || port > 65535 {
		return errors.New("PORT must be an integer between 1 and 65535")
	}
	if !cfg.CloudMode {
		if err := ValidateSelfHostedDatabaseURL(cfg.DatabaseURL); err != nil {
			return err
		}
	}
	return nil
}

// Validate performs the checks needed before any external resource is
// opened. JWT_SECRET remains mandatory in every mode; production binaries
// enable the stricter set through the compile-time production build tag.
func Validate(cfg *Config) error {
	if cfg == nil {
		return errors.New("configuration is nil")
	}
	if strings.TrimSpace(cfg.JWTSecret) == "" {
		return errors.New("JWT_SECRET is required")
	}
	if productionBuild {
		return ValidateProductionConfig(cfg)
	}
	return nil
}

func validConfigOrigin(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return false
	}
	if u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.Hostname() == "" {
		return false
	}
	return true
}

func validateRateLimit(raw string) error {
	parts := strings.SplitN(strings.TrimSpace(raw), "/", 2)
	if len(parts) != 2 {
		return errors.New("must use count/duration format")
	}
	count, err := strconv.Atoi(parts[0])
	if err != nil || count < 1 {
		return errors.New("count must be a positive integer")
	}
	duration, err := time.ParseDuration(parts[1])
	if err != nil || duration <= 0 {
		return errors.New("duration must be positive")
	}
	return nil
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

// getEnvPositiveInt returns the parsed env value when it is a positive integer,
// and 0 otherwise (0 signals "use the component's safe default").
func getEnvPositiveInt(key string) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return n
	}
	return 0
}

// getEnvPositiveDuration returns the parsed duration when it is positive,
// and 0 otherwise (0 signals "use the component's safe default").
func getEnvPositiveDuration(key string) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil && d > 0 {
		return d
	}
	return 0
}
