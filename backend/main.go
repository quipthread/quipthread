package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	neturl "net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/quipthread/quipthread/cloud"
	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/handlers"
	authhandlers "github.com/quipthread/quipthread/handlers/auth"
	"github.com/quipthread/quipthread/middleware"
	"github.com/quipthread/quipthread/models"
	"github.com/quipthread/quipthread/notifications"
)

func main() {
	cfg := config.Load()

	if cfg.JWTSecret == "" {
		log.Fatal("JWT_SECRET environment variable is required")
	}

	store, err := openStore(cfg)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer store.Close() //nolint:errcheck // deferred close on program exit

	// Seed the dev test site used by the / test page.
	if s, _ := store.GetSite("dev-site"); s == nil {
		_ = store.CreateSite(&models.Site{ID: "dev-site", OwnerID: "dev", Domain: "localhost"})
	}

	// Cloud master DB — opened only when CLOUD_MODE=true.
	var cloudStore cloud.Store
	if cfg.CloudMode {
		cs, err := openCloudStore(cfg)
		if err != nil {
			log.Fatalf("open cloud database: %v", err)
		}
		if closer, ok := cs.(interface{ Close() error }); ok {
			defer closer.Close() //nolint:errcheck // deferred close on program exit
		}
		cloudStore = cs
	}

	tenantCache := middleware.NewStoreCache()

	// Notification dispatcher — runs in background, cancelled on shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	notifier := notifications.Build(cfg, store)
	go notifications.StartDispatcher(ctx, store, notifier, cfg)

	authHandler := authhandlers.NewHandlerWithCloud(store, cfg, cloudStore)
	commentsHandler := handlers.NewCommentsHandler(store, cfg)
	adminHandler := handlers.NewAdminHandler(store, cfg)
	configHandler := handlers.NewConfigHandler(cfg, store)
	exportHandler := handlers.NewExportHandler(store, cfg)
	analyticsHandler := handlers.NewAnalyticsHandler(store, cfg)
	modRulesHandler := handlers.NewModRulesHandler(store, cfg)
	accountHandler := handlers.NewAccountHandler(store, cfg)

	// Rate limiters — parse config strings, fall back to defaults on bad input.
	commentsRL := buildRateLimiter(cfg.RateLimitComments, 5, 10*time.Minute)
	authRL := buildRateLimiter(cfg.RateLimitAuth, 10, 5*time.Minute)

	// IP extractor for rate limiting. Only trust proxy headers when explicitly
	// configured — otherwise clients can spoof X-Forwarded-For to bypass limits.
	ipFn := middleware.RemoteAddrIP
	if cfg.TrustProxy {
		ipFn = middleware.RealIP
	}

	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RequestID)
	r.Use(middleware.CORS(cfg.AllowedOrigins))
	if cfg.CloudMode && cloudStore != nil {
		r.Use(middleware.InjectTenantStore(cloudStore, tenantCache, cfg))
	}

	// --- Auth routes (public) -----------------------------------------------
	r.Get("/auth/github/login", authHandler.GithubLogin)
	r.Get("/auth/github/callback", authHandler.GithubCallback)
	r.Get("/auth/google/login", authHandler.GoogleLogin)
	r.Get("/auth/google/callback", authHandler.GoogleCallback)

	// Email auth endpoints are rate-limited (brute-force protection).
	r.With(middleware.RateLimit(authRL, ipFn)).Post("/auth/email/register", authHandler.EmailRegister)
	r.Get("/auth/email/verify/{token}", authHandler.EmailVerify)
	r.With(middleware.RateLimit(authRL, ipFn)).Post("/auth/email/resend-verification", authHandler.EmailResend)
	r.With(middleware.RateLimit(authRL, ipFn)).Post("/auth/email/login", authHandler.EmailLogin)
	r.With(middleware.RateLimit(authRL, ipFn)).Post("/auth/email/forgot", authHandler.EmailForgot)
	r.Get("/auth/email/reset/{token}", authHandler.EmailResetPage)
	r.Post("/auth/email/reset/{token}", authHandler.EmailReset)

	// Cloud email auth routes (cloud mode only)
	r.Get("/auth/email/cloud-verify", authHandler.CloudEmailVerify)
	r.With(middleware.RateLimit(authRL, ipFn)).Post("/auth/email/cloud-reset-request", authHandler.CloudPasswordResetRequest)
	r.Get("/auth/email/cloud-reset-confirm", authHandler.CloudResetPage)
	r.With(middleware.RateLimit(authRL, ipFn)).Post("/auth/email/cloud-reset-confirm", authHandler.CloudPasswordResetConfirm)

	r.Post("/auth/logout", authHandler.Logout)
	r.Get("/api/auth/me", authHandler.Me)

	// Billing routes (public webhook + admin-protected status/checkout/portal).
	handlers.RegisterBillingRoutes(r, store, cfg, cloudStore, tenantCache)

	// --- Public API routes --------------------------------------------------
	r.Get("/api/config", configHandler.PublicConfig)
	r.Get("/api/comments", commentsHandler.List)

	// Approval page (server-rendered, no dashboard login required) — M7
	r.Get("/approve/{token}", handlers.HandleApprovalPage(store))
	r.Post("/approve/{token}", handlers.HandleApprovalAction(store))

	// embed.js — embedded in production builds (go:build production), served
	// from disk in dev builds.
	r.Get("/embed.js", func(w http.ResponseWriter, r *http.Request) {
		if len(embedJSBytes) > 0 {
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			_, _ = w.Write(embedJSBytes)
			return
		}
		http.ServeFile(w, r, "../embed/dist/embed.iife.js")
	})

	// Admin dashboard — in dev with DEV_DASHBOARD_URL set, proxy to the Astro
	// dev server so HMR works. In production (or dev without the env var),
	// serve the embedded/disk-based static build.
	//
	// The Astro dashboard has NO base path configured, so:
	//   - Auth pages (/login, /signup, /forgot-password) are served directly at
	//     those root paths by Astro.
	//   - Dashboard pages live at /comments, /sites, etc. in Astro, but are
	//     exposed to the browser under /dashboard/* (prefix stripped when proxying).
	if devURL := os.Getenv("DEV_DASHBOARD_URL"); devURL != "" {
		devProxy := newDevProxy(devURL)

		// Auth pages: proxy directly — Astro dev server serves them at their
		// natural paths (/login, /signup, /forgot-password).
		for _, p := range []string{"/login", "/signup", "/forgot-password"} {
			r.Handle(p, devProxy)
		}

		// Dashboard pages: strip /dashboard prefix so Astro sees /comments, /sites,
		// etc. — matching the routes defined in src/pages/.
		dashProxy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/dashboard" || r.URL.Path == "/dashboard/" {
				r.URL.Path = "/"
			} else {
				r.URL.Path = strings.TrimPrefix(r.URL.Path, "/dashboard")
			}
			devProxy.ServeHTTP(w, r)
		})
		r.Handle("/dashboard", dashProxy)
		r.Handle("/dashboard/*", dashProxy)

		// Catch-all for Vite internals: /@vite/client, /_astro/*, HMR WebSocket, etc.
		r.NotFound(devProxy.ServeHTTP)
	} else {
		dashFS := http.FS(dashboardSubFS())
		dashFileServer := http.FileServer(dashFS)

		// Auth pages at root-level paths.
		serveStaticPage := func(path string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				f, err := dashFS.Open(path)
				if err != nil {
					http.NotFound(w, r)
					return
				}
				defer f.Close() //nolint:errcheck // deferred close; embedded FS file
				http.ServeContent(w, r, "index.html", time.Time{}, f.(io.ReadSeeker))
			}
		}
		r.Get("/login", serveStaticPage("login/index.html"))
		r.Get("/signup", serveStaticPage("signup/index.html"))
		r.Get("/forgot-password", serveStaticPage("forgot-password/index.html"))

		// Astro assets — without a base path, asset hrefs in HTML are /_astro/...
		// and favicon is at /favicon.svg.
		r.Handle("/_astro/*", dashFileServer)
		r.Get("/favicon.svg", dashFileServer.ServeHTTP)

		// Dashboard pages: strip /dashboard prefix to match the Astro output layout.
		r.Handle("/dashboard", http.RedirectHandler("/dashboard/", http.StatusMovedPermanently))
		r.Handle("/dashboard/*", http.StripPrefix("/dashboard", dashFileServer))
	}

	// Health check — required by Fly.io deployment checks.
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Root redirects to the dashboard.
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard/", http.StatusFound)
	})

	// Dev test page — available at /dev for testing the embed widget.
	r.Get("/dev", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, devTestPage) //nolint:errcheck // ResponseWriter.Write errors are not actionable
	})

	// --- Authenticated commenter routes -------------------------------------
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(cfg.JWTSecret))

		r.With(middleware.RateLimit(commentsRL, ipFn), middleware.EnforceCommentQuota(store, cfg)).Post("/api/comments", commentsHandler.Create)
		r.Delete("/api/comments/{id}", commentsHandler.Delete)
	})

	// --- Admin routes -------------------------------------------------------
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAdmin(cfg.JWTSecret))

		r.Get("/api/admin/comments", adminHandler.ListComments)
		r.Patch("/api/admin/comments/{id}", adminHandler.UpdateComment)
		r.Post("/api/admin/comments/{id}/reply", adminHandler.Reply)
		r.Delete("/api/admin/comments/{id}", adminHandler.DeleteComment)

		r.Get("/api/admin/users", adminHandler.ListUsers)
		r.Patch("/api/admin/users/{id}", adminHandler.UpdateUser)

		r.Get("/api/admin/sites", adminHandler.ListSites)
		r.With(middleware.EnforceSiteLimit(store, cfg)).Post("/api/admin/sites", adminHandler.CreateSite)
		r.Patch("/api/admin/sites/{id}", adminHandler.UpdateSite)

		// Analytics route
		r.Get("/api/admin/analytics", analyticsHandler.Get)

		// Moderation rules routes
		r.Get("/api/admin/modrules/blocklist", modRulesHandler.List)
		r.Post("/api/admin/modrules/blocklist", modRulesHandler.Add)
		r.Delete("/api/admin/modrules/blocklist/{id}", modRulesHandler.Delete)
		r.Post("/api/admin/modrules/blocklist/import", modRulesHandler.Import)

		// Account routes
		r.Get("/api/admin/account", accountHandler.Get)
		r.Patch("/api/admin/account/profile", accountHandler.UpdateProfile)
		r.Patch("/api/admin/account/password", accountHandler.UpdatePassword)
		r.Delete("/api/admin/account/identity/{provider}", accountHandler.DisconnectIdentity)
		r.Get("/api/admin/account/security", accountHandler.GetSecurity)
		r.Patch("/api/admin/account/security", accountHandler.UpdateSecurity)

		// Export route
		r.Get("/api/admin/export", exportHandler.Export)

		// Import routes
		r.Post("/api/admin/import/disqus", adminHandler.ImportDisqus)
		r.Post("/api/admin/import/wordpress", adminHandler.ImportWordPress)
		r.Post("/api/admin/import/remark42", adminHandler.ImportRemark42)
		r.Post("/api/admin/import/native", adminHandler.ImportNative)
		r.Post("/api/admin/import/quipthread", adminHandler.ImportQuipthreadDB)
		r.Post("/api/admin/import/sqlite/inspect", adminHandler.ImportSQLiteInspect)
		r.Post("/api/admin/import/sqlite/run", adminHandler.ImportSQLiteRun)
	})

	addr := fmt.Sprintf(":%s", cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("quipthread listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down…")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

func openStore(cfg *config.Config) (db.Store, error) {
	url := cfg.DatabaseURL
	if strings.HasPrefix(url, "libsql://") || strings.HasPrefix(url, "https://") {
		dsn := url
		if cfg.TursoAuthToken != "" && !strings.Contains(url, "authToken=") {
			sep := "?"
			if strings.Contains(url, "?") {
				sep = "&"
			}
			dsn = url + sep + "authToken=" + neturl.QueryEscape(cfg.TursoAuthToken)
		}
		return db.NewLibSQLStore(dsn)
	}

	// Local SQLite — ensure the data directory exists first.
	if err := os.MkdirAll(filepath.Dir(url), 0750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return db.NewSQLiteStore(url)
}

func buildRateLimiter(spec string, defaultCount int, defaultPeriod time.Duration) middleware.RateLimiter {
	count, period, err := middleware.ParseWindow(spec)
	if err != nil {
		log.Printf("invalid rate limit spec %q, using default: %v", spec, err)
		count, period = defaultCount, defaultPeriod
	}
	return middleware.NewMemoryRateLimiter(count, period)
}

const devTestPage = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Quipthread — Dev Test</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body { font-family: system-ui, sans-serif; transition: background 0.2s, color 0.2s; }
    body.light { background: #ffffff; color: #1A1714; }
    body.dark  { background: #0F0F0F; color: #E8E3DC; }
    .controls {
      display: flex;
      align-items: center;
      gap: 0.5rem;
      padding: 0.75rem 1.5rem;
      border-bottom: 1px solid rgba(128,128,128,0.15);
      position: sticky;
      top: 0;
      background: inherit;
      z-index: 100;
    }
    .controls span { font-size: 0.75rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; opacity: 0.4; margin-right: 0.25rem; }
    .controls button {
      padding: 0.3rem 0.75rem;
      border-radius: 4px;
      border: 1px solid currentColor;
      cursor: pointer;
      font-size: 0.8125rem;
      background: transparent;
      color: inherit;
      opacity: 0.5;
    }
    .controls button.active { background: #E07F32; border-color: #E07F32; color: #fff; opacity: 1; }
    .main { max-width: 760px; margin: 0 auto; padding: 3rem 1.5rem; }
    h1 { font-size: 1.5rem; margin-bottom: 0.25rem; }
    .meta { font-size: 0.875rem; opacity: 0.5; margin-bottom: 3rem; }
  </style>
</head>
<body class="light">
  <div class="controls">
    <span>Theme</span>
    <button id="btn-default" class="active" onclick="setTheme('default')">Admin default</button>
    <button id="btn-light" onclick="setTheme('light')">Light</button>
    <button id="btn-dark" onclick="setTheme('dark')">Dark</button>
    <button id="btn-auto" onclick="setTheme('auto')">Auto</button>
  </div>
  <div class="main">
    <h1>Dev Test Post</h1>
    <p class="meta">This page is only served in local development.</p>

    <div
      id="comments"
      data-site-id="dev-site"
      data-page-id="/dev-test"
      data-page-url="http://localhost:8080/"
      data-page-title="Dev Test Post"
    ></div>
  </div>

  <script>
    function setTheme(theme) {
      const comments = document.getElementById('comments')
      const isDark = theme === 'dark' || (theme === 'auto' && window.matchMedia('(prefers-color-scheme: dark)').matches)
      document.body.className = isDark ? 'dark' : 'light'
      document.querySelectorAll('.controls button').forEach(b => b.classList.remove('active'))
      document.getElementById('btn-' + theme).classList.add('active')
      if (theme === 'default') {
        delete comments.dataset.theme
      } else {
        comments.dataset.theme = theme
      }
    }
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
      if (document.getElementById('btn-auto').classList.contains('active')) setTheme('auto')
    })
  </script>
  <script src="/embed.js"></script>
</body>
</html>`
