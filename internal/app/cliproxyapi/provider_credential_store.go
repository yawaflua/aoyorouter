package cliproxyapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
	logger *slog.Logger
}

type providerCredentialRepository interface {
	GetProviders(context.Context) ([]*models.Provider, error)
	GetProvider(context.Context, string) (*models.Provider, error)
	UpdateProviderCredentials(context.Context, string, string, map[string]any) (*models.Provider, error)
}

func NewProviderCredentialStore(repo providerCredentialRepository, vendor *providers.ProviderVendor, logger *slog.Logger) *ProviderCredentialStore {
	return &ProviderCredentialStore{repo: repo, vendor: vendor, logger: logger}
}

func (s *ProviderCredentialStore) List(ctx context.Context) ([]*coreauth.Auth, error) {
	providers, err := s.repo.GetProviders(ctx)
	if err != nil {
		s.logger.Error("provider credential store: failed to list providers", "error", err)
		return nil, err
	}
	auths := make([]*coreauth.Auth, 0, len(providers))
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		if provider.Type == aoyorouter.ProviderType_PROVIDER_TYPE_CURSOR {
			continue
		}
		if provider.Disabled {
			s.logger.Debug("provider credential store: skipping disabled provider", "provider_id", provider.ID, "name", provider.Name)
			continue
		}
		if len(provider.Credentials) == 0 {
			if s.requiresCredentials(provider) {
				s.logger.Warn("provider credential store: provider has empty credentials", "provider_id", provider.ID, "name", provider.Name, "type", provider.Type, "client_secret", provider.ClientSecret)
			}
			continue
		}
		credentialType := s.providerCredentialType(provider)
		if credentialType == "" {
			s.logger.Warn("provider credential store: unknown credential type", "provider_id", provider.ID, "name", provider.Name, "type", provider.Type)
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
	s.logger.Info("provider credential store: listed credentials", "providers", len(providers), "auths", len(auths))
	return auths, nil
}

func (s *ProviderCredentialStore) requiresCredentials(provider *models.Provider) bool {
	if provider == nil {
		return false
	}
	cfg, err := s.vendor.ProviderOAuthConfig(provider.Type)
	if err != nil {
		return false
	}
	def := cfg.GetOAuthDefinition()
	return def != nil && def.CredentialProvider != ""
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
	if len(credentials) == 0 {
		s.logger.Warn("provider credential store: saving empty credentials", "provider_id", provider.ID, "name", provider.Name)
	}
	s.logger.Info("provider credential store: saving credentials", "provider_id", provider.ID, "name", provider.Name, "credential_type", auth.Provider, "keys", credentialKeys(credentials))
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
	s.logger.Info("provider credential store: deleting credentials", "provider_id", provider.ID, "name", provider.Name)
	_, err = s.repo.UpdateProviderCredentials(ctx, provider.ID, provider.ClientSecret, map[string]any{})
	return err
}

func credentialKeys(credentials map[string]any) []string {
	keys := make([]string, 0, len(credentials))
	for key := range credentials {
		keys = append(keys, key)
	}
	return keys
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
