package models

import (
	"time"

	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
)

type Provider struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	Type    aoyorouter.ProviderType `json:"type"`
	BaseUrl string                  `json:"client_id"`

	ClientSecret string         `json:"client_secret"`
	Credentials  map[string]any `json:"credentials"`

	UseProxy     bool   `json:"use_proxy"`
	Proxy        string `json:"proxy"`
	IsCloudflare bool   `json:"is_cloudflare"`

	Priority  int       `json:"priority"`
	Disabled  bool      `json:"disabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
