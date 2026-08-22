package providers

import (
	"context"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/yawaflua/aoyorouter/internal/models"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ProviderOAuthDefinition struct {
	Provider           string
	CredentialProvider string
	Endpoint           string
	DefaultURL         string
	Callback           bool
}


type ProviderConfig interface {
	LoadQuota(ctx context.Context, credentials map[string]any, useProxy bool, proxyURL string) *aoyorouter.ProviderQuota
	GetOAuthDefinition() *ProviderOAuthDefinition
	AddProviderConfig(ctx context.Context, cfg *config.Config, provider *models.Provider)
	RemoveProviderConfig(cfg *config.Config, provider *models.Provider)
}

func ProviderOAuthConfig(providerType aoyorouter.ProviderType) (ProviderConfig, error) {
	switch providerType {
	case aoyorouter.ProviderType_PROVIDER_TYPE_ANTHROPIC:
		return &AnthropicProvider{}, nil
	case aoyorouter.ProviderType_PROVIDER_TYPE_KIMI:
		return &KimiProvider{}, nil
	case aoyorouter.ProviderType_PROVIDER_TYPE_GROK:
		return &XAIProvider{}, nil
	case aoyorouter.ProviderType_PROVIDER_TYPE_ANTIGRAVITY:
		return &AntigravityProvider{}, nil
	case aoyorouter.ProviderType_PROVIDER_TYPE_OPENAI:
		return &CodexProvider{}, nil
	case aoyorouter.ProviderType_PROVIDER_TYPE_CUSTOM:
		return &CustomProvider{}, nil
	case aoyorouter.ProviderType_PROVIDER_TYPE_OPENCODE_ZEN:
		return &OpencodeZenProvider{}, nil
	case aoyorouter.ProviderType_PROVIDER_TYPE_OPENCODE_GO:
		return &OpencodeGoProvider{}, nil
	default:
		return nil, status.Error(codes.InvalidArgument, "provider does not support this authorization flow")
	}
}

func ProviderOAuthUsesCallback(provider ProviderConfig) bool {
	return provider.GetOAuthDefinition().Callback
}
