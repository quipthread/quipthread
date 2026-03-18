package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/quipthread/quipthread/models"
)

const schema = `
CREATE TABLE IF NOT EXISTS sites (
    id          TEXT PRIMARY KEY,
    owner_id    TEXT NOT NULL,
    domain      TEXT NOT NULL,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    display_name  TEXT NOT NULL,
    email         TEXT,
    avatar_url    TEXT,
    role          TEXT DEFAULT 'commenter',
    banned        INTEGER DEFAULT 0,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_identities (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL,
    provider      TEXT NOT NULL,
    provider_id   TEXT NOT NULL,
    password_hash TEXT,
    UNIQUE(provider, provider_id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS comments (
    id            TEXT PRIMARY KEY,
    site_id       TEXT NOT NULL,
    page_id       TEXT NOT NULL,
    page_url      TEXT,
    page_title    TEXT,
    parent_id     TEXT,
    user_id       TEXT NOT NULL,
    content       TEXT NOT NULL,
    status        TEXT DEFAULT 'pending',
    imported      INTEGER DEFAULT 0,
    disqus_author TEXT,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (site_id) REFERENCES sites(id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS approval_tokens (
    token         TEXT PRIMARY KEY,
    comment_id    TEXT NOT NULL,
    expires_at    DATETIME NOT NULL,
    FOREIGN KEY (comment_id) REFERENCES comments(id)
);

CREATE TABLE IF NOT EXISTS email_tokens (
    token      TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL,
    type       TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id                  TEXT PRIMARY KEY DEFAULT 'account',
    stripe_customer_id  TEXT NOT NULL DEFAULT '',
    stripe_sub_id       TEXT NOT NULL DEFAULT '',
    plan                TEXT NOT NULL DEFAULT 'hobby',
    status              TEXT NOT NULL DEFAULT 'active',
    interval            TEXT NOT NULL DEFAULT '',
    trial_ends_at       DATETIME,
    current_period_end  DATETIME,
    updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

type sqlStore struct {
	db *sql.DB
}

func (s *sqlStore) migrate() error {
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	// Idempotent additive migrations — ignore "duplicate column name" errors.
	s.db.Exec(`ALTER TABLE sites ADD COLUMN last_notified_at DATETIME`)
	s.db.Exec(`ALTER TABLE sites ADD COLUMN theme TEXT NOT NULL DEFAULT 'auto'`)
	s.db.Exec(`ALTER TABLE users ADD COLUMN email_verified INTEGER NOT NULL DEFAULT 0`)

	// Seed the default subscription row if it doesn't exist.
	s.db.Exec(`INSERT OR IGNORE INTO subscriptions (id) VALUES ('account')`)

	// M26: global keyword blocklist table.
	s.db.Exec(`CREATE TABLE IF NOT EXISTS blocked_terms (
		id         TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
		term       TEXT NOT NULL UNIQUE,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)

	// M33: account-level Turnstile keys
	s.db.Exec(`ALTER TABLE subscriptions ADD COLUMN turnstile_site_key TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE subscriptions ADD COLUMN turnstile_secret_key TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE user_identities ADD COLUMN username TEXT NOT NULL DEFAULT ''`)

	return nil
}

// ---- Comments ---------------------------------------------------------------

func (s *sqlStore) GetComment(id string) (*models.Comment, error) {
	row := s.db.QueryRow(`
		SELECT id, site_id, page_id, page_url, page_title, parent_id,
		       user_id, content, status, imported, disqus_author, created_at, updated_at
		FROM comments WHERE id = ?`, id)
	return scanComment(row)
}

