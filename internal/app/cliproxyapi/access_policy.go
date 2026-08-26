package cliproxyapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	cliproxysession "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/session"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres/provider_repo"
)

type accessPolicy struct {
	restrictedProviders []string
	restrictedModels    []string
}

// providerFingerprintTTL bounds how long a resolved provider fingerprint is
// reused. Set() runs on every authenticate, and without this it issued one
// GetProvider query per restricted provider per request.
const providerFingerprintTTL = 30 * time.Second

type AccessPolicyStore struct {
	policies             sync.Map
	providerFingerprints sync.Map
	providerRepo         *provider_repo.ProviderRepo

	// resolvedAt records when each provider ID was last looked up.
	resolvedAt sync.Map
}

type RestrictedSelector struct {
	policies *AccessPolicyStore
	next     coreauth.Selector
}


func NewAccessPolicyStore(providerRepo ...*provider_repo.ProviderRepo) *AccessPolicyStore {
	store := &AccessPolicyStore{}
	if len(providerRepo) > 0 {
		store.providerRepo = providerRepo[0]
	}
	return store
}

func (s *AccessPolicyStore) Set(ctx context.Context, apiKeyID string, providers, models []string) {
	if s == nil {
		return
	}
	scope := cliproxysession.CallerScope(apiKeyID)
	if scope == "" {
		return
	}
	policy := accessPolicy{
		restrictedProviders: normalizeRestrictions(providers),
		restrictedModels:    normalizeRestrictions(models),
	}
	s.resolveProviderFingerprints(ctx, policy.restrictedProviders)
	if len(policy.restrictedProviders) == 0 && len(policy.restrictedModels) == 0 {
		s.policies.Delete(scope)
		return
	}
	s.policies.Store(scope, policy)
}

func (s *AccessPolicyStore) resolveProviderFingerprints(ctx context.Context, providerIDs []string) {
	if s == nil || s.providerRepo == nil {
		return
	}
	now := time.Now()
	for _, providerID := range providerIDs {
		if _, err := uuid.Parse(providerID); err != nil {
			continue
		}
		if last, ok := s.resolvedAt.Load(providerID); ok {
			if t, ok := last.(time.Time); ok && now.Sub(t) < providerFingerprintTTL {
				continue
			}
		}
		s.resolvedAt.Store(providerID, now)

		provider, err := s.providerRepo.GetProvider(ctx, providerID)
		if err != nil || provider == nil || strings.HasPrefix(provider.ClientSecret, "oauth:") {
			s.providerFingerprints.Delete(providerID)
			continue
		}
		s.providerFingerprints.Store(providerID, credentialFingerprint(provider.ClientSecret, provider.BaseUrl))
	}
}

func (s *AccessPolicyStore) get(scope string) (accessPolicy, bool) {
	if s == nil || strings.TrimSpace(scope) == "" {
		return accessPolicy{}, false
	}
	value, ok := s.policies.Load(strings.TrimSpace(scope))
	if !ok {
		return accessPolicy{}, false
	}
	policy, ok := value.(accessPolicy)
	return policy, ok
}

func NewRestrictedSelector(policies *AccessPolicyStore, next coreauth.Selector) *RestrictedSelector {
	if next == nil {
		next = &coreauth.RoundRobinSelector{}
	}
	return &RestrictedSelector{policies: policies, next: next}
}

func (s *RestrictedSelector) Pick(ctx context.Context, provider, model string, opts coreexecutor.Options, auths []*coreauth.Auth) (*coreauth.Auth, error) {
	policy, ok := s.policy(opts)
	if !ok {
		return s.next.Pick(ctx, provider, model, opts, auths)
	}

	requestedModel := metadataString(opts.Metadata, coreexecutor.RequestedModelMetadataKey)
	if restrictedMatch(policy.restrictedModels, model) || restrictedMatch(policy.restrictedModels, requestedModel) {
		blockedModel := strings.TrimSpace(requestedModel)
		if blockedModel == "" {
			blockedModel = strings.TrimSpace(model)
		}
		return nil, coreauth.NewRequestScopedError(fmt.Sprintf("access to model %q is forbidden", blockedModel), http.StatusForbidden)
	}

	allowed := make([]*coreauth.Auth, 0, len(auths))
	for _, candidate := range auths {
		if candidate == nil || s.providerRestricted(policy.restrictedProviders, candidate) {
			continue
		}
		allowed = append(allowed, candidate)
	}
	if len(allowed) == 0 && len(auths) > 0 {
		return nil, coreauth.NewRequestScopedError("access to the requested provider is forbidden", http.StatusForbidden)
	}
	return s.next.Pick(ctx, provider, model, opts, allowed)
}

