package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api"
	"github.com/yawaflua/aoyorouter/internal/app/cliproxyapi"
	"github.com/yawaflua/aoyorouter/internal/models/providers"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const providerOAuthPending = "pending"

type providerOAuthSession struct {
	ProviderID         string
	Provider           string
	CredentialProvider string
	StartedAt          time.Time
	ExpiresAt          time.Time
	Completed          bool
}

type providerOAuthStore struct {
	mu sync.Mutex
}

func (a *AoyoRouterService) CreateProviderAuthorization(ctx context.Context, req *aoyorouter.CreateProviderAuthorizationRequest) (*aoyorouter.CreateProviderAuthorizationResponse, error) {
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "provider name is required")
	}
	definition, err := providers.ProviderOAuthConfig(req.GetType())
	if err != nil {
		return nil, err
	}
	customURL := strings.TrimSpace(req.GetCustomUrl())
	if customURL == "" {
		customURL = definition.GetOAuthDefinition().DefaultURL
	}

	provider, err := a.ProviderRepo.CreateProvider(ctx, name, int32(req.GetType()), customURL, "oauth:pending", req.GetUseProxy(), req.GetProxy(), req.GetProxy() == "")
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("provider_id", provider.ID)
	if definition.GetOAuthDefinition().Callback {
		query.Set("is_webui", "true")
	}
	var response cliproxyapi.ManagementAuthorization
	if err := a.Management.ManagementJSON(ctx, http.MethodGet, definition.GetOAuthDefinition().Endpoint, query, nil, &response); err != nil {
		_ = a.ProviderRepo.DeleteProvider(ctx, provider.ID)
		return nil, status.Errorf(codes.Internal, "%s authorization returned an incomplete response", definition.GetOAuthDefinition().Provider)
	}

	flow := response.Flow
	if flow == "" && definition.GetOAuthDefinition().Callback {
		flow = "callback"
	}
	return &aoyorouter.CreateProviderAuthorizationResponse{
		AuthorizationUrl: response.URL, State: response.State, ProviderId: provider.ID,
		Flow: flow, UserCode: response.UserCode, ExpiresIn: response.ExpiresIn,
	}, nil
}

func (a *AoyoRouterService) CompleteProviderAuthorization(ctx context.Context, req *aoyorouter.CompleteProviderAuthorizationRequest) (*aoyorouter.ProviderAuthorizationStatusResponse, error) {
	state := strings.TrimSpace(req.GetState())

	provider, _, ok := a.providerOAuthSession(state)
	if !ok {
		return nil, status.Error(codes.NotFound, "authorization session was not found or expired")
	}
	if ok {
		return &aoyorouter.ProviderAuthorizationStatusResponse{Status: "ok", ProviderId: provider}, nil
	}

	if completed, err := a.completeStoredProviderAuthorization(ctx, state, provider); err != nil {
		return nil, err
	} else if completed {
		return &aoyorouter.ProviderAuthorizationStatusResponse{Status: "ok", ProviderId: provider}, nil
	}
	if conf, err := providers.ProviderOAuthConfig(req.Type); err != nil {
		return nil, err
	} else if !conf.GetOAuthDefinition().Callback {
		return a.providerAuthorizationStatus(ctx, state, provider, req.GetUseProxy(), req.GetProxy())
	}

	callbackURL := strings.TrimSpace(req.GetCallbackUrl())
	if callbackURL == "" {
		return nil, status.Error(codes.InvalidArgument, "callback URL is required")
	}

	body := map[string]string{"provider": provider, "redirect_url": callbackURL}
	var response cliproxyapi.ManagementAuthorization
	if err := a.Management.ManagementJSON(ctx, http.MethodPost, "/v0/management/oauth-callback", nil, body, &response); err != nil {
		if completed, completionErr := a.completeStoredProviderAuthorization(ctx, state, provider); completionErr == nil && completed {
			return &aoyorouter.ProviderAuthorizationStatusResponse{Status: "ok", ProviderId: provider}, nil
		}
		return nil, status.Errorf(codes.InvalidArgument, "failed to submit authorization callback: %v", err)
	}
	return a.providerAuthorizationStatus(ctx, state, provider, req.GetUseProxy(), req.GetProxy())
}

