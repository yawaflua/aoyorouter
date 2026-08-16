package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

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
	mu       sync.Mutex
	sessions map[string]providerOAuthSession
}

type managementAuthorization struct {
	Status    string `json:"status"`
	URL       string `json:"url"`
	State     string `json:"state"`
	Flow      string `json:"flow"`
	UserCode  string `json:"user_code"`
	ExpiresIn int32  `json:"expires_in"`
	Error     string `json:"error"`
}

func newProviderOAuthStore() *providerOAuthStore {
	return &providerOAuthStore{sessions: make(map[string]providerOAuthSession)}
}

func (a *AoyoRouterService) CreateProviderAuthorization(ctx context.Context, req *aoyorouter.CreateProviderAuthorizationRequest) (*aoyorouter.CreateProviderAuthorizationResponse, error) {
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "provider name is required")
	}
	definition, err := providerOAuthConfig(req.GetType())
	if err != nil {
		return nil, err
	}
	customURL := strings.TrimSpace(req.GetCustomUrl())
	if customURL == "" {
		customURL = definition.DefaultURL
	}

	provider, err := a.ProviderRepo.CreateProvider(ctx, name, int32(req.GetType()), customURL, "oauth:pending")
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("provider_id", provider.ID)
	if definition.Callback {
		query.Set("is_webui", "true")
	}
	var response managementAuthorization
	if err := a.managementJSON(ctx, http.MethodGet, definition.Endpoint, query, nil, &response); err != nil {
		_ = a.ProviderRepo.DeleteProvider(ctx, provider.ID)
		return nil, status.Errorf(codes.Unavailable, "failed to start %s authorization: %v", definition.Provider, err)
	}
	if response.State == "" || response.URL == "" {
		_ = a.ProviderRepo.DeleteProvider(ctx, provider.ID)
		return nil, status.Errorf(codes.Internal, "%s authorization returned an incomplete response", definition.Provider)
	}

	now := time.Now()
	expiresAt := now.Add(10 * time.Minute)
	if response.ExpiresIn > 0 {
		expiresAt = now.Add(time.Duration(response.ExpiresIn) * time.Second)
	}
	a.ProviderOAuth.mu.Lock()
	for state, session := range a.ProviderOAuth.sessions {
		if now.After(session.ExpiresAt.Add(time.Minute)) {
			delete(a.ProviderOAuth.sessions, state)
		}
	}
	a.ProviderOAuth.sessions[response.State] = providerOAuthSession{
		ProviderID: provider.ID, Provider: definition.Provider, CredentialProvider: definition.CredentialProvider,
		StartedAt: now, ExpiresAt: expiresAt,
	}
	a.ProviderOAuth.mu.Unlock()

	flow := response.Flow
	if flow == "" && definition.Callback {
		flow = "callback"
	}
	return &aoyorouter.CreateProviderAuthorizationResponse{
		AuthorizationUrl: response.URL, State: response.State, ProviderId: provider.ID,
		Flow: flow, UserCode: response.UserCode, ExpiresIn: response.ExpiresIn,
	}, nil
}

func (a *AoyoRouterService) CompleteProviderAuthorization(ctx context.Context, req *aoyorouter.CompleteProviderAuthorizationRequest) (*aoyorouter.ProviderAuthorizationStatusResponse, error) {
	state := strings.TrimSpace(req.GetState())
	session, ok := a.providerOAuthSession(state)
	if !ok {
		return nil, status.Error(codes.NotFound, "authorization session was not found or expired")
	}
	if session.Completed {
		return providerAuthorizationOK(session), nil
	}
	if completed, err := a.completeStoredProviderAuthorization(ctx, state, session); err != nil {
		return nil, err
	} else if completed {
		return providerAuthorizationOK(session), nil
	}
	if !providerOAuthUsesCallback(session.Provider) {
		return a.providerAuthorizationStatus(ctx, state, session)
	}
	callbackURL := strings.TrimSpace(req.GetCallbackUrl())
	if callbackURL == "" {
		return nil, status.Error(codes.InvalidArgument, "callback URL is required")
	}
	body := map[string]string{"provider": session.Provider, "redirect_url": callbackURL}
	var response managementAuthorization
	if err := a.managementJSON(ctx, http.MethodPost, "/v0/management/oauth-callback", nil, body, &response); err != nil {
		if completed, completionErr := a.completeStoredProviderAuthorization(ctx, state, session); completionErr == nil && completed {
			return providerAuthorizationOK(session), nil
		}
		return nil, status.Errorf(codes.InvalidArgument, "failed to submit authorization callback: %v", err)
	}
	return a.providerAuthorizationStatus(ctx, state, session)
}

func (a *AoyoRouterService) GetProviderAuthorizationStatus(ctx context.Context, req *aoyorouter.GetProviderAuthorizationStatusRequest) (*aoyorouter.ProviderAuthorizationStatusResponse, error) {
	state := strings.TrimSpace(req.GetState())
	session, ok := a.providerOAuthSession(state)
	if !ok {
		return nil, status.Error(codes.NotFound, "authorization session was not found or expired")
	}
	if session.Completed {
		return providerAuthorizationOK(session), nil
	}
	if completed, err := a.completeStoredProviderAuthorization(ctx, state, session); err != nil {
		return nil, err
	} else if completed {
		return providerAuthorizationOK(session), nil
	}
	return a.providerAuthorizationStatus(ctx, state, session)
}

