package models

import "time"

type Provider struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Type         int32     `json:"type"`
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
