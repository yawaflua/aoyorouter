package models

import (
	"fmt"
	"strings"
	"time"
)

type PushKeys struct {
	P256dh string `json:"p256dh"`
	Auth   string `json:"auth"`
}

type PushSubscription struct {
	ID             string            `json:"id"`
	Endpoint       string            `json:"endpoint"`
	Keys           PushKeys          `json:"keys"`
	ExpirationTime *int64            `json:"expiration_time"`
	UserAgent      string            `json:"user_agent"`
	Labels         map[string]string `json:"labels"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type NotificationEvent struct {
	ID         int64     `json:"id"`
	Subject    string    `json:"subject"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	Tag        string    `json:"tag"`
	ProviderID string    `json:"provider_id"`
	URL        string    `json:"url"`
	CreatedAt  time.Time `json:"created_at"`
}

type VapidKeys struct {
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

func (s *PushSubscription) Validate() error {
	if strings.TrimSpace(s.Endpoint) == "" {
		return fmt.Errorf("push subscription endpoint is required")
	}
	if IsInAppEndpoint(s.Endpoint) {
		return nil
	}
	if strings.TrimSpace(s.Keys.P256dh) == "" || strings.TrimSpace(s.Keys.Auth) == "" {
		return fmt.Errorf("push subscription keys are required")
	}
	return nil
}

const InAppEndpointPrefix = "inapp:"

func IsInAppEndpoint(endpoint string) bool {
	return strings.HasPrefix(strings.TrimSpace(endpoint), InAppEndpointPrefix)
}

const quotaTopicPrefix = "provider-quota:"

func QuotaTopic(providerID string) string {
	return quotaTopicPrefix + providerID
}

func ProviderIDFromQuotaTopic(subject string) (string, bool) {
	id, ok := strings.CutPrefix(subject, quotaTopicPrefix)
	if !ok || id == "" {
		return "", false
	}
	return id, true
}
