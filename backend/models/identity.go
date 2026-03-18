package models

type UserIdentity struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	Provider     string `json:"provider"`    // github | google | email
	ProviderID   string `json:"provider_id"` // provider-specific user ID or email
	Username     string `json:"username"`    // human-readable handle (e.g. GitHub login)
	PasswordHash string `json:"-"`           // only populated for email provider
}