func (s *RestrictedSelector) policy(opts coreexecutor.Options) (accessPolicy, bool) {
	if s == nil || s.policies == nil {
		return accessPolicy{}, false
	}
	return s.policies.get(metadataString(opts.Metadata, coreexecutor.CallerScopeMetadataKey))
}

func (s *RestrictedSelector) providerRestricted(restrictions []string, candidate *coreauth.Auth) bool {
	if candidate == nil {
		return false
	}
	if restrictedMatch(restrictions, candidate.ID) || restrictedMatch(restrictions, candidate.Provider) {
		return true
	}
	if providerTypeRestricted(restrictions, candidate.Provider) {
		return true
	}
	if restrictedMatch(restrictions, metadataString(candidate.Metadata, "provider_id")) {
		return true
	}
	apiKey := metadataString(candidate.Metadata, "api_key")
	if apiKey == "" && candidate.Attributes != nil {
		apiKey = strings.TrimSpace(candidate.Attributes["api_key"])
	}
	if apiKey == "" || s == nil || s.policies == nil {
		return false
	}
	baseURL := ""
	if candidate.Attributes != nil {
		baseURL = strings.TrimSpace(candidate.Attributes["base_url"])
	}
	fingerprint := credentialFingerprint(apiKey, baseURL)
	for _, restriction := range restrictions {
		stored, ok := s.policies.providerFingerprints.Load(restriction)
		if ok && stored == fingerprint {
			return true
		}
	}
	return false
}

func providerTypeRestricted(restrictions []string, provider string) bool {
	provider = normalizeRestriction(provider)
	for _, restriction := range restrictions {
		switch restriction {
		case "openai":
			if provider == "openai" || provider == "codex" {
				return true
			}
		case "anthropic":
			if provider == "anthropic" || provider == "claude" {
				return true
			}
		case "grok":
			if provider == "grok" || provider == "xai" {
				return true
			}
		case "kimi", "moonshot":
			if provider == "kimi" || provider == "moonshot" || provider == "openai-compatible-kimi" {
				return true
			}
		case "antigravity":
			if provider == "antigravity" || provider == "gemini" {
				return true
			}
		case "opencode-zen", "opencode_zen", "zen":
			if strings.Contains(provider, "opencode") && strings.Contains(provider, "zen") {
				return true
			}
		case "opencode-go", "opencode_go", "go":
			if strings.Contains(provider, "opencode") && (strings.HasSuffix(provider, "-go") || strings.HasSuffix(provider, "_go")) {
				return true
			}
		}
	}
	return false
}

func credentialFingerprint(values ...string) string {
	hashInput := strings.Builder{}
	for _, value := range values {
		hashInput.WriteByte(0)
		hashInput.WriteString(strings.TrimSpace(value))
	}
	sum := sha256.Sum256([]byte(hashInput.String()))
	return hex.EncodeToString(sum[:])
}

func restrictedMatch(restrictions []string, value string) bool {
	value = normalizeRestriction(value)
	if value == "" {
		return false
	}
	variants := []string{value}
	if withoutPrefix, ok := strings.CutPrefix(value, "models/"); ok {
		variants = append(variants, withoutPrefix)
	}
	for _, restriction := range restrictions {
		for _, variant := range variants {
			matched, err := path.Match(restriction, variant)
			if err != nil {
				// Fail closed. Treating ErrBadPattern as "no match" meant a
				// single malformed restriction silently granted access to
				// everything it was supposed to block.
				slog.Warn("access policy: malformed restriction pattern, denying",
					slog.String("pattern", restriction), slog.Any("err", err))
				return true
			}
			if matched || restriction == variant {
				return true
			}
		}
	}
	return false
}

func normalizeRestrictions(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = normalizeRestriction(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizeRestriction(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}
