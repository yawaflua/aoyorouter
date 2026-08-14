package models

type LogEntry struct {
	ID         string `json:"id"`
	ProviderID string `json:"provider_id"`
	Message    string `json:"message"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
