package models

import "time"

type Provider struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Type         int32          `json:"type"`
	BaseUrl     string         `json:"client_id"`
	ClientSecret string         `json:"client_secret"`
	Credentials  map[string]any `json:"credentials"`
	UseProxy     bool           `json:"use_proxy"`
	Proxy        string         `json:"proxy"`
	IsCloudflare bool           `json:"is_cloudflare"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}
