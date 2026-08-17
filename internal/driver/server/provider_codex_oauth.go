package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	codexAuthorizationURL = "https://auth.openai.com/oauth/authorize"
	codexTokenURL         = "https://auth.openai.com/oauth/token"
	codexClientID         = "app_EMoamEEZ73f0CkXaXp7hrann" // Default codexClientID
	codexRedirectURI      = "http://localhost:1455/auth/callback"
)

type codexOAuthSession struct {
	Verifier   string
	ProviderID string
	ExpiresAt  time.Time
}

type codexOAuthStore struct {
	mu       sync.Mutex
	sessions map[string]codexOAuthSession
}

type codexTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
}

type codexClaims struct {
	Email string `json:"email"`
	Auth  struct {
		AccountID string `json:"chatgpt_account_id"`
		PlanType  string `json:"chatgpt_plan_type"`
	} `json:"https://api.openai.com/auth"`
}

func newCodexOAuthStore() *codexOAuthStore {
	return &codexOAuthStore{sessions: make(map[string]codexOAuthSession)}
}

func (a *AoyoRouterService) CreateCodexAuthorization(ctx context.Context, req *aoyorouter.CreateCodexAuthorizationRequest) (*aoyorouter.CreateCodexAuthorizationResponse, error) {
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "provider name is required")
	}
	state, err := randomURLSafe(32)

	if err != nil {
		return nil, status.Error(codes.Internal, "failed to create OAuth state")
	}
	verifier, err := randomURLSafe(64)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to create PKCE verifier")
	}
	challengeBytes := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes[:])

	provider, err := a.ProviderRepo.CreateProvider(ctx, name, int32(aoyorouter.ProviderType_PROVIDER_TYPE_OPENAI), strings.TrimSpace(req.GetCustomUrl()), "oauth:pending", req.GetUseProxy(), req.GetProxy())
	if err != nil {
		return nil, err
	}

	a.CodexOAuth.mu.Lock()
	now := time.Now()
	for key, session := range a.CodexOAuth.sessions {
		if now.After(session.ExpiresAt) {
			delete(a.CodexOAuth.sessions, key)
		}
	}
	a.CodexOAuth.sessions[state] = codexOAuthSession{Verifier: verifier, ProviderID: provider.ID, ExpiresAt: now.Add(10 * time.Minute)}
	a.CodexOAuth.mu.Unlock()

	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {codexClientID},
		"redirect_uri":          {codexRedirectURI},
		"scope":                 {"openid profile email offline_access api.connectors.read api.connectors.invoke"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
		"originator":            {"codex_cli_rs"},
	}

	return &aoyorouter.CreateCodexAuthorizationResponse{
		AuthorizationUrl: codexAuthorizationURL + "?" + params.Encode(),
		State:            state,
		ProviderId:       provider.ID,
	}, nil
}

func (a *AoyoRouterService) CompleteCodexAuthorization(ctx context.Context, req *aoyorouter.CompleteCodexAuthorizationRequest) (*aoyorouter.CompleteCodexAuthorizationResponse, error) {
	code, callbackState, err := parseCodexCallback(req.GetCallbackUrl())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	state := strings.TrimSpace(req.GetState())
	if callbackState != "" && callbackState != state {
		return nil, status.Error(codes.InvalidArgument, "OAuth state does not match")
	}

	a.CodexOAuth.mu.Lock()
	session, ok := a.CodexOAuth.sessions[state]
	if ok {
		delete(a.CodexOAuth.sessions, state)
	}
	a.CodexOAuth.mu.Unlock()
	if !ok || time.Now().After(session.ExpiresAt) {
		return nil, status.Error(codes.InvalidArgument, "OAuth session expired; generate a new authorization link")
	}
	cleanupPending := func() {
		_ = a.ProviderRepo.DeleteProvider(ctx, session.ProviderID)
	}
	provider, err := a.ProviderRepo.GetProvider(ctx, session.ProviderID)
	if err != nil {
		return nil, status.Error(codes.NotFound, "pending Codex provider was not found")
	}

	tokens, err := exchangeCodexCode(ctx, code, session.Verifier, provider.UseProxy, provider.Proxy)
	if err != nil {
		cleanupPending()
		return nil, status.Errorf(codes.Unauthenticated, "failed to exchange Codex authorization code: %v", err)
	}
	claims, err := parseCodexClaims(tokens.IDToken)
	if err != nil {
		cleanupPending()
		return nil, status.Errorf(codes.Unauthenticated, "failed to parse Codex identity token: %v", err)
	}
	if claims.Email == "" || claims.Auth.AccountID == "" {
		cleanupPending()
		return nil, status.Error(codes.Unauthenticated, "Codex token is missing account information")
	}

	credentials := codexCredentials(provider.Name, provider.ID, claims, tokens)
	if _, err := a.ProviderRepo.UpdateProviderCredentials(ctx, provider.ID, "oauth:database", credentials); err != nil {
		cleanupPending()
		return nil, err
	}

	return &aoyorouter.CompleteCodexAuthorizationResponse{Status: "ok", ProviderId: provider.ID}, nil
}

func randomURLSafe(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func parseCodexCallback(input string) (string, string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", "", fmt.Errorf("callback URL is required")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", "", fmt.Errorf("invalid callback URL")
	}
	if oauthError := parsed.Query().Get("error"); oauthError != "" {
		return "", "", fmt.Errorf("Codex authorization failed: %s", oauthError)
	}
	code := strings.TrimSpace(parsed.Query().Get("code"))
	if code == "" {
		return "", "", fmt.Errorf("callback URL does not contain an authorization code")
	}
	return code, strings.TrimSpace(parsed.Query().Get("state")), nil
}

func exchangeCodexCode(ctx context.Context, code, verifier string, useProxy bool, proxyURL string) (*codexTokenResponse, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {codexClientID},
		"code":          {code},
		"redirect_uri":  {codexRedirectURI},
		"code_verifier": {verifier},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, codexTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	client, err := proxyHTTPClient(useProxy, proxyURL)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var tokens codexTokenResponse
	if err := json.NewDecoder(response.Body).Decode(&tokens); err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI returned status %d", response.StatusCode)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" || tokens.IDToken == "" {
		return nil, fmt.Errorf("OpenAI returned incomplete credentials")
	}
	return &tokens, nil
}

func parseCodexClaims(idToken string) (*codexClaims, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var claims codexClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	return &claims, nil
}

func codexCredentials(label, providerID string, claims *codexClaims, tokens *codexTokenResponse) map[string]any {
	return map[string]any{
		"type":          "codex",
		"label":         label,
		"provider_id":   providerID,
		"id_token":      tokens.IDToken,
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"account_id":    claims.Auth.AccountID,
		"email":         claims.Email,
		"last_refresh":  time.Now().UTC().Format(time.RFC3339),
		"expired":       time.Now().UTC().Add(time.Duration(tokens.ExpiresIn) * time.Second).Format(time.RFC3339),
	}
}