func (s *sqlStore) ListComments(siteID, pageID string, page, pageSize int) ([]*models.Comment, int, error) {
	offset := (page - 1) * pageSize

	var total int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM comments WHERE site_id = ? AND page_id = ? AND status = 'approved'`,
		siteID, pageID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count comments: %w", err)
	}

	rows, err := s.db.Query(`
		SELECT c.id, c.site_id, c.page_id, c.page_url, c.page_title, c.parent_id,
		       c.user_id, c.content, c.status, c.imported, c.disqus_author, c.created_at, c.updated_at,
		       u.display_name, u.avatar_url
		FROM comments c
		LEFT JOIN users u ON c.user_id = u.id
		WHERE c.site_id = ? AND c.page_id = ? AND c.status = 'approved'
		ORDER BY c.created_at ASC
		LIMIT ? OFFSET ?`,
		siteID, pageID, pageSize, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()

	comments := make([]*models.Comment, 0, pageSize)
	for rows.Next() {
		c, err := scanCommentWithAuthor(rows)
		if err != nil {
			return nil, 0, err
		}
		comments = append(comments, c)
	}
	return comments, total, rows.Err()
}

func (s *sqlStore) ListAdminComments(siteID, status string, page, pageSize int) ([]*models.Comment, int, error) {
	offset := (page - 1) * pageSize

	var (
		total int
		args  []interface{}
		where string
	)

	if siteID != "" && status != "" {
		where = "WHERE site_id = ? AND status = ?"
		args = []interface{}{siteID, status, pageSize, offset}
	} else if siteID != "" {
		where = "WHERE site_id = ?"
		args = []interface{}{siteID, pageSize, offset}
	} else if status != "" {
		where = "WHERE status = ?"
		args = []interface{}{status, pageSize, offset}
	} else {
		args = []interface{}{pageSize, offset}
	}

	countArgs := args[:len(args)-2]
	err := s.db.QueryRow(
		fmt.Sprintf(`SELECT COUNT(*) FROM comments %s`, where),
		countArgs...,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count admin comments: %w", err)
	}

	rows, err := s.db.Query(
		fmt.Sprintf(`
			SELECT id, site_id, page_id, page_url, page_title, parent_id,
			       user_id, content, status, imported, disqus_author, created_at, updated_at
			FROM comments %s
			ORDER BY created_at DESC
			LIMIT ? OFFSET ?`, where),
		args...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list admin comments: %w", err)
	}
	defer rows.Close()

	return scanComments(rows, total, pageSize)
}

func (s *sqlStore) CreateComment(c *models.Comment) error {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now

	_, err := s.db.Exec(`
		INSERT INTO comments
		  (id, site_id, page_id, page_url, page_title, parent_id, user_id, content, status, imported, disqus_author, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.SiteID, c.PageID, c.PageURL, c.PageTitle,
		nullStr(c.ParentID), c.UserID, c.Content, c.Status,
		boolInt(c.Imported), nullStr(c.DisqusAuthor),
		c.CreatedAt, c.UpdatedAt,
	)
	return err
}

func (s *sqlStore) UpdateComment(c *models.Comment) error {
	c.UpdatedAt = time.Now().UTC()
	_, err := s.db.Exec(`
		UPDATE comments SET content = ?, status = ?, updated_at = ?
		WHERE id = ?`,
		c.Content, c.Status, c.UpdatedAt, c.ID,
	)
	return err
}

func (s *sqlStore) DeleteComment(id string) error {
	_, err := s.db.Exec(`DELETE FROM comments WHERE id = ?`, id)
	return err
}

