package models

// SiteDigestMaterialization is the tenant-local, read-only input assembled
// for a claimed site digest. It contains only the parent identity, site
// metadata, and comments explicitly mapped to that parent.
type SiteDigestMaterialization struct {
	Outbox   *NotificationOutbox `json:"outbox"`
	Site     *Site               `json:"site"`
	Comments []*Comment          `json:"comments"`
}
