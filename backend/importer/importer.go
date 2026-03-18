// Package importer converts external comment formats into Quipthread models.
// Each sub-parser returns a Result containing deduplicated synthetic users and
// the comments to be bulk-inserted via db.Store.ImportComments.
package importer

import (
	"net/url"
	"strings"

	"github.com/quipthread/quipthread/models"
)

// Result is the output of every importer.
type Result struct {
	// Users contains one record per unique author, keyed by deterministic ID.
	// Callers should UpsertUser for each before calling ImportComments.
	Users []*models.User
	// Comments are ready for ImportComments; SiteID is set by the caller.
	Comments []*models.Comment
}

// pageIDFromURL extracts the URL path, trimming a trailing slash.
// Returns "/" for empty or root paths.
func pageIDFromURL(rawURL string) string {
	if rawURL == "" {
		return "/"
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	path := strings.TrimRight(u.Path, "/")
	if path == "" {
		return "/"
	}
	return path
}

// syntheticUserID builds a deterministic, collision-resistant user ID for
// imported authors so UpsertUser is idempotent across repeated imports.
// Format: "<provider>-user:<identifier>", e.g. "disqus-user:john123".
func syntheticUserID(provider, identifier string) string {
	if identifier == "" {
		identifier = "anonymous"
	}
	return provider + "-user:" + identifier
}

// commentID returns the canonical Quipthread comment ID for an imported record.
// Using a stable, prefixed ID makes INSERT OR IGNORE dedup work correctly and
// allows parent references to be resolved without a separate lookup.
func commentID(provider, externalID string) string {
	return provider + ":" + externalID
}
