package cliproxyapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/yawaflua/aoyorouter/internal/models"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
)

type ProviderCredentialStore struct {
	repo providerCredentialRepository
}

type providerCredentialRepository interface {
	GetProviders(context.Context) ([]*models.Provider, error)
	GetProvider(context.Context, string) (*models.Provider, error)
	UpdateProviderCredentials(context.Context, string, string, map[string]any) (*models.Provider, error)
}

func NewProviderCredentialStore(repo providerCredentialRepository) *ProviderCredentialStore {
	return &ProviderCredentialStore{repo: repo}
}

func (s *ProviderCredentialStore) List(ctx context.Context) ([]*coreauth.Auth, error) {
	providers, err := s.repo.GetProviders(ctx)
	if err != nil {
		return nil, err
	}
	auths := make([]*coreauth.Auth, 0, len(providers))
	for _, provider := range providers {
		if provider == nil || len(provider.Credentials) == 0 {
			continue
		}
		if provider.Disabled {
			continue
		}
		credentialType := providerCredentialType(provider)
		if credentialType == "" {
			continue
		}
		var proxyUrl string
		if provider.UseProxy && provider.Proxy != "" {
			proxyUrl = provider.Proxy
		}
		credentials := cloneCredentials(provider.Credentials)
		credentials["type"] = credentialType
		credentials["provider_id"] = provider.ID
		credentials["label"] = provider.Name
		auths = append(auths, &coreauth.Auth{
			ID:       provider.ID,
			Provider: credentialType,
			Label:    provider.Name,
			Status:   coreauth.StatusActive,
			Attributes: map[string]string{
				coreauth.AttributeSourceBackend: coreauth.AuthSourcePostgres,
				"priority":                      strconv.Itoa(provider.Priority),
			},
			Metadata:  credentials,
			CreatedAt: provider.CreatedAt,
			UpdatedAt: provider.UpdatedAt,
			ProxyURL:  proxyUrl,
			Disabled:  provider.Disabled,
		})
	}
	return auths, nil
}

func providerCredentialType(provider *models.Provider) string {
	if provider == nil {
		return ""
	}
	if credentialType, _ := provider.Credentials["type"].(string); strings.TrimSpace(credentialType) != "" {
		return strings.ToLower(strings.TrimSpace(credentialType))
	}

	switch provider.Type {
	case aoyorouter.ProviderType_PROVIDER_TYPE_OPENAI:
		return "codex"
	case aoyorouter.ProviderType_PROVIDER_TYPE_ANTHROPIC:
		return "claude"
	case aoyorouter.ProviderType_PROVIDER_TYPE_KIMI:
		return "kimi"
	case aoyorouter.ProviderType_PROVIDER_TYPE_GROK:
		return "xai"
	case aoyorouter.ProviderType_PROVIDER_TYPE_ANTIGRAVITY:
		return "antigravity"
	default:
		return ""
	}
}

func (s *ProviderCredentialStore) Save(ctx context.Context, auth *coreauth.Auth) (string, error) {
	if auth == nil || strings.TrimSpace(auth.ID) == "" {
		return "", fmt.Errorf("provider credential store: auth ID is required")
	}
	provider, err := s.repo.GetProvider(ctx, auth.ID)
	if err != nil {
		return "", fmt.Errorf("provider credential store: get provider: %w", err)
	}
	credentials, err := authCredentials(auth)
	if err != nil {
		return "", fmt.Errorf("provider credential store: encode credentials: %w", err)
	}
	credentials["type"] = auth.Provider
	credentials["label"] = provider.Name
	credentials["provider_id"] = provider.ID
	credentials["last_refresh"] = time.Now().UTC().Format(time.RFC3339)
	clientSecret := provider.ClientSecret
	if clientSecret == "oauth:pending" {
		clientSecret = "oauth:database"
	}
	if _, err := s.repo.UpdateProviderCredentials(ctx, provider.ID, clientSecret, credentials); err != nil {
		return "", fmt.Errorf("provider credential store: save credentials: %w", err)
	}
	return provider.ID, nil
}

func (s *ProviderCredentialStore) Delete(ctx context.Context, id string) error {
	provider, err := s.repo.GetProvider(ctx, id)
	if err != nil {
		return fmt.Errorf("provider credential store: get provider: %w", err)
	}
	_, err = s.repo.UpdateProviderCredentials(ctx, provider.ID, provider.ClientSecret, map[string]any{})
	return err
}

func cloneCredentials(credentials map[string]any) map[string]any {
	cloned := make(map[string]any, len(credentials))
	for key, value := range credentials {
		cloned[key] = value
	}
	return cloned
}

func authCredentials(auth *coreauth.Auth) (map[string]any, error) {
	credentials := make(map[string]any)
	if auth.Storage != nil {
		data, err := json.Marshal(auth.Storage)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &credentials); err != nil {
			return nil, err
		}
	}
	for key, value := range auth.Metadata {
		credentials[key] = value
	}
	return credentials, nil
}
