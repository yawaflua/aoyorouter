package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api"
	"github.com/yawaflua/aoyorouter/pkg/cursor"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// cursorOAuthFlow tracks an in-flight Cursor deep-link login.
type cursorOAuthFlow struct {
	state      string
	providerID string
	loginURL   string
	flow       *cursor.LoginFlow
}

var (
	cursorOAuthMu    sync.Mutex
	cursorOAuthFlows = map[string]*cursorOAuthFlow{}
)


func (a *AoyoRouterService) createCursorProviderAuthorization(ctx context.Context, req *aoyorouter.CreateProviderAuthorizationRequest) (*aoyorouter.CreateProviderAuthorizationResponse, error) {
	name := req.GetName()
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "provider name is required")
	}

	customURL := req.GetCustomUrl()
	if customURL == "" {
		customURL = "https://api2.cursor.sh/"
	}

	provider, err := a.ProviderRepo.CreateProvider(ctx, name, int32(req.GetType()), customURL, "oauth:pending", req.GetUseProxy(), req.GetProxy(), req.GetProxy() == "", req.GetPriority())
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if req.GetUseProxy() {
		proxyURL = req.GetProxy()
	}
	flow, err := cursor.NewLoginFlow(proxyURL)
	if err != nil {
		_ = a.ProviderRepo.DeleteProvider(ctx, provider.ID)
		return nil, status.Errorf(codes.Internal, "cursor login init: %v", err)
	}

	state := uuid.NewString()
	api.RegisterOAuthSession(state, provider.ID)

	cursorOAuthMu.Lock()
	cursorOAuthFlows[state] = &cursorOAuthFlow{
		state:      state,
		providerID: provider.ID,
		loginURL:   flow.LoginURL,
		flow:       flow,
	}
	cursorOAuthMu.Unlock()

	return &aoyorouter.CreateProviderAuthorizationResponse{
		AuthorizationUrl: flow.LoginURL,
		State:            state,
		ProviderId:       provider.ID,
		Flow: "device",
	}, nil
}


func (a *AoyoRouterService) cursorProviderAuthorizationStatus(ctx context.Context, state string) (*aoyorouter.ProviderAuthorizationStatusResponse, error) {
	cursorOAuthMu.Lock()
	flow, ok := cursorOAuthFlows[state]
	cursorOAuthMu.Unlock()
	if !ok {
		return nil, status.Error(codes.NotFound, "authorization session was not found or expired")
	}

	pollCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	result, done, err := flow.flow.PollOnce(pollCtx)
	if err != nil {
		api.SetOAuthSessionError(state, err.Error())
		cursorOAuthMu.Lock()
		delete(cursorOAuthFlows, state)
		cursorOAuthMu.Unlock()
		return &aoyorouter.ProviderAuthorizationStatusResponse{Status: "error", ProviderId: flow.providerID, Error: err.Error()}, nil
	}
	if !done {
		return &aoyorouter.ProviderAuthorizationStatusResponse{Status: providerOAuthPending, ProviderId: flow.providerID}, nil
	}

	credentials := map[string]any{
		"cookie":       result.Cookie,
		"access_token": result.AccessToken,
		"auth_id":      result.AuthID,
	}
	provider, err := a.ProviderRepo.UpdateProviderCredentials(ctx, flow.providerID, result.Cookie, credentials)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to store cursor credentials: %v", err)
	}

	if err := a.addProvider(provider, ctx); err != nil {
		return nil, fmt.Errorf("failed to register cursor provider: %w", err)
	}

	api.CompleteOAuthSession(state)
	cursorOAuthMu.Lock()
	delete(cursorOAuthFlows, state)
	cursorOAuthMu.Unlock()
	return &aoyorouter.ProviderAuthorizationStatusResponse{Status: "ok", ProviderId: flow.providerID}, nil
}
