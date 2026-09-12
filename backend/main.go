package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
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
	"github.com/quipthread/quipthread/cloud/approvaltoken"
	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/handlers"
	authhandlers "github.com/quipthread/quipthread/handlers/auth"
	"github.com/quipthread/quipthread/middleware"
	"github.com/quipthread/quipthread/migration"
	"github.com/quipthread/quipthread/models"
	"github.com/quipthread/quipthread/notifications"
	"github.com/quipthread/quipthread/session"
)

func main() {
	cfg := config.Load()

	// Structured logging. LOG_FORMAT=json for machine-readable output (production);
	// default is human-readable text for dev. In Go 1.21+, slog.SetDefault also
	// redirects log.Print* calls through slog so existing log.Printf calls are included.
	logLevel := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		logLevel = slog.LevelDebug
	}
	var logHandler slog.Handler
	if os.Getenv("LOG_FORMAT") == "json" {
		logHandler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	} else {
		logHandler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	}
	slog.SetDefault(slog.New(logHandler))
	log.SetFlags(0) // timestamps are handled by slog; avoid duplication

	if err := config.Validate(cfg); err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	if !cfg.CloudMode || !cloudRuntimeBuild {
		if err := config.ValidateSelfHostedDatabaseURL(cfg.DatabaseURL); err != nil {
			log.Fatalf("database configuration: %v", err)
		}
	}
	if err := applyServerMigrations(context.Background(), cfg); err != nil {
		log.Fatalf("migrate database: %v", err)
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

	// Cloud master DB — opened only when CLOUD_MODE=true. Startup fails closed
	// if a cloud build is unavailable or no control-plane store can be opened:
	// running "cloud mode" without the store would skip site-registry dual
	// writes and tenant isolation, so it must never happen silently.
	var cloudStore cloud.Store
	if cfg.CloudMode {
		if err := validateCloudConfig(cfg); err != nil {
			log.Fatalf("cloud mode startup: %v", err)
		}
		cs, err := validateCloudStore(openCloudStore(cfg))
		if err != nil {
			log.Fatalf("cloud mode startup: %v", err)
		}
		if closer, ok := cs.(interface{ Close() error }); ok {
			defer closer.Close() //nolint:errcheck // deferred close on program exit
		}
		cloudStore = cs
	}

	tenantCache := middleware.NewStoreCacheWithLimits(cfg.TenantStoreCacheCapacity, cfg.TenantStoreCacheTTL)

	// Startup config summary — sanitized (no secrets). Helps self-hosters and
	// cloud operators quickly confirm the running configuration.
	slog.Info("quipthread starting",
		"port", cfg.Port,
		"base_url", cfg.BaseURL,
		"cloud_mode", cfg.CloudMode,
		"email_auth", cfg.EmailAuthEnabled,
		"github_oauth", cfg.GitHubClientID != "",
		"google_oauth", cfg.GoogleClientID != "",
		"turnstile", cfg.TurnstileSecretKey != "",
		"trust_proxy", cfg.TrustProxy,
		"database", sanitizeDBURL(cfg.DatabaseURL),
	)

	// Notification dispatcher — runs in background, cancelled on shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if cfg.CloudMode {
		// Containment: the dispatcher enumerates sites and reads pending
		// comments through the global constructor store, which would cross
		// tenant boundaries in cloud mode. Until a tenant-aware dispatcher
		// exists it must not start, and StartDispatcher additionally fails
		// closed if it is ever invoked with CloudMode set.
		slog.Info("notification dispatcher disabled in cloud mode")
	} else {
		notifier := notifications.Build(cfg, store)
		go notifications.StartDispatcher(ctx, store, notifier, cfg)
	}
	deliveryWorker := startCloudNotificationDelivery(ctx, cfg, cloudStore, tenantCache)

	authHandler := authhandlers.NewHandlerWithCloud(store, cfg, cloudStore)
	commentsHandler := handlers.NewCommentsHandler(store, cfg)
	adminHandler := handlers.NewAdminHandlerWithCloud(store, cfg, cloudStore)
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
	r.Use(middleware.Recovery)
	r.Use(chimiddleware.RequestID)
	r.Use(middleware.CORSWithBaseURL(cfg.BaseURL, cfg.AllowedOrigins))
	r.Use(middleware.LimitRequestBody)
	r.Use(middleware.EnforceDashboardOrigin(cfg.BaseURL))
	r.Use(middleware.SecurityHeaders)
	if cfg.CloudMode && cloudStore != nil {
		r.Use(middleware.InjectTenantStore(cloudStore, tenantCache, cfg))
	}

	// --- Auth routes (public) -----------------------------------------------
	r.Get("/auth/github/login", authHandler.GithubLogin)
	r.Get("/auth/github/callback", authHandler.GithubCallback)
	r.Get("/auth/github/link", authHandler.GithubLink)
	r.Get("/auth/google/login", authHandler.GoogleLogin)
	r.Get("/auth/google/callback", authHandler.GoogleCallback)
	r.Get("/auth/google/link", authHandler.GoogleLink)

	// Email auth endpoints are rate-limited (brute-force protection).
	r.With(middleware.RateLimit(authRL, ipFn)).Post("/auth/email/register", authHandler.EmailRegister)
	r.Get("/auth/email/verify/{token}", authHandler.EmailVerify)
	r.With(middleware.RateLimit(authRL, ipFn)).Post("/auth/email/resend-verification", authHandler.EmailResend)
	r.With(middleware.RateLimit(authRL, ipFn)).Post("/auth/email/login", authHandler.EmailLogin)
	r.With(middleware.RateLimit(authRL, ipFn)).Post("/auth/email/forgot", authHandler.EmailForgot)
	r.Get("/auth/email/poll", authHandler.EmailPoll)
	r.Get("/auth/email/reset/{token}", authHandler.EmailResetPage)
	r.Post("/auth/email/reset/{token}", authHandler.EmailReset)

	// Cloud email auth routes (no-op in non-cloud builds).
	authhandlers.RegisterCloudAuthRoutes(r, authHandler, authRL, ipFn)

	// Dashboard and embed sessions are separate trust domains. The old routes
	// remain dashboard-only compatibility aliases; they do not accept the
	// legacy cookie.
	r.Post("/auth/logout", authHandler.Logout)
	r.Post("/auth/dashboard/logout", authHandler.DashboardLogout)
	r.Post("/auth/embed/logout", authHandler.EmbedLogout)
	r.Post("/api/auth/dashboard/logout", authHandler.DashboardLogout)
	r.Post("/api/auth/embed/logout", authHandler.EmbedLogout)
	r.Get("/api/auth/me", authHandler.Me)
	r.Get("/api/auth/dashboard/me", middleware.RequireAuthForAudience(cfg.JWTSecret, session.DashboardAudience, store)(http.HandlerFunc(authHandler.DashboardMe)).ServeHTTP)
	r.Get("/api/auth/embed/me", middleware.RequireAuthForAudience(cfg.JWTSecret, session.EmbedAudience, store)(http.HandlerFunc(authHandler.EmbedMe)).ServeHTTP)
	r.Post("/api/auth/dashboard/logout-all", middleware.RequireAuthForAudience(cfg.JWTSecret, session.DashboardAudience, store)(http.HandlerFunc(accountHandler.LogoutAll)).ServeHTTP)
	r.Post("/auth/logout-all", middleware.RequireAuthForAudience(cfg.JWTSecret, session.DashboardAudience, store)(http.HandlerFunc(accountHandler.LogoutAll)).ServeHTTP)

	// Billing routes (public webhook + admin-protected status/checkout/portal).
	handlers.RegisterBillingRoutes(r, store, cfg, cloudStore, tenantCache)

	// Invitation routes (cloud-only; no-op stub in non-cloud builds).
	handlers.RegisterInvitationRoutes(r, cfg, store, cloudStore)

	// SSO auth route (cloud-only; no-op stub in non-cloud builds).
	authhandlers.RegisterSSORoutes(r, authHandler, middleware.EnforceExactOrigin(cfg.BaseURL))

	// Embed preview page — public, no auth, safe to iframe from the dashboard.
	r.Get("/embed-preview", handlers.HandleEmbedPreview)

	// --- Public API routes (see publicRoutes.register) ----------------------
	//
	// Route-builder seam: the covered public embed/comment routes —
	// GET /api/config, GET /api/comments, POST /api/comments,
	// DELETE /api/comments/{id}, POST /api/comments/{id}/vote,
	// POST /api/comments/{id}/flag — are mounted by publicRoutes.register
	// with their exact production middleware wiring, extracted verbatim so
	// router-level tests can exercise the real wiring without a server.
	publicRoutes{
		cfg:             cfg,
		store:           store,
		cloudStore:      cloudStore,
		tenantCache:     tenantCache,
		configHandler:   configHandler,
		commentsHandler: commentsHandler,
		jwtSecret:       cfg.JWTSecret,
		commentsRL:      commentsRL,
		ipFn:            ipFn,
	}.register(r)

	registerApprovalRoutes(r, cfg, store, cloudStore, tenantCache)

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

		// Dashboard pages: require a valid session server-side to avoid flashing
		// the layout before the client-side AuthGuard can redirect to /login.
		requireSessionOrLogin := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				cookie, err := r.Cookie(session.DashboardCookieName)
				if err != nil {
					http.Redirect(w, r, "/login", http.StatusFound)
					return
				}
				if claims, _ := session.ParseForAudience(cfg.JWTSecret, cookie.Value, session.DashboardAudience); claims == nil {
					http.Redirect(w, r, "/login", http.StatusFound)
					return
				}
				next.ServeHTTP(w, r)
			})
		}
		r.Handle("/dashboard", http.RedirectHandler("/dashboard/", http.StatusMovedPermanently))
		r.Handle("/dashboard/*", requireSessionOrLogin(http.StripPrefix("/dashboard", dashFileServer)))

		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"error":"not_found"}`) //nolint:errcheck // ResponseWriter.Write errors are not actionable
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, notFoundPage) //nolint:errcheck // ResponseWriter.Write errors are not actionable
		})
	}

	// Health check — required by Fly.io deployment checks.
	// Returns JSON with DB reachability so operators can diagnose degraded state.
	serverStart := time.Now()
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		_, dbErr := store.CountSites()
		dbOK := dbErr == nil

		status := http.StatusOK
		dbStatus := "ok"
		if !dbOK {
			status = http.StatusServiceUnavailable
			dbStatus = "error"
			slog.ErrorContext(req.Context(), "health check: database unreachable", "error", dbErr)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprintf(w, `{"status":%q,"database":%q,"uptime_seconds":%d}`, //nolint:errcheck // ResponseWriter.Write errors are not actionable
			map[bool]string{true: "ok", false: "degraded"}[dbOK],
			dbStatus,
			int(time.Since(serverStart).Seconds()),
		)
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

	// --- Admin routes -------------------------------------------------------
	r.Group(func(r chi.Router) {
		r.Use(middleware.EnforceExactOrigin(cfg.BaseURL))
		r.Use(middleware.RequireAdminForAudience(cfg.JWTSecret, session.DashboardAudience, store))

		r.Get("/api/admin/comments", adminHandler.ListComments)
		r.Patch("/api/admin/comments/{id}", adminHandler.UpdateComment)
		r.Post("/api/admin/comments/{id}/reply", adminHandler.Reply)
		r.Delete("/api/admin/comments/{id}", adminHandler.DeleteComment)

		r.Get("/api/admin/users", adminHandler.ListUsers)
		r.Patch("/api/admin/users/{id}", adminHandler.UpdateUser)

		r.Get("/api/admin/sites", adminHandler.ListSites)
		r.With(middleware.EnforceSiteLimit(store, cfg)).Post("/api/admin/sites", adminHandler.CreateSite)
		r.Patch("/api/admin/sites/{id}", adminHandler.UpdateSite)
		r.Delete("/api/admin/sites/{id}", adminHandler.DeleteSite)

		// SSO secret management (cloud Enterprise only; no-op stub in non-cloud builds).
		handlers.RegisterSSOAdminRoutes(r, adminHandler)

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
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 * 1024,
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

	deliveryShutdownCtx, deliveryShutdownCancel := context.WithTimeout(context.Background(), notifications.CloudTenantDeliveryShutdownTimeout)
	if err := deliveryWorker.Stop(deliveryShutdownCtx); err != nil {
		log.Printf("cloud notification delivery shutdown: %v", err)
	}
	deliveryShutdownCancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}

	// Requests have stopped — release cached tenant stores before the deferred
	// global/cloud store closes run at function exit.
	if err := tenantCache.Close(); err != nil {
		log.Printf("tenant store cache shutdown: %v", err)
	}
}

// startCloudNotificationDelivery is the only startup wiring for cloud
// notification delivery. It is deliberately fail-closed: the dedicated opt-in
// and every runtime prerequisite must be present before a worker is created.
// Cloud mode never falls back to StartDispatcher or the process-global store.
func startCloudNotificationDelivery(ctx context.Context, cfg *config.Config, cloudStore cloud.Store, tenantCache *middleware.StoreCache) *notifications.CloudTenantDeliveryWorker {
	if err := validateCloudNotificationDeliveryConfig(cfg, cloudStore, tenantCache); err != nil {
		if cfg != nil && cfg.CloudNotificationDeliveryEnabled {
			slog.Warn("cloud notification delivery disabled", "reason", err.Error())
		}
		return nil
	}

	channels := notifications.CloudEligibleChannelNames(cfg)
	opener := func(openCtx context.Context, locator cloud.AccountLocator) (notifications.CloudTenantDeliveryLease, error) {
		if err := openCtx.Err(); err != nil {
			return nil, err
		}
		account := &cloud.Account{ID: locator.ID, DBType: locator.DBType, DBURL: locator.DBURL}
		cacheLease, err := tenantCache.GetOrOpenContext(openCtx, locator.ID, middleware.StorageFingerprint(locator.DBType, locator.DBURL), func(openCtx context.Context) (db.Store, error) {
			return middleware.OpenTenantStoreContext(openCtx, account, cfg)
		})
		if err != nil {
			return nil, err
		}
		if err := openCtx.Err(); err != nil {
			cacheLease.Release()
			return nil, err
		}
		return notifications.NewCloudTenantDeliveryStoreLease(cacheLease.Store(), func() error {
			cacheLease.Release()
			return nil
		}), nil
	}
	options := notifications.CloudTenantDeliveryPassOptions{
		Config: cfg, ChannelNames: channels, BaseURL: cfg.BaseURL,
		ApprovalTokenHMACKey:      cfg.ApprovalTokenHMACKey,
		ApprovalTokenLocatorStore: cloudStore,
		SenderFactory:             notifications.NewCloudTenantChannelSenderFactory(cfg),
	}
	return notifications.StartCloudTenantDeliveryWorkerWithSummary(ctx, func(passCtx context.Context) (notifications.CloudTenantDeliveryPassReport, error) {
		return notifications.RunCloudTenantDeliveryPass(passCtx, cloudStore, opener, options)
	}, func(summary notifications.CloudTenantDeliverySummary) {
		slog.Info("cloud notification delivery pass complete",
			"pages", summary.Pages,
			"accounts", summary.Accounts,
			"tenants", summary.Tenants,
			"parents_claimed", summary.ParentsClaimed,
			"parents_finalized", summary.ParentsFinalized,
			"children_sent", summary.ChildrenSent,
			"timed_out_tenants", summary.TimedOutTenants,
			"failure_counts", summary.FailureCounts,
			"pass_failure", summary.PassFailure,
		)
	})
}

func validateCloudNotificationDeliveryConfig(cfg *config.Config, cloudStore cloud.Store, tenantCache *middleware.StoreCache) error {
	if cfg == nil || !cfg.CloudMode || !cfg.CloudNotificationDeliveryEnabled {
		return fmt.Errorf("dedicated cloud notification delivery opt-in is disabled")
	}
	if cloudStore == nil {
		return fmt.Errorf("cloud control-plane store is unavailable")
	}
	if tenantCache == nil {
		return fmt.Errorf("tenant store cache is unavailable")
	}
	if err := validateCloudConfig(cfg); err != nil {
		return fmt.Errorf("cloud prerequisites are incomplete")
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return fmt.Errorf("base URL is required")
	}
	if len(notifications.CloudEligibleChannelNames(cfg)) == 0 {
		return fmt.Errorf("no cloud notification channel is configured")
	}
	return nil
}

// publicRoutes carries the collaborators needed to mount the covered public
// embed/comment routes. It exists as a route-builder seam so router-level
// tests can exercise the exact production wiring (handler construction and
// middleware chain) without starting a server.
type publicRoutes struct {
	cfg             *config.Config
	store           db.Store // constructor/global store: legacy fallbacks + quota source
	cloudStore      cloud.Store
	tenantCache     *middleware.StoreCache
	configHandler   *handlers.ConfigHandler
	commentsHandler *handlers.CommentsHandler
	jwtSecret       string
	commentsRL      middleware.RateLimiter
	ipFn            func(*http.Request) string
}

// register mounts exactly these routes, with the same conditional resolver
// wiring production has always used:
//
//	GET  /api/config                 (public)
//	GET  /api/comments               (public; optional auth populates user_voted)
//	POST /api/comments               (RequireAuth)
//	DELETE /api/comments/{id}        (RequireAuth)
//	POST /api/comments/{id}/vote     (RequireAuth)
//	POST /api/comments/{id}/flag     (RequireAuth)
//
// When the cloud-only public site resolver is enabled, every one of them
// resolves its tenant from the site registry via route-local middleware; the
// global InjectTenantStore middleware bypasses these exact routes so
// cookies/JWT never determine the tenant store here. With the flag disabled or
// in self-hosted mode, legacy behavior is preserved.
func (d publicRoutes) register(r chi.Router) {
	enabled := d.cfg.CloudMode && d.cfg.PublicSiteResolverEnabled && d.cloudStore != nil

	if enabled {
		resolvePublic := middleware.PublicSiteResolver(d.cloudStore, d.tenantCache, d.cfg)
		r.With(resolvePublic).Get("/api/config", d.configHandler.PublicConfig)
		r.With(resolvePublic, middleware.InjectAuthForAudience(d.jwtSecret, session.EmbedAudience, d.store)).Get("/api/comments", d.commentsHandler.List)
	} else {
		r.Get("/api/config", d.configHandler.PublicConfig)
		// InjectAuth is optional — populates user_voted when a session cookie is present.
		r.With(middleware.InjectAuthForAudience(d.jwtSecret, session.EmbedAudience, d.store)).Get("/api/comments", d.commentsHandler.List)
	}

	// --- Authenticated commenter routes -------------------------------------
	r.Group(func(g chi.Router) {
		// In cloud public-resolver mode the route-local resolver runs after this
		// middleware, so there is no tenant store available yet. This middleware
		// authenticates the audience first; the resolved tenant's session state
		// is checked below. Other routes validate against their store here.
		fallback := d.store
		if enabled {
			fallback = nil
		}
		g.Use(middleware.RequireAuthForAudience(d.jwtSecret, session.EmbedAudience, fallback))

		if enabled {
			requireTenantAuth := middleware.RequireAuthForAudience(d.jwtSecret, session.EmbedAudience, nil)
			// Public-resolver mode: POST /api/comments resolves its tenant from
			// the body's site_id via the route-local body resolver (the
			// global InjectTenantStore bypasses this exact route so a session
			// cookie can never select its tenant store). Chain order is
			// RequireAuth → RateLimit → body resolver → tenant auth → origin/CSRF check →
			// request-scoped quota → handler; the origin check is bound to the
			// resolved verified site's Domain, and quota reads only the
			// resolved public tenant and fails closed without it.
			resolveBody := middleware.PublicSiteResolverPOST(d.cloudStore, d.tenantCache, d.cfg)
			g.With(middleware.RateLimit(d.commentsRL, d.ipFn), resolveBody, requireTenantAuth,
				middleware.EnforcePublicPostOrigin(d.cfg),
				middleware.EnforceCommentQuota(d.store, d.cfg)).Post("/api/comments", d.commentsHandler.Create)

			// Comment mutations (delete/vote/flag) resolve their tenant from
			// the required siteId query parameter via the same query resolver
			// as the public GET routes; the global InjectTenantStore bypasses
			// these exact routes so a session cookie can never select their
			// tenant store. Chain order: RequireAuth → RateLimit → query
			// resolver → tenant auth → site-bound origin check → (flag only) request-scoped
			// plan gate → handler. The origin check requires an exact match
			// against the resolved site's Domain but no JSON media type —
			// these requests are bodyless.
			resolvePublic := middleware.PublicSiteResolver(d.cloudStore, d.tenantCache, d.cfg)
			enforceMutationOrigin := middleware.EnforcePublicMutationOrigin(d.cfg)
			g.With(middleware.RateLimit(d.commentsRL, d.ipFn), resolvePublic, requireTenantAuth, enforceMutationOrigin).
				Delete("/api/comments/{id}", d.commentsHandler.Delete)
			g.With(middleware.RateLimit(d.commentsRL, d.ipFn), resolvePublic, requireTenantAuth, enforceMutationOrigin).
				Post("/api/comments/{id}/vote", d.commentsHandler.Vote)
			g.With(middleware.RateLimit(d.commentsRL, d.ipFn), resolvePublic, requireTenantAuth, enforceMutationOrigin,
				middleware.RequirePlanPublic(d.cfg, "starter")).Post("/api/comments/{id}/flag", d.commentsHandler.Flag)
		} else {
			// Legacy ordering preserved when disabled or self-hosted.
			if !d.cfg.CloudMode {
				g.Use(middleware.EnforceEmbedOrigin(d.cfg.AllowedOrigins))
			}
			g.With(middleware.RateLimit(d.commentsRL, d.ipFn), middleware.EnforceCommentQuota(d.store, d.cfg)).Post("/api/comments", d.commentsHandler.Create)
			g.Delete("/api/comments/{id}", d.commentsHandler.Delete)
			g.Post("/api/comments/{id}/vote", d.commentsHandler.Vote)
			g.With(middleware.RequirePlan(d.store, d.cfg, "starter")).Post("/api/comments/{id}/flag", d.commentsHandler.Flag)
		}
	})
}

// newApprovalTenantResolver returns the narrow tenant-store resolver used by
// the cloud approval controller. It resolves the locator account's own tenant
// store through the shared bounded tenant-store cache and never opens the
// global/master content store: the only account it will ever open is the one
// the central locator names, looked up through the control-plane store.
func newApprovalTenantResolver(cfg *config.Config, cloudStore cloud.Store, cache *middleware.StoreCache) handlers.ApprovalTenantStoreResolver {
	return func(accountID string) (db.Store, func(), error) {
		acc, err := cloudStore.GetAccountByID(accountID)
		if err != nil || acc == nil {
			return nil, nil, fmt.Errorf("approval tenant: account unavailable")
		}
		lease, err := cache.GetOrOpen(accountID, middleware.StorageFingerprint(acc.DBType, acc.DBURL), func() (db.Store, error) {
			return middleware.OpenTenantStore(acc, cfg)
		})
		if err != nil {
			return nil, nil, err
		}
		return lease.Store(), lease.Release, nil
	}
}

// registerApprovalRoutes mounts the approval page/action routes:
//
//	GET  /approve/{token}
//	POST /approve/{token}
//
// Self-hosted deployments keep the original handlers wired to their store,
// unchanged. In managed cloud mode (CLOUD_MODE=true) the routes resolve the
// raw path token to a hashed central locator, then serve and mutate only the
// locator account's tenant store (see handlers.ApprovalCloudDeps). They are
// mounted only when every dependency and configuration value is valid — the
// control-plane store and the dedicated approval-token HMAC key — otherwise
// they fail closed with the deterministic storeless 503 feature_unavailable:
// there is no safe tenant resolution without those, and falling back to the
// global constructor store would read and mutate content across tenant
// boundaries.
func registerApprovalRoutes(r chi.Router, cfg *config.Config, store db.Store, cloudStore cloud.Store, tenantCache *middleware.StoreCache) {
	if cfg.CloudMode {
		if cloudStore != nil && validateCloudConfig(cfg) == nil {
			deps := handlers.ApprovalCloudDeps{
				CloudStore:    cloudStore,
				HMACKey:       cfg.ApprovalTokenHMACKey,
				ResolveTenant: newApprovalTenantResolver(cfg, cloudStore, tenantCache),
				Now:           time.Now,
			}
			r.Get("/approve/{token}", handlers.HandleCloudApprovalPage(deps))
			r.Post("/approve/{token}", handlers.HandleCloudApprovalAction(deps))
			return
		}
		r.Get("/approve/{token}", handlers.HandleApprovalPageUnavailable)
		r.Post("/approve/{token}", handlers.HandleApprovalActionUnavailable)
		return
	}
	// Approval page (server-rendered, no dashboard login required) — M7
	r.Get("/approve/{token}", handlers.HandleApprovalPage(store))
	r.Post("/approve/{token}", handlers.HandleApprovalAction(store))
}

// sanitizeDBURL strips auth tokens from database URLs before logging.
func sanitizeDBURL(u string) string {
	if strings.HasPrefix(u, "libsql://") || strings.HasPrefix(u, "https://") {
		if parsed, err := neturl.Parse(u); err == nil {
			parsed.RawQuery = "" // strip authToken= query param
			return parsed.String()
		}
	}
	return u
}

// validateCloudConfig fails closed when CLOUD_MODE=true is set but required
// cloud-only configuration is missing or invalid. The approval-token HMAC key
// must be present and at least 32 bytes; errors carry only length metadata,
// never key material. Self-hosted deployments are unaffected: validation is a
// no-op when CloudMode is false.
func validateCloudConfig(cfg *config.Config) error {
	if !cfg.CloudMode {
		return nil
	}
	return approvaltoken.ValidateKey(cfg.ApprovalTokenHMACKey)
}

// validateCloudStore fails closed when CLOUD_MODE=true cannot be honored:
// either the store could not be opened, or the build/runtime cannot supply a
// cloud control-plane store (nil with no error). In both cases cloud mode must
// not start — site-registry dual writes and tenant isolation depend on it.
func validateCloudStore(cs cloud.Store, err error) (cloud.Store, error) {
	if err != nil {
		return nil, err
	}
	if cs == nil {
		return nil, fmt.Errorf("no cloud control-plane store available; refusing to run cloud mode without registry dual writes")
	}
	return cs, nil
}

func openStore(cfg *config.Config) (db.Store, error) {
	if !cfg.CloudMode || !cloudRuntimeBuild {
		if err := config.ValidateSelfHostedDatabaseURL(cfg.DatabaseURL); err != nil {
			return nil, err
		}
	}
	url := cfg.DatabaseURL
	if strings.HasPrefix(url, "libsql://") || strings.HasPrefix(url, "https://") {
		return db.NewLibSQLStoreWithAuthToken(url, cfg.TursoAuthToken)
	}

	// Local SQLite — ensure the data directory exists first.
	if err := os.MkdirAll(filepath.Dir(url), 0750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return db.NewSQLiteStore(url)
}

func applyServerMigrations(ctx context.Context, cfg *config.Config) error {
	runner, err := migration.NewRunner(migration.Config{})
	if err != nil {
		return err
	}
	if strings.HasPrefix(cfg.DatabaseURL, "libsql://") || strings.HasPrefix(cfg.DatabaseURL, "https://") {
		_, err := runner.ApplyExisting(ctx, migration.Target{AccountID: "server", TargetURL: cfg.DatabaseURL, AuthToken: cfg.TursoAuthToken})
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DatabaseURL), 0750); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	path, err := filepath.Abs(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("resolve database path: %w", err)
	}
	targetURL := (&neturl.URL{Scheme: "sqlite", Path: path}).String()
	_, err = runner.ApplyExisting(ctx, migration.Target{
		AccountID: "self-hosted",
		TargetURL: targetURL,
	})
	return err
}

func buildRateLimiter(spec string, defaultCount int, defaultPeriod time.Duration) middleware.RateLimiter {
	count, period, err := middleware.ParseWindow(spec)
	if err != nil {
		log.Printf("invalid rate limit spec %q, using default: %v", spec, err)
		count, period = defaultCount, defaultPeriod
	}
	return middleware.NewMemoryRateLimiter(count, period)
}

const notFoundPage = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>404 — Quipthread</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body { font-family: system-ui, sans-serif; background: #0f0f0f; color: #e8e3dc; min-height: 100vh; display: flex; align-items: center; justify-content: center; }
    .wrap { text-align: center; padding: 2rem; }
    .code { font-size: 5rem; font-weight: 700; color: #e07f32; line-height: 1; }
    h1 { font-size: 1.25rem; font-weight: 500; margin: 0.75rem 0 1.5rem; opacity: 0.6; }
    a { color: #e07f32; text-decoration: none; font-size: 0.9375rem; border-bottom: 1px solid transparent; }
    a:hover { border-bottom-color: #e07f32; }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="code">404</div>
    <h1>Page not found</h1>
    <a href="https://app.quipthread.com">Go home</a>
  </div>
</body>
</html>`

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
