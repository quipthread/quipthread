-- +goose Up
-- Baseline migration representing the full schema as of March 2026.
-- All tables include every column that was previously added via ALTER TABLE.
-- CREATE TABLE IF NOT EXISTS makes this safe to run against existing databases
-- (all statements are no-ops when tables already exist).

CREATE TABLE IF NOT EXISTS sites (
    id               TEXT PRIMARY KEY,
    owner_id         TEXT NOT NULL,
    domain           TEXT NOT NULL,
    theme            TEXT NOT NULL DEFAULT 'auto',
    last_notified_at DATETIME,
    notify_interval  INTEGER,
    created_at       DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS users (
    id             TEXT PRIMARY KEY,
    display_name   TEXT NOT NULL,
    email          TEXT,
    avatar_url     TEXT,
    role           TEXT DEFAULT 'commenter',
    banned         INTEGER DEFAULT 0,
    shadow_banned  INTEGER NOT NULL DEFAULT 0,
    email_verified INTEGER NOT NULL DEFAULT 0,
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_identities (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL,
    provider      TEXT NOT NULL,
    provider_id   TEXT NOT NULL,
    password_hash TEXT,
    username      TEXT NOT NULL DEFAULT '',
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
    upvotes       INTEGER NOT NULL DEFAULT 0,
    flags         INTEGER NOT NULL DEFAULT 0,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (site_id) REFERENCES sites(id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS approval_tokens (
    token      TEXT PRIMARY KEY,
    comment_id TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
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
    id                   TEXT PRIMARY KEY DEFAULT 'account',
    stripe_customer_id   TEXT NOT NULL DEFAULT '',
    stripe_sub_id        TEXT NOT NULL DEFAULT '',
    plan                 TEXT NOT NULL DEFAULT 'hobby',
    status               TEXT NOT NULL DEFAULT 'active',
    interval             TEXT NOT NULL DEFAULT '',
    trial_ends_at        DATETIME,
    current_period_end   DATETIME,
    turnstile_site_key   TEXT NOT NULL DEFAULT '',
    turnstile_secret_key TEXT NOT NULL DEFAULT '',
    updated_at           DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS blocked_terms (
    id         TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    term       TEXT NOT NULL UNIQUE,
    is_regex   INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS comment_votes (
    id         TEXT PRIMARY KEY,
    comment_id TEXT NOT NULL,
    user_id    TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(comment_id, user_id),
    FOREIGN KEY (comment_id) REFERENCES comments(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id)    REFERENCES users(id)    ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS comment_flags (
    id         TEXT PRIMARY KEY,
    comment_id TEXT NOT NULL,
    user_id    TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(comment_id, user_id),
    FOREIGN KEY (comment_id) REFERENCES comments(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id)    REFERENCES users(id)    ON DELETE CASCADE
);

INSERT OR IGNORE INTO subscriptions (id) VALUES ('account');

-- +goose Down
-- Rollback not supported: dropping these tables would destroy all data.
-- To roll back, restore a database backup taken before the migration.
