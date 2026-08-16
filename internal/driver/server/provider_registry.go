package server

import (
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type providerOAuthDefinition struct {
	Provider           string
	CredentialProvider string
	Endpoint           string
	DefaultURL         string
	Callback           bool
}

func providerOAuthConfig(providerType aoyorouter.ProviderType) (providerOAuthDefinition, error) {
	switch providerType {
	case aoyorouter.ProviderType_PROVIDER_TYPE_ANTHROPIC:
		return anthropicOAuthDefinition(), nil
	case aoyorouter.ProviderType_PROVIDER_TYPE_KIMI:
		return kimiOAuthDefinition(), nil
	case aoyorouter.ProviderType_PROVIDER_TYPE_GROK:
		return xaiOAuthDefinition(), nil
	case aoyorouter.ProviderType_PROVIDER_TYPE_ANTIGRAVITY:
		return antigravityOAuthDefinition(), nil
	default:
		return providerOAuthDefinition{}, status.Error(codes.InvalidArgument, "provider does not support this authorization flow")
	}
}

func providerOAuthUsesCallback(provider string) bool {
	return provider == anthropicOAuthDefinition().Provider || provider == antigravityOAuthDefinition().Provider
}
