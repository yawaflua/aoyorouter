package server

import "testing"

func TestProviderOAuthReadyAcceptsStoredPendingCredentials(t *testing.T) {
	if !providerOAuthReady("oauth:pending", map[string]any{"access_token": "token"}) {
		t.Fatal("stored OAuth credentials must be ready even when legacy status is pending")
	}
	if providerOAuthReady("oauth:pending", map[string]any{}) {
		t.Fatal("empty pending credentials must not be ready")
	}
}
