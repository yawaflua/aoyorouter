package server

func anthropicOAuthDefinition() providerOAuthDefinition {
	return providerOAuthDefinition{
		Provider:           "anthropic",
		CredentialProvider: "claude",
		Endpoint:           "/v0/management/anthropic-auth-url",
		Callback:           true,
	}
}
