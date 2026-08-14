package models

import "time"

type ApiKey struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	IsDeleted bool      `json:"is_deleted"`
	IsActive  bool      `json:"is_active"`
}
