package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	cloudpkg "github.com/quipthread/quipthread/cloud"
	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/models"
	"github.com/quipthread/quipthread/session"
)

// Provider abstracts OAuth and OAuth-like login flows.
type Provider interface {
	Name() string
	LoginURL(state string) string
	ExchangeUser(ctx context.Context, r *http.Request) (*UserInfo, error)
}

// UserInfo is the normalised identity returned by every provider.
type UserInfo struct {
	ProviderID  string
	Provider    string
	Email       string
	DisplayName string
	AvatarURL   string
	Username    string // human-readable handle; populated by providers that expose one (e.g. GitHub login)
}

// Handler holds shared dependencies for all auth sub-handlers.
type Handler struct {
	store      db.Store
	config     *config.Config
	cloudStore cloudpkg.Store
	github     *GithubProvider
	google     *GoogleProvider
	email      *EmailProvider
}

func NewHandler(store db.Store, cfg *config.Config) *Handler {
	return NewHandlerWithCloud(store, cfg, nil)
}

func NewHandlerWithCloud(store db.Store, cfg *config.Config, cloudStore cloudpkg.Store) *Handler {
	h := &Handler{
		store:      store,
		config:     cfg,
		cloudStore: cloudStore,
	}

	if cfg.GitHubClientID != "" && cfg.GitHubSecret != "" {
		h.github = NewGithubProvider(cfg)
	}
	if cfg.GoogleClientID != "" && cfg.GoogleSecret != "" {
		h.google = NewGoogleProvider(cfg)
	}
	if cfg.EmailAuthEnabled {
		h.email = NewEmailProvider(store, cfg)
	}

	return h
}

// Me returns current session claims as JSON, or 401 if not authenticated.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(session.CookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	claims, err := session.Parse(h.config.JWTSecret, cookie.Value)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":           claims.Sub,
		"display_name": claims.DisplayName,
		"provider":     claims.Provider,
		"role":         claims.Role,
	})
}

// Logout clears the session cookie.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	session.ClearCookie(w)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "logged out"}) //nolint:errcheck // error response; connection may already be broken
}

// --- state cookie helpers ---------------------------------------------------

const stateCookieName = "quipthread_oauth_state"

func generateState() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func setStateCookie(w http.ResponseWriter, state string) {
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
}

func validateStateCookie(r *http.Request, state string) bool {
	cookie, err := r.Cookie(stateCookieName)
	if err != nil {
		return false
	}
	return state != "" && cookie.Value == state
}

func clearStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   stateCookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

// --- returnTo cookie helpers ------------------------------------------------

const returnToCookieName = "quipthread_return_to"

// validateReturnTo returns true if returnTo is a safe http/https URL whose
// origin matches either the configured BASE_URL or one of AllowedOrigins.
// When AllowedOrigins is empty, any http/https URL is accepted.
func (h *Handler) validateReturnTo(returnTo string) bool {
	u, err := url.Parse(returnTo)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	origin := u.Scheme + "://" + u.Host

	// Always allow returnTo on the same origin as BASE_URL (covers /admin/*).
	if base, err := url.Parse(h.config.BaseURL); err == nil {
		baseOrigin := base.Scheme + "://" + base.Host
		if origin == baseOrigin {
			return true
		}
	}

	if len(h.config.AllowedOrigins) == 0 {
		return true
	}
	for _, allowed := range h.config.AllowedOrigins {
		if strings.TrimRight(allowed, "/") == origin {
			return true
		}
	}
	return false
}

