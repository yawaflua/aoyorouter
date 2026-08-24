package cliproxyapi

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/yawaflua/aoyorouter/internal/models"
	"github.com/yawaflua/aoyorouter/internal/models/providers"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
)

type ProviderCredentialStore struct {
	repo   providerCredentialRepository
	vendor *providers.ProviderVendor
}

type providerCredentialRepository interface {
	GetProviders(context.Context) ([]*models.Provider, error)
	GetProvider(context.Context, string) (*models.Provider, error)
	UpdateProviderCredentials(context.Context, string, string, map[string]any) (*models.Provider, error)
}

func NewProviderCredentialStore(repo providerCredentialRepository, vendor *providers.ProviderVendor) *ProviderCredentialStore {
	return &ProviderCredentialStore{repo: repo, vendor: vendor}
}

func (s *ProviderCredentialStore) List(ctx context.Context) ([]*coreauth.Auth, error) {
	providers, err := s.repo.GetProviders(ctx)
	if err != nil {
		return nil, err
	}
	auths := make([]*coreauth.Auth, 0, len(providers))
	for _, provider := range providers {
		if provider.Type == aoyorouter.ProviderType_PROVIDER_TYPE_CURSOR {
			continue
		}
		if provider == nil || len(provider.Credentials) == 0 {
			continue
		}
		if provider.Disabled {
			continue
		}
		credentialType := s.providerCredentialType(provider)
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
		attributes := map[string]string{
			coreauth.AttributeSourceBackend: coreauth.AuthSourcePostgres,
			"priority":                      strconv.Itoa(provider.Priority),
		}

		auths = append(auths, &coreauth.Auth{
			ID:         provider.ID,
			Provider:   credentialType,
			Label:      provider.Name,
			Status:     coreauth.StatusActive,
			Attributes: attributes,
			Metadata:   credentials,
			CreatedAt:  provider.CreatedAt,
			UpdatedAt:  provider.UpdatedAt,
			ProxyURL:   proxyUrl,
			Disabled:   provider.Disabled,
			Prefix:     credentialType,
		})
	}
	return auths, nil
}

func (s *ProviderCredentialStore) providerCredentialType(provider *models.Provider) string {
	if provider == nil {
		return ""
	}

	cfg, err := s.vendor.ProviderOAuthConfig(provider.Type)
	if err != nil {
		return ""
	}
	return cfg.GetOAuthDefinition().CredentialProvider
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
	maps.Copy(cloned, credentials)
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
	maps.Copy(credentials, auth.Metadata)
	return credentials, nil
}
