package models

import (
	"time"

	"github.com/google/uuid"
)

type UsageEntry struct {
	ID         int64     `json:"id"`
	ApiTokenID uuid.UUID `json:"api_token"`

	Provider string `json:"provider"`
	Latency  int    `json:"latency"`

	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
	CachedTokens int64 `json:"cached_tokens"`

	Model     string `json:"model"`
	Reasoning string `json:"reasoning"`
	Failed    bool   `json:"failed"`
	Error     string `json:"error,omitempty"`

	RequestedAt time.Time `json:"requested_at"`
	CreatedAt   time.Time `json:"created_at"`
}