func setReturnToCookie(w http.ResponseWriter, returnTo string) {
	http.SetCookie(w, &http.Cookie{
		Name:     returnToCookieName,
		Value:    returnTo,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
}

// consumeReturnToCookie reads the returnTo cookie, clears it, and returns its value.
func consumeReturnToCookie(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie(returnToCookieName)
	if err != nil {
		return ""
	}
	http.SetCookie(w, &http.Cookie{
		Name:   returnToCookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	return cookie.Value
}

// --- shared upsert + JWT issue ----------------------------------------------

// upsertAndIssueToken finds or creates the user for the given provider identity,
// then issues a session JWT. Returns the signed token string.
func (h *Handler) upsertAndIssueToken(info *UserInfo) (string, error) {
	identity, err := h.store.GetIdentity(info.Provider, info.ProviderID)
	if err != nil {
		return "", fmt.Errorf("get identity: %w", err)
	}

	var userID string

	if identity == nil {
		// First user to register becomes admin automatically.
		role := "commenter"
		if _, total, err := h.store.ListUsers(1, 1); err == nil && total == 0 {
			role = "admin"
		}

		u := &models.User{
			DisplayName: info.DisplayName,
			Email:       info.Email,
			AvatarURL:   info.AvatarURL,
			Role:        role,
		}
		if err := h.store.UpsertUser(u); err != nil {
			return "", fmt.Errorf("upsert user: %w", err)
		}

		ident := &models.UserIdentity{
			UserID:     u.ID,
			Provider:   info.Provider,
			ProviderID: info.ProviderID,
			Username:   info.Username,
		}
		if err := h.store.CreateIdentity(ident); err != nil {
			return "", fmt.Errorf("create identity: %w", err)
		}
		userID = u.ID
	} else {
		userID = identity.UserID
		u, err := h.store.GetUser(userID)
		if err != nil {
			return "", fmt.Errorf("get user: %w", err)
		}
		if u != nil {
			u.DisplayName = info.DisplayName
			u.AvatarURL = info.AvatarURL
			if info.Email != "" {
				u.Email = info.Email
			}
			if err := h.store.UpsertUser(u); err != nil {
				log.Printf("auth: upsert user profile on login %s: %v", userID, err)
			}
		}
		if info.Username != "" {
			if err := h.store.UpdateIdentityUsername(userID, info.Provider, info.Username); err != nil {
				log.Printf("auth: update identity username %s: %v", userID, err)
			}
		}
	}

	user, err := h.store.GetUser(userID)
	if err != nil || user == nil {
		return "", fmt.Errorf("could not load user after upsert")
	}
	if user.Banned {
		return "", fmt.Errorf("account is banned")
	}

	return session.Issue(h.config.JWTSecret, user.ID, user.DisplayName, info.Provider, user.Role, "")
}

// seedTenantDB ensures the tenant SQLite contains a user record and identity
// row for the given account. It is a no-op for non-SQLite tenant DBs and is
// safe to call on every login (idempotent — only writes on first call).
func seedTenantDB(dbURL, accountID string, info *UserInfo) {
	// Skip non-local DBs (Turso/libSQL provisioning is handled separately).
	if strings.HasPrefix(dbURL, "libsql://") || strings.HasPrefix(dbURL, "https://") {
		return
	}
	s, err := db.NewSQLiteStore(dbURL)
	if err != nil {
		log.Printf("seedTenantDB: open store for account %s: %v", accountID, err)
		return
	}
	defer s.Close()

	existing, _ := s.GetUser(accountID)
	if existing == nil {
		u := &models.User{
			ID:          accountID,
			DisplayName: info.DisplayName,
			Email:       info.Email,
			AvatarURL:   info.AvatarURL,
			Role:        "admin",
		}
		if err := s.UpsertUser(u); err != nil {
			log.Printf("seedTenantDB: upsert user for account %s: %v", accountID, err)
		}
		if err := s.CreateIdentity(&models.UserIdentity{
			UserID:     accountID,
			Provider:   info.Provider,
			ProviderID: info.ProviderID,
			Username:   info.Username,
		}); err != nil {
			log.Printf("seedTenantDB: create identity for account %s: %v", accountID, err)
		}
	} else {
		// Keep profile fields fresh on every login.
		existing.DisplayName = info.DisplayName
		existing.AvatarURL = info.AvatarURL
		if info.Email != "" {
			existing.Email = info.Email
		}
		if err := s.UpsertUser(existing); err != nil {
			log.Printf("seedTenantDB: update user for account %s: %v", accountID, err)
		}
		if info.Username != "" {
			if err := s.UpdateIdentityUsername(accountID, info.Provider, info.Username); err != nil {
				log.Printf("seedTenantDB: update username for account %s: %v", accountID, err)
			}
		}
	}
}

// cloudUpsertAndIssueToken handles OAuth sign-in/sign-up in cloud mode.
// It looks up the OAuth link, falls back to email match, or creates a new account.
func (h *Handler) cloudUpsertAndIssueToken(
	w http.ResponseWriter, r *http.Request,
	info *UserInfo,
) {
	// Step 1: oauth_links lookup — returning user
	link, err := h.cloudStore.GetOAuthLink(info.Provider, info.ProviderID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	if link != nil {
		acc, err := h.cloudStore.GetAccountByID(link.AccountID)
		if err != nil || acc == nil {
			http.Error(w, "account not found", http.StatusInternalServerError)
			return
		}
		seedTenantDB(acc.DBURL, acc.ID, info)
		tok, err := session.Issue(h.config.JWTSecret, acc.ID, acc.Email, info.Provider, "admin", acc.ID)
		if err != nil {
			http.Error(w, "session error", http.StatusInternalServerError)
			return
		}
		session.SetCookie(w, tok, r.TLS != nil)
		http.Redirect(w, r, h.config.BaseURL+"/dashboard/", http.StatusFound)
		return
	}

	// Step 2: email match — link new provider to existing account
	if info.Email != "" {
		acc, err := h.cloudStore.GetAccountByEmail(info.Email)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
		if acc != nil {
			_ = h.cloudStore.CreateOAuthLink(&cloudpkg.OAuthLink{
				AccountID:      acc.ID,
				Provider:       info.Provider,
				ProviderUserID: info.ProviderID,
				Email:          info.Email,
			})
			seedTenantDB(acc.DBURL, acc.ID, info)
			tok, err := session.Issue(h.config.JWTSecret, acc.ID, acc.Email, info.Provider, "admin", acc.ID)
			if err != nil {
				http.Error(w, "session error", http.StatusInternalServerError)
				return
			}
			session.SetCookie(w, tok, r.TLS != nil)
			http.Redirect(w, r, h.config.BaseURL+"/dashboard/", http.StatusFound)
			return
		}
	}

	// Step 3: new account
	accountID := uuid.New().String()
	dbPath, err := cloudpkg.ProvisionSQLite(h.config.TenantDataDir, accountID)
	if err != nil {
		http.Error(w, "provisioning failed", http.StatusInternalServerError)
		return
	}
	acc := &cloudpkg.Account{
		ID:            accountID,
		Email:         info.Email,
		EmailVerified: true, // OAuth emails are pre-verified
		Plan:          "hobby",
		DBType:        "sqlite",
		DBURL:         dbPath,
		CreatedAt:     time.Now().UTC(),
	}
	if err := h.cloudStore.CreateAccount(acc); err != nil {
		http.Error(w, "account creation failed", http.StatusInternalServerError)
		return
	}
	_ = h.cloudStore.CreateOAuthLink(&cloudpkg.OAuthLink{
		AccountID:      accountID,
		Provider:       info.Provider,
		ProviderUserID: info.ProviderID,
		Email:          info.Email,
	})
	seedTenantDB(dbPath, accountID, info)
	tok, err := session.Issue(h.config.JWTSecret, accountID, info.Email, info.Provider, "admin", accountID)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	session.SetCookie(w, tok, r.TLS != nil)
	http.Redirect(w, r, h.config.BaseURL+"/dashboard/onboarding", http.StatusFound)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck // error response; connection may already be broken
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