func (a *AoyoRouterService) GetProviderAuthorizationStatus(ctx context.Context, req *aoyorouter.GetProviderAuthorizationStatusRequest) (*aoyorouter.ProviderAuthorizationStatusResponse, error) {
	state := strings.TrimSpace(req.GetState())
	provider, session, ok := a.providerOAuthSession(state)
	if !ok {
		return nil, status.Error(codes.NotFound, "authorization session was not found or expired")
	}
	if session != "" {
		return &aoyorouter.ProviderAuthorizationStatusResponse{Status: "error", ProviderId: provider, Error: session}, nil
	}
	if completed, err := a.completeStoredProviderAuthorization(ctx, state, provider); err != nil {
		return nil, err
	} else if completed {
		return &aoyorouter.ProviderAuthorizationStatusResponse{Status: "ok", ProviderId: provider}, nil
	}
	return a.providerAuthorizationStatus(ctx, state, session, req.GetUseProxy(), req.GetProxy())
}

func (a *AoyoRouterService) providerAuthorizationStatus(ctx context.Context, state string, session string, useProxy bool, proxyURL string) (*aoyorouter.ProviderAuthorizationStatusResponse, error) {
	provider, session, ok := api.GetOAuthSession(state)
	if !ok {
		return nil, status.Error(codes.NotFound, "authorization session was not found or expired")
	}

	switch session {
	case "wait", providerOAuthPending, "":
		return &aoyorouter.ProviderAuthorizationStatusResponse{Status: providerOAuthPending, ProviderId: provider}, nil
	case "error":
		a.cleanupProviderOAuth(ctx, state, session, "")
		return &aoyorouter.ProviderAuthorizationStatusResponse{Status: "error", ProviderId: provider, Error: session}, nil
	case "ok":
		completed, err := a.completeStoredProviderAuthorization(ctx, state, provider)
		if err != nil {
			return nil, err
		}
		if !completed {
			return nil, status.Error(codes.Internal, "authorization completed but credentials could not be found")
		}
		return &aoyorouter.ProviderAuthorizationStatusResponse{Status: "ok", ProviderId: provider}, nil
	default:
		return nil, status.Errorf(codes.Internal, "unexpected authorization status %q", session)
	}
}

func (a *AoyoRouterService) completeStoredProviderAuthorization(ctx context.Context, state string, provider string) (bool, error) {
	credentials, err := a.findProviderOAuthCredentials(ctx, provider)
	if err != nil {
		return false, nil
	}
	if !providerCredentialsCompleted(credentials) {
		return false, nil
	}
	if _, err := a.ProviderRepo.UpdateProviderCredentials(ctx, provider, "oauth:database", credentials); err != nil {
		return false, err
	}
	return true, nil
}

func providerCredentialsCompleted(credentials map[string]any) bool {
	accessToken, _ := credentials["access_token"].(string)
	return strings.TrimSpace(accessToken) != ""
}

func (a *AoyoRouterService) providerOAuthSession(state string) (string, string, bool) {
	return api.GetOAuthSession(state)
}

func (a *AoyoRouterService) findProviderOAuthCredentials(ctx context.Context, id string) (map[string]any, error) {
	provider, err := a.ProviderRepo.GetProvider(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(provider.Credentials) == 0 {
		return nil, fmt.Errorf("no %s credentials in database", provider.ID)
	}

	return provider.Credentials, nil
}

func (a *AoyoRouterService) cleanupProviderOAuth(ctx context.Context, state string, provider string, _ string) {
	_ = a.ProviderRepo.DeleteProvider(ctx, provider)
}