func (s *sqlStore) CountApprovedCommentsByUser(userID, siteID string) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM comments
		WHERE user_id = ? AND site_id = ? AND status = 'approved'`,
		userID, siteID,
	).Scan(&count)
	return count, err
}

func (s *sqlStore) ImportComments(siteID string, comments []*models.Comment) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO comments
		  (id, site_id, page_id, page_url, page_title, parent_id, user_id, content, status, imported, disqus_author, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	inserted := 0
	for _, c := range comments {
		if c.ID == "" {
			c.ID = uuid.NewString()
		}
		c.SiteID = siteID
		if c.CreatedAt.IsZero() {
			c.CreatedAt = time.Now().UTC()
		}
		c.UpdatedAt = c.CreatedAt

		res, err := stmt.Exec(
			c.ID, c.SiteID, c.PageID, c.PageURL, c.PageTitle,
			nullStr(c.ParentID), c.UserID, c.Content, c.Status,
			boolInt(c.Imported), nullStr(c.DisqusAuthor),
			c.CreatedAt, c.UpdatedAt,
		)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		inserted += int(n)
	}

	return inserted, tx.Commit()
}

func (s *sqlStore) ExportComments(siteID string, filter ExportFilter) ([]*models.Comment, error) {
	var sb strings.Builder
	sb.WriteString(`SELECT c.id, c.site_id, c.page_id, c.page_url, c.page_title, c.parent_id,
		c.user_id, c.content, c.status, c.imported, c.disqus_author, c.created_at, c.updated_at,
		COALESCE(u.display_name, c.disqus_author, '') AS author_name,
		COALESCE(u.avatar_url, '') AS author_avatar
		FROM comments c LEFT JOIN users u ON c.user_id = u.id
		WHERE c.site_id = ?`)
	args := []interface{}{siteID}

	if filter.Status != "all" {
		sb.WriteString(` AND c.status = 'approved'`)
	}
	if filter.From != nil {
		sb.WriteString(` AND c.created_at >= ?`)
		args = append(args, filter.From.UTC().Format("2006-01-02 15:04:05"))
	}
	if filter.To != nil {
		sb.WriteString(` AND c.created_at <= ?`)
		args = append(args, filter.To.UTC().Format("2006-01-02 15:04:05"))
	}
	if filter.PageID != "" {
		sb.WriteString(` AND c.page_id = ?`)
		args = append(args, filter.PageID)
	}
	sb.WriteString(` ORDER BY c.created_at ASC`)

	rows, err := s.db.Query(sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("export comments: %w", err)
	}
	defer rows.Close()

	var comments []*models.Comment
	for rows.Next() {
		c, err := scanCommentWithAuthor(rows)
		if err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

// ---- Users ------------------------------------------------------------------

func (s *sqlStore) GetUser(id string) (*models.User, error) {
	row := s.db.QueryRow(`
		SELECT id, display_name, email, avatar_url, role, banned, email_verified, created_at
		FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func (s *sqlStore) UpsertUser(u *models.User) error {
	if u.ID == "" {
		u.ID = uuid.NewString()
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	if u.Role == "" {
		u.Role = "commenter"
	}

	_, err := s.db.Exec(`
		INSERT INTO users (id, display_name, email, avatar_url, role, banned, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  display_name = excluded.display_name,
		  email        = excluded.email,
		  avatar_url   = excluded.avatar_url`,
		u.ID, u.DisplayName, nullStr(u.Email), nullStr(u.AvatarURL),
		u.Role, boolInt(u.Banned), u.CreatedAt,
	)
	return err
}

func (s *sqlStore) UpdateUser(u *models.User) error {
	_, err := s.db.Exec(`
		UPDATE users SET display_name = ?, email = ?, avatar_url = ?, role = ?, banned = ?
		WHERE id = ?`,
		u.DisplayName, nullStr(u.Email), nullStr(u.AvatarURL),
		u.Role, boolInt(u.Banned), u.ID,
	)
	return err
}

func (s *sqlStore) ListUsers(page, pageSize int) ([]*models.User, int, error) {
	offset := (page - 1) * pageSize

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(`
		SELECT id, display_name, email, avatar_url, role, banned, email_verified, created_at
		FROM users ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		pageSize, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	users := make([]*models.User, 0, pageSize)
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

// ---- Identities -------------------------------------------------------------

func (s *sqlStore) GetIdentity(provider, providerID string) (*models.UserIdentity, error) {
	row := s.db.QueryRow(`
		SELECT id, user_id, provider, provider_id, password_hash
		FROM user_identities WHERE provider = ? AND provider_id = ?`,
		provider, providerID,
	)

	var (
		id, userID, prov, provID string
		hash                     sql.NullString
	)
	err := row.Scan(&id, &userID, &prov, &provID, &hash)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &models.UserIdentity{
		ID:           id,
		UserID:       userID,
		Provider:     prov,
		ProviderID:   provID,
		PasswordHash: hash.String,
	}, nil
}

func (s *sqlStore) CreateIdentity(identity *models.UserIdentity) error {
	if identity.ID == "" {
		identity.ID = uuid.NewString()
	}
	_, err := s.db.Exec(`
		INSERT INTO user_identities (id, user_id, provider, provider_id, password_hash, username)
		VALUES (?, ?, ?, ?, ?, ?)`,
		identity.ID, identity.UserID, identity.Provider,
		identity.ProviderID, nullStr(identity.PasswordHash), identity.Username,
	)
	return err
}

func (s *sqlStore) UpdateIdentityPassword(identityID, hash string) error {
	_, err := s.db.Exec(`
		UPDATE user_identities SET password_hash = ? WHERE id = ?`,
		hash, identityID,
	)
	return err
}

// ---- Sites ------------------------------------------------------------------

func (s *sqlStore) GetSite(id string) (*models.Site, error) {
	row := s.db.QueryRow(`
		SELECT id, owner_id, domain, theme, created_at, last_notified_at FROM sites WHERE id = ?`, id)
	return scanSite(row)
}

func (s *sqlStore) ListSites() ([]*models.Site, error) {
	rows, err := s.db.Query(`SELECT id, owner_id, domain, theme, created_at, last_notified_at FROM sites ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sites []*models.Site
	for rows.Next() {
		site, err := scanSite(rows)
		if err != nil {
			return nil, err
		}
		sites = append(sites, site)
	}
	return sites, rows.Err()
}

func (s *sqlStore) CountSites() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sites`).Scan(&n)
	return n, err
}

func (s *sqlStore) UpdateSiteLastNotifiedAt(siteID string, t time.Time) error {
	_, err := s.db.Exec(`UPDATE sites SET last_notified_at = ? WHERE id = ?`, t.UTC(), siteID)
	return err
}

func (s *sqlStore) CountPendingComments(siteID string) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM comments WHERE site_id = ? AND status = 'pending'`, siteID,
	).Scan(&count)
	return count, err
}

