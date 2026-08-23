package providers

import (
	"context"
	"log/slog"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/yawaflua/aoyorouter/internal/adapter/cursor"
	"github.com/yawaflua/aoyorouter/internal/adapter/warp"
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

type ProviderVendor struct {
	logger *slog.Logger
	warp   *warp.Warp
	cursor *cursor.CursorServer
}

func NewProviderVendor(logger *slog.Logger, warp *warp.Warp, cursor *cursor.CursorServer) *ProviderVendor {
	return &ProviderVendor{
		logger: logger,
		warp:   warp,
		cursor: cursor,
	}
}

type ProviderConfig interface {
	LoadQuota(ctx context.Context, credentials map[string]any, useProxy bool, proxyURL string) *aoyorouter.ProviderQuota
	GetOAuthDefinition() *ProviderOAuthDefinition
	AddProviderConfig(ctx context.Context, cfg *config.Config, provider *models.Provider)
	RemoveProviderConfig(cfg *config.Config, provider *models.Provider)
}

func (p *ProviderVendor) ProviderOAuthConfig(providerType aoyorouter.ProviderType) (ProviderConfig, error) {
	switch providerType {
	case aoyorouter.ProviderType_PROVIDER_TYPE_ANTHROPIC:
		return NewAnthropicProvider(p.logger), nil
	case aoyorouter.ProviderType_PROVIDER_TYPE_KIMI:
		return NewKimiProvider(p.logger), nil
	case aoyorouter.ProviderType_PROVIDER_TYPE_GROK:
		return NewXAIProvider(p.logger), nil
	case aoyorouter.ProviderType_PROVIDER_TYPE_ANTIGRAVITY:
		return NewAnthropicProvider(p.logger), nil
	case aoyorouter.ProviderType_PROVIDER_TYPE_OPENAI:
		return NewCodexProvider(p.logger), nil
	case aoyorouter.ProviderType_PROVIDER_TYPE_CUSTOM:
		return NewCustomProvider(p.logger), nil
	case aoyorouter.ProviderType_PROVIDER_TYPE_OPENCODE_ZEN:
		return NewOpencodeZenProvider(p.logger), nil
	case aoyorouter.ProviderType_PROVIDER_TYPE_OPENCODE_GO:
		return NewOpencodeGoProvider(p.logger), nil
	case aoyorouter.ProviderType_PROVIDER_TYPE_CLINE:
		return NewClineProvider(p.logger), nil
	case aoyorouter.ProviderType_PROVIDER_TYPE_CURSOR:
		return NewCursorProvider(p.logger, p.cursor), nil
	default:
		return nil, status.Error(codes.InvalidArgument, "provider is not supported")
	}
}

func ProviderOAuthUsesCallback(provider ProviderConfig) bool {
	return provider.GetOAuthDefinition().Callback
}
