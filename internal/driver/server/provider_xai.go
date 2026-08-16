package server

func xaiOAuthDefinition() providerOAuthDefinition {
	return providerOAuthDefinition{
		Provider:           "xai",
		CredentialProvider: "xai",
		Endpoint:           "/v0/management/xai-auth-url",
		DefaultURL:         "https://api.x.ai/v1",
	}
}
