CREATE TABLE approval_tokens (
    token         TEXT PRIMARY KEY,
    comment_id    TEXT NOT NULL,
    expires_at    DATETIME NOT NULL,
    FOREIGN KEY (comment_id) REFERENCES comments(id)
);
CREATE TABLE blocked_terms (
		id         TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
		term       TEXT NOT NULL UNIQUE,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
CREATE TABLE comment_flags (
    id         TEXT PRIMARY KEY,
    comment_id TEXT NOT NULL,
    user_id    TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(comment_id, user_id),
    FOREIGN KEY (comment_id) REFERENCES comments(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id)    REFERENCES users(id)    ON DELETE CASCADE
);
CREATE TABLE comment_votes (
    id         TEXT PRIMARY KEY,
    comment_id TEXT NOT NULL,
    user_id    TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(comment_id, user_id),
    FOREIGN KEY (comment_id) REFERENCES comments(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id)    REFERENCES users(id)    ON DELETE CASCADE
);
CREATE TABLE comments (
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
CREATE TABLE email_tokens (
    token      TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL,
    type       TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id)
);
CREATE TABLE goose_db_version (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		version_id INTEGER NOT NULL,
		is_applied INTEGER NOT NULL,
		tstamp TIMESTAMP DEFAULT (datetime('now'))
	);
CREATE TABLE sites (
    id          TEXT PRIMARY KEY,
    owner_id    TEXT NOT NULL,
    domain      TEXT NOT NULL,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
, last_notified_at DATETIME, theme TEXT NOT NULL DEFAULT 'auto', notify_interval INTEGER);
CREATE TABLE subscriptions (
    id                  TEXT PRIMARY KEY DEFAULT 'account',
    stripe_customer_id  TEXT NOT NULL DEFAULT '',
    stripe_sub_id       TEXT NOT NULL DEFAULT '',
    plan                TEXT NOT NULL DEFAULT 'hobby',
    status              TEXT NOT NULL DEFAULT 'active',
    interval            TEXT NOT NULL DEFAULT '',
    trial_ends_at       DATETIME,
    current_period_end  DATETIME,
    updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP
, turnstile_site_key TEXT NOT NULL DEFAULT '', turnstile_secret_key TEXT NOT NULL DEFAULT '');
CREATE TABLE user_identities (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL,
    provider      TEXT NOT NULL,
    provider_id   TEXT NOT NULL,
    password_hash TEXT, username TEXT NOT NULL DEFAULT '',
    UNIQUE(provider, provider_id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);
CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    display_name  TEXT NOT NULL,
    email         TEXT,
    avatar_url    TEXT,
    role          TEXT DEFAULT 'commenter',
    banned        INTEGER DEFAULT 0,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
, email_verified INTEGER NOT NULL DEFAULT 0);
INSERT INTO goose_db_version(version_id,is_applied) VALUES (0,1),(1,1);