func (a *AoyoRouterService) providerAuthorizationStatus(ctx context.Context, state string, session providerOAuthSession) (*aoyorouter.ProviderAuthorizationStatusResponse, error) {
	var response managementAuthorization
	if err := a.managementJSON(ctx, http.MethodGet, "/v0/management/get-auth-status", url.Values{"state": {state}}, nil, &response); err != nil {
		return nil, status.Errorf(codes.Unavailable, "failed to check authorization status: %v", err)
	}
	switch response.Status {
	case "wait", providerOAuthPending, "":
		return &aoyorouter.ProviderAuthorizationStatusResponse{Status: providerOAuthPending, ProviderId: session.ProviderID}, nil
	case "error":
		a.cleanupProviderOAuth(ctx, state, session, "")
		return &aoyorouter.ProviderAuthorizationStatusResponse{Status: "error", ProviderId: session.ProviderID, Error: response.Error}, nil
	case "ok":
		completed, err := a.completeStoredProviderAuthorization(ctx, state, session)
		if err != nil {
			return nil, err
		}
		if !completed {
			return nil, status.Error(codes.Internal, "authorization completed but credentials could not be found")
		}
		return providerAuthorizationOK(session), nil
	default:
		return nil, status.Errorf(codes.Internal, "unexpected authorization status %q", response.Status)
	}
}

func (a *AoyoRouterService) completeStoredProviderAuthorization(ctx context.Context, state string, session providerOAuthSession) (bool, error) {
	credentials, err := a.findProviderOAuthCredentials(ctx, session)
	if err != nil {
		return false, nil
	}
	if !providerCredentialsCompleted(credentials) {
		return false, nil
	}
	if _, err := a.ProviderRepo.UpdateProviderCredentials(ctx, session.ProviderID, "oauth:database", credentials); err != nil {
		return false, err
	}
	a.ProviderOAuth.mu.Lock()
	session.Completed = true
	session.ExpiresAt = time.Now().Add(10 * time.Minute)
	a.ProviderOAuth.sessions[state] = session
	a.ProviderOAuth.mu.Unlock()
	return true, nil
}

func providerCredentialsCompleted(credentials map[string]any) bool {
	accessToken, _ := credentials["access_token"].(string)
	return strings.TrimSpace(accessToken) != ""
}

func providerAuthorizationOK(session providerOAuthSession) *aoyorouter.ProviderAuthorizationStatusResponse {
	return &aoyorouter.ProviderAuthorizationStatusResponse{Status: "ok", ProviderId: session.ProviderID}
}

func (a *AoyoRouterService) providerOAuthSession(state string) (providerOAuthSession, bool) {
	if state == "" {
		return providerOAuthSession{}, false
	}
	a.ProviderOAuth.mu.Lock()
	defer a.ProviderOAuth.mu.Unlock()
	session, ok := a.ProviderOAuth.sessions[state]
	if !ok || time.Now().After(session.ExpiresAt.Add(time.Minute)) {
		delete(a.ProviderOAuth.sessions, state)
		return providerOAuthSession{}, false
	}
	return session, true
}

func (a *AoyoRouterService) findProviderOAuthCredentials(ctx context.Context, session providerOAuthSession) (map[string]any, error) {
	provider, err := a.ProviderRepo.GetProvider(ctx, session.ProviderID)
	if err != nil {
		return nil, err
	}
	if len(provider.Credentials) == 0 {
		return nil, fmt.Errorf("no %s credentials in database", session.Provider)
	}
	credentialType, _ := provider.Credentials["type"].(string)
	if !strings.EqualFold(strings.TrimSpace(credentialType), session.CredentialProvider) {
		return nil, fmt.Errorf("unexpected credential type %q", credentialType)
	}
	return provider.Credentials, nil
}

func (a *AoyoRouterService) cleanupProviderOAuth(ctx context.Context, state string, session providerOAuthSession, _ string) {
	a.ProviderOAuth.mu.Lock()
	delete(a.ProviderOAuth.sessions, state)
	a.ProviderOAuth.mu.Unlock()
	_ = a.ProviderRepo.DeleteProvider(ctx, session.ProviderID)
}

func (a *AoyoRouterService) managementJSON(ctx context.Context, method, path string, query url.Values, body any, output any) error {
	endpoint := strings.TrimRight(a.CPAPIManagementURL, "/") + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+a.CPAPIManagementPassword)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var apiError struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &apiError)
		if apiError.Error != "" {
			return fmt.Errorf("%s", apiError.Error)
		}
		return fmt.Errorf("management API returned status %d", response.StatusCode)
	}
	if output != nil && len(data) > 0 {
		if err := json.Unmarshal(data, output); err != nil {
			return err
		}
	}
	return nil
}