func (s *sqlStore) ListPendingComments(siteID string) ([]*models.Comment, error) {
	rows, err := s.db.Query(`
		SELECT c.id, c.site_id, c.page_id, c.page_url, c.page_title, c.parent_id,
		       c.user_id, c.content, c.status, c.imported, c.disqus_author, c.created_at, c.updated_at,
		       u.display_name, u.avatar_url
		FROM comments c
		LEFT JOIN users u ON c.user_id = u.id
		WHERE c.site_id = ? AND c.status = 'pending'
		ORDER BY c.created_at ASC`, siteID)
	if err != nil {
		return nil, fmt.Errorf("list pending comments: %w", err)
	}
	defer rows.Close()

	var comments []*models.Comment
	for rows.Next() {
		c, err := scanCommentWithAuthor(rows)
		if err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

func (s *sqlStore) CreateSite(site *models.Site) error {
	if site.ID == "" {
		site.ID = uuid.NewString()
	}
	if site.CreatedAt.IsZero() {
		site.CreatedAt = time.Now().UTC()
	}
	if site.Theme == "" {
		site.Theme = "auto"
	}
	_, err := s.db.Exec(`
		INSERT INTO sites (id, owner_id, domain, theme, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		site.ID, site.OwnerID, site.Domain, site.Theme, site.CreatedAt,
	)
	return err
}

func (s *sqlStore) UpdateSite(site *models.Site) error {
	_, err := s.db.Exec(`UPDATE sites SET theme = ? WHERE id = ?`, site.Theme, site.ID)
	return err
}

// ---- Approval tokens --------------------------------------------------------

func (s *sqlStore) GetApprovalToken(token string) (*models.ApprovalToken, error) {
	row := s.db.QueryRow(`
		SELECT token, comment_id, expires_at FROM approval_tokens WHERE token = ?`, token)

	at := &models.ApprovalToken{}
	err := row.Scan(&at.Token, &at.CommentID, &at.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return at, err
}

func (s *sqlStore) CreateApprovalToken(t *models.ApprovalToken) error {
	_, err := s.db.Exec(`
		INSERT INTO approval_tokens (token, comment_id, expires_at)
		VALUES (?, ?, ?)`,
		t.Token, t.CommentID, t.ExpiresAt,
	)
	return err
}

func (s *sqlStore) DeleteApprovalToken(token string) error {
	_, err := s.db.Exec(`DELETE FROM approval_tokens WHERE token = ?`, token)
	return err
}

// ---- Email tokens -----------------------------------------------------------

func (s *sqlStore) CreateEmailToken(t *models.EmailToken) error {
	_, err := s.db.Exec(`
		INSERT INTO email_tokens (token, user_id, type, expires_at)
		VALUES (?, ?, ?, ?)`,
		t.Token, t.UserID, t.Type, t.ExpiresAt,
	)
	return err
}

func (s *sqlStore) GetEmailToken(token string) (*models.EmailToken, error) {
	row := s.db.QueryRow(`
		SELECT token, user_id, type, expires_at FROM email_tokens WHERE token = ?`, token)

	t := &models.EmailToken{}
	err := row.Scan(&t.Token, &t.UserID, &t.Type, &t.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

func (s *sqlStore) DeleteEmailToken(token string) error {
	_, err := s.db.Exec(`DELETE FROM email_tokens WHERE token = ?`, token)
	return err
}

func (s *sqlStore) SetEmailVerified(userID string) error {
	_, err := s.db.Exec(`UPDATE users SET email_verified = 1 WHERE id = ?`, userID)
	return err
}

func (s *sqlStore) UpdatePasswordHashByUser(userID, provider, hash string) error {
	_, err := s.db.Exec(`
		UPDATE user_identities SET password_hash = ?
		WHERE user_id = ? AND provider = ?`,
		hash, userID, provider,
	)
	return err
}

// ---- Scan helpers -----------------------------------------------------------

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanComment(s scanner) (*models.Comment, error) {
	var (
		c                      models.Comment
		pageURL, pageTitle     sql.NullString
		parentID, disqusAuthor sql.NullString
		imported               int
	)
	err := s.Scan(
		&c.ID, &c.SiteID, &c.PageID, &pageURL, &pageTitle,
		&parentID, &c.UserID, &c.Content, &c.Status,
		&imported, &disqusAuthor, &c.CreatedAt, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	c.PageURL = pageURL.String
	c.PageTitle = pageTitle.String
	c.ParentID = parentID.String
	c.DisqusAuthor = disqusAuthor.String
	c.Imported = imported != 0

	return &c, nil
}

func scanCommentWithAuthor(s scanner) (*models.Comment, error) {
	var (
		c                        models.Comment
		pageURL, pageTitle       sql.NullString
		parentID, disqusAuthor   sql.NullString
		imported                 int
		authorName, authorAvatar sql.NullString
	)
	err := s.Scan(
		&c.ID, &c.SiteID, &c.PageID, &pageURL, &pageTitle,
		&parentID, &c.UserID, &c.Content, &c.Status,
		&imported, &disqusAuthor, &c.CreatedAt, &c.UpdatedAt,
		&authorName, &authorAvatar,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	c.PageURL = pageURL.String
	c.PageTitle = pageTitle.String
	c.ParentID = parentID.String
	c.DisqusAuthor = disqusAuthor.String
	c.Imported = imported != 0
	c.AuthorName = authorName.String
	c.AuthorAvatar = authorAvatar.String

	return &c, nil
}

func scanComments(rows *sql.Rows, total, pageSize int) ([]*models.Comment, int, error) {
	comments := make([]*models.Comment, 0, pageSize)
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, 0, err
		}
		comments = append(comments, c)
	}
	return comments, total, rows.Err()
}

func scanSite(s scanner) (*models.Site, error) {
	var (
		site           models.Site
		lastNotifiedAt sql.NullTime
	)
	err := s.Scan(&site.ID, &site.OwnerID, &site.Domain, &site.Theme, &site.CreatedAt, &lastNotifiedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if lastNotifiedAt.Valid {
		site.LastNotifiedAt = &lastNotifiedAt.Time
	}
	return &site, nil
}

func scanUser(s scanner) (*models.User, error) {
	var (
		u             models.User
		email         sql.NullString
		avatar        sql.NullString
		banned        int
		emailVerified int
	)
	err := s.Scan(&u.ID, &u.DisplayName, &email, &avatar, &u.Role, &banned, &emailVerified, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.Email = email.String
	u.AvatarURL = avatar.String
	u.Banned = banned != 0
	u.EmailVerified = emailVerified != 0
	return &u, nil
}

// ---- Utility ----------------------------------------------------------------

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---- Subscription -----------------------------------------------------------

func (s *sqlStore) GetSubscription() (*models.Subscription, error) {
	row := s.db.QueryRow(`
		SELECT stripe_customer_id, stripe_sub_id, plan, status, interval,
		       trial_ends_at, current_period_end, updated_at
		FROM subscriptions WHERE id = 'account'`)

	var (
		sub              models.Subscription
		trialEndsAt      sql.NullTime
		currentPeriodEnd sql.NullTime
	)
	err := row.Scan(
		&sub.StripeCustomerID, &sub.StripeSubID, &sub.Plan, &sub.Status, &sub.Interval,
		&trialEndsAt, &currentPeriodEnd, &sub.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return &models.Subscription{Plan: "hobby", Status: "active"}, nil
	}
	if err != nil {
		return nil, err
	}
	if trialEndsAt.Valid {
		sub.TrialEndsAt = &trialEndsAt.Time
	}
	if currentPeriodEnd.Valid {
		sub.CurrentPeriodEnd = &currentPeriodEnd.Time
	}
	return &sub, nil
}

func (s *sqlStore) UpsertSubscription(sub *models.Subscription) error {
	sub.UpdatedAt = time.Now().UTC()
	_, err := s.db.Exec(`
		INSERT INTO subscriptions
		  (id, stripe_customer_id, stripe_sub_id, plan, status, interval,
		   trial_ends_at, current_period_end, updated_at)
		VALUES ('account', ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  stripe_customer_id = excluded.stripe_customer_id,
		  stripe_sub_id      = excluded.stripe_sub_id,
		  plan               = excluded.plan,
		  status             = excluded.status,
		  interval           = excluded.interval,
		  trial_ends_at      = excluded.trial_ends_at,
		  current_period_end = excluded.current_period_end,
		  updated_at         = excluded.updated_at`,
		sub.StripeCustomerID, sub.StripeSubID, sub.Plan, sub.Status, sub.Interval,
		nullTime(sub.TrialEndsAt), nullTime(sub.CurrentPeriodEnd), sub.UpdatedAt,
	)
	return err
}

func (s *sqlStore) GetAnalytics(siteID string, from time.Time, limit int, tier int) (*models.AnalyticsResult, error) {
	result := &models.AnalyticsResult{
		Volume:     []models.VolumePoint{},
		Pages:      []models.PageStat{},
		Commenters: []models.CommenterStat{},
	}

	// siteFilter builds a WHERE fragment and args for optional site + time filtering.
	// siteID="" means all sites (business aggregate).
	siteFilter := func(tablePrefix string) (string, []interface{}) {
		var clauses []string
		var args []interface{}
		col := "site_id"
		if tablePrefix != "" {
			col = tablePrefix + ".site_id"
		}
		if siteID != "" {
			clauses = append(clauses, col+" = ?")
			args = append(args, siteID)
		}
		ts := "created_at"
		if tablePrefix != "" {
			ts = tablePrefix + ".created_at"
		}
		if !from.IsZero() {
			clauses = append(clauses, ts+" >= ?")
			args = append(args, from)
		}
		if len(clauses) == 0 {
			return "", args
		}
		return "AND " + strings.Join(clauses, " AND "), args
	}

	// --- Volume over time ---------------------------------------------------
	// modernc.org/sqlite stores time.Time as Go's .String() format
	// ("2006-01-02 15:04:05.999999999 +0000 UTC"), which SQLite's strftime
	// cannot parse. substr(created_at, 1, 10) reliably extracts YYYY-MM-DD.
	sf, sfArgs := siteFilter("")
	volQuery := `SELECT substr(created_at, 1, 10) AS date, COUNT(*) AS count
		FROM comments WHERE status = 'approved' ` + sf + ` GROUP BY date ORDER BY date ASC`
	rows, err := s.db.Query(volQuery, sfArgs...)
	if err != nil {
		return nil, fmt.Errorf("analytics volume: %w", err)
	}
	dateMap := make(map[string]int)
	var minDate string
	for rows.Next() {
		var date string
		var count int
		if err := rows.Scan(&date, &count); err != nil {
			rows.Close()
			return nil, fmt.Errorf("analytics volume scan: %w", err)
		}
		dateMap[date] = count
		if minDate == "" {
			minDate = date
		}
	}
	rows.Close()

	start := from
	if from.IsZero() && minDate != "" {
		start, _ = time.Parse("2006-01-02", minDate)
	}
	if !start.IsZero() {
		today := time.Now().UTC()
		for d := start; !d.After(today); d = d.AddDate(0, 0, 1) {
			ds := d.Format("2006-01-02")
			result.Volume = append(result.Volume, models.VolumePoint{Date: ds, Count: dateMap[ds]})
		}
	}

	// --- Top pages ----------------------------------------------------------
	sf, sfArgs = siteFilter("")
	pageQuery := `SELECT page_id, COALESCE(NULLIF(MAX(page_title), ''), page_id) AS page_title, COUNT(*) AS count
		FROM comments WHERE status = 'approved' ` + sf + ` GROUP BY page_id ORDER BY count DESC LIMIT ?`
	pageRows, err := s.db.Query(pageQuery, append(sfArgs, limit)...)
	if err != nil {
		return nil, fmt.Errorf("analytics pages: %w", err)
	}
	defer pageRows.Close()
	for pageRows.Next() {
		var p models.PageStat
		if err := pageRows.Scan(&p.PageID, &p.PageTitle, &p.Count); err != nil {
			return nil, fmt.Errorf("analytics pages scan: %w", err)
		}
		result.Pages = append(result.Pages, p)
	}

	// --- Top commenters -----------------------------------------------------
	sf, sfArgs = siteFilter("c")
	cQuery := `SELECT COALESCE(NULLIF(u.display_name, ''), c.disqus_author, 'Anonymous') AS display_name, COUNT(*) AS count
		FROM comments c LEFT JOIN users u ON c.user_id = u.id
		WHERE c.status = 'approved' ` + sf + ` GROUP BY c.user_id ORDER BY count DESC LIMIT ?`
	cRows, err := s.db.Query(cQuery, append(sfArgs, limit)...)
	if err != nil {
		return nil, fmt.Errorf("analytics commenters: %w", err)
	}
	defer cRows.Close()
	for cRows.Next() {
		var c models.CommenterStat
		if err := cRows.Scan(&c.DisplayName, &c.Count); err != nil {
			return nil, fmt.Errorf("analytics commenters scan: %w", err)
		}
		result.Commenters = append(result.Commenters, c)
	}

	if tier < 1 {
		return result, nil
	}

	// --- Pro+: Status breakdown ---------------------------------------------
	sf, sfArgs = siteFilter("")
	sbRows, err := s.db.Query(
		`SELECT status, COUNT(*) AS count FROM comments WHERE 1=1 `+sf+` GROUP BY status`,
		sfArgs...,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics status: %w", err)
	}
	defer sbRows.Close()
	result.StatusBreakdown = []models.StatusStat{}
	for sbRows.Next() {
		var ss models.StatusStat
		if err := sbRows.Scan(&ss.Status, &ss.Count); err != nil {
			return nil, fmt.Errorf("analytics status scan: %w", err)
		}
		result.StatusBreakdown = append(result.StatusBreakdown, ss)
	}

	// --- Pro+: Peak activity by hour ----------------------------------------
	sf, sfArgs = siteFilter("")
	hourRows, err := s.db.Query(
		`SELECT CAST(substr(created_at, 12, 2) AS INTEGER) AS hour, COUNT(*) AS count
		 FROM comments WHERE status = 'approved' `+sf+` GROUP BY hour ORDER BY hour ASC`,
		sfArgs...,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics peak hours: %w", err)
	}
	defer hourRows.Close()
	hourMap := make(map[int]int, 24)
	for hourRows.Next() {
		var hour, count int
		if err := hourRows.Scan(&hour, &count); err != nil {
			return nil, fmt.Errorf("analytics peak hours scan: %w", err)
		}
		hourMap[hour] = count
	}
	for h := 0; h < 24; h++ {
		result.PeakHours = append(result.PeakHours, models.PeakHourStat{Hour: h, Count: hourMap[h]})
	}

	// --- Pro+: Peak activity by day of week ---------------------------------
	// strftime('%w', ...) works once we pass a clean YYYY-MM-DD string.
	sf, sfArgs = siteFilter("")
	dayRows, err := s.db.Query(
		`SELECT CAST(strftime('%w', substr(created_at, 1, 10)) AS INTEGER) AS dow, COUNT(*) AS count
		 FROM comments WHERE status = 'approved' `+sf+` GROUP BY dow ORDER BY dow ASC`,
		sfArgs...,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics peak days: %w", err)
	}
	defer dayRows.Close()
	dayMap := make(map[int]int, 7)
	for dayRows.Next() {
		var dow, count int
		if err := dayRows.Scan(&dow, &count); err != nil {
			return nil, fmt.Errorf("analytics peak days scan: %w", err)
		}
		dayMap[dow] = count
	}
	for d := 0; d < 7; d++ {
		result.PeakDays = append(result.PeakDays, models.PeakDayStat{Day: d, Count: dayMap[d]})
	}

	if tier < 2 {
		return result, nil
	}

	// --- Business+: Return commenter rate -----------------------------------
	sf, sfArgs = siteFilter("c")
	var totalCommenters, returningCommenters int
	if err := s.db.QueryRow(
		`SELECT COUNT(DISTINCT c.user_id) FROM comments c WHERE c.status = 'approved' `+sf,
		sfArgs...,
	).Scan(&totalCommenters); err != nil {
		return nil, fmt.Errorf("analytics total commenters: %w", err)
	}
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM (
			SELECT c.user_id FROM comments c WHERE c.status = 'approved' `+sf+`
			GROUP BY c.user_id HAVING COUNT(*) > 1
		)`,
		sfArgs...,
	).Scan(&returningCommenters); err != nil {
		return nil, fmt.Errorf("analytics returning commenters: %w", err)
	}
	var rate float64
	if totalCommenters > 0 {
		rate = float64(returningCommenters) / float64(totalCommenters) * 100
	}
	result.ReturnRate = &rate

	return result, nil
}

// ---- Blocked terms ----------------------------------------------------------

func (s *sqlStore) ListBlockedTerms() ([]*models.BlockedTerm, error) {
	rows, err := s.db.Query(`SELECT id, term, created_at FROM blocked_terms ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.BlockedTerm
	for rows.Next() {
		var t models.BlockedTerm
		if err := rows.Scan(&t.ID, &t.Term, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

func (s *sqlStore) AddBlockedTerm(term string) (*models.BlockedTerm, error) {
	t := &models.BlockedTerm{Term: term, CreatedAt: time.Now().UTC()}
	_, err := s.db.Exec(
		`INSERT INTO blocked_terms (term) VALUES (?) ON CONFLICT(term) DO NOTHING`,
		term,
	)
	if err != nil {
		return nil, err
	}
	// Fetch back to get the generated id (may be pre-existing if conflict).
	row := s.db.QueryRow(`SELECT id, term, created_at FROM blocked_terms WHERE term = ?`, term)
	if err := row.Scan(&t.ID, &t.Term, &t.CreatedAt); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *sqlStore) DeleteBlockedTerm(id string) error {
	_, err := s.db.Exec(`DELETE FROM blocked_terms WHERE id = ?`, id)
	return err
}

func (s *sqlStore) BulkAddBlockedTerms(terms []string) (added int, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO blocked_terms (term) VALUES (?) ON CONFLICT(term) DO NOTHING`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	for _, term := range terms {
		res, execErr := stmt.Exec(term)
		if execErr != nil {
			return added, execErr
		}
		n, _ := res.RowsAffected()
		added += int(n)
	}
	return added, tx.Commit()
}

func (s *sqlStore) CountCommentsThisMonth() (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM comments
		WHERE created_at >= strftime('%Y-%m-01', 'now')`).Scan(&count)
	return count, err
}

func nullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// ---- User identities (account management) -----------------------------------

func (s *sqlStore) ListUserIdentities(userID string) ([]*models.UserIdentity, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, provider, provider_id, COALESCE(username,'') FROM user_identities WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.UserIdentity
	for rows.Next() {
		i := &models.UserIdentity{}
		if err := rows.Scan(&i.ID, &i.UserID, &i.Provider, &i.ProviderID, &i.Username); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *sqlStore) GetIdentityByUser(userID, provider string) (*models.UserIdentity, error) {
	row := s.db.QueryRow(
		`SELECT id, user_id, provider, provider_id, COALESCE(password_hash,'') FROM user_identities WHERE user_id = ? AND provider = ?`,
		userID, provider)
	i := &models.UserIdentity{}
	if err := row.Scan(&i.ID, &i.UserID, &i.Provider, &i.ProviderID, &i.PasswordHash); err != nil {
		return nil, err
	}
	return i, nil
}

func (s *sqlStore) DeleteUserIdentity(userID, provider string) error {
	_, err := s.db.Exec(
		`DELETE FROM user_identities WHERE user_id = ? AND provider = ?`, userID, provider)
	return err
}

func (s *sqlStore) UpdateUserDisplayName(userID, displayName string) error {
	_, err := s.db.Exec(`UPDATE users SET display_name = ? WHERE id = ?`, displayName, userID)
	return err
}

func (s *sqlStore) UpdateIdentityUsername(userID, provider, username string) error {
	_, err := s.db.Exec(
		`UPDATE user_identities SET username = ? WHERE user_id = ? AND provider = ?`,
		username, userID, provider)
	return err
}

// ---- Turnstile keys ---------------------------------------------------------

func (s *sqlStore) GetTurnstileKeys() (siteKey, secretKey string, err error) {
	err = s.db.QueryRow(
		`SELECT COALESCE(turnstile_site_key,''), COALESCE(turnstile_secret_key,'') FROM subscriptions WHERE id = 'account'`,
	).Scan(&siteKey, &secretKey)
	return
}

func (s *sqlStore) SetTurnstileKeys(siteKey, secretKey string) error {
	_, err := s.db.Exec(
		`UPDATE subscriptions SET turnstile_site_key = ?, turnstile_secret_key = ? WHERE id = 'account'`,
		siteKey, secretKey)
	return err
}
