package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/yawaflua/aoyorouter/internal/models"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
)

type XAIProvider struct {
}

var errInvalidXAIQuotaResponse = errors.New("invalid quota response")

const (
	xaiUserURL              = "https://cli-chat-proxy.grok.com/v1/user"
	xaiBillingURL           = "https://cli-chat-proxy.grok.com/v1/billing?format=credits"
	xaiQuotaResponseMaxSize = 64 << 10
	xaiClientVersion        = "0.2.120"
)

type xaiUserResponse struct {
	UserID string `json:"userId"`
}

type xaiCents struct {
	Value *float64 `json:"val"`
}

type xaiUsagePeriod struct {
	Type  string `json:"type"`
	Start string `json:"start"`
	End   string `json:"end"`
}

type xaiBillingConfig struct {
	CreditUsagePercent *float64        `json:"creditUsagePercent"`
	CurrentPeriod      *xaiUsagePeriod `json:"currentPeriod"`
	MonthlyLimit       *xaiCents       `json:"monthlyLimit"`
	Used               *xaiCents       `json:"used"`
	BillingPeriodStart string          `json:"billingPeriodStart"`
	BillingPeriodEnd   string          `json:"billingPeriodEnd"`
}

type xaiBillingResponse struct {
	Config           *xaiBillingConfig `json:"config"`
	SubscriptionTier string            `json:"subscriptionTier"`
}

// RemoveProviderConfig implements [ProviderConfig].
func (x *XAIProvider) RemoveProviderConfig(cfg *config.Config, provider *models.Provider) {
	for index, configured := range cfg.XAIKey {
		if configured.APIKey == provider.ClientSecret && configured.BaseURL == provider.BaseUrl {
			cfg.XAIKey = append(cfg.XAIKey[:index], cfg.XAIKey[index+1:]...)
			return
		}
	}
}

// AddProviderConfig implements [providers.ProviderConfig].
func (x *XAIProvider) AddProviderConfig(ctx context.Context, cfg *config.Config, provider *models.Provider) {
	if strings.HasPrefix(provider.ClientSecret, "oauth:") {
		return
	}

	cfg.XAIKey = append(cfg.XAIKey, config.XAIKey{APIKey: provider.ClientSecret, BaseURL: provider.BaseUrl, Prefix: "grok"})

}

// GetOAuthDefinition implements [providers.ProviderConfig].
func (x *XAIProvider) GetOAuthDefinition() *ProviderOAuthDefinition {
	return &ProviderOAuthDefinition{
		Provider:           "xai",
		CredentialProvider: "xai",
		Endpoint:           "/v0/management/xai-auth-url",
		DefaultURL:         "https://api.x.ai",
	}
}

// LoadQuota implements [providers.ProviderConfig].
func (x *XAIProvider) LoadQuota(ctx context.Context, credentials map[string]any, useProxy bool, proxyURL string) *aoyorouter.ProviderQuota {
	accessToken, _ := credentials["access_token"].(string)
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return &aoyorouter.ProviderQuota{Error: "xAI credentials are incomplete"}
	}

	client, err := proxyHTTPClient(useProxy, proxyURL)
	if err != nil {
		return &aoyorouter.ProviderQuota{Error: "quota request failed"}
	}
	quotaClient := *client
	quotaClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	var user xaiUserResponse
	statusCode, err := loadXAIQuotaJSON(ctx, &quotaClient, xaiUserURL, accessToken, "", &user)
	if err != nil {
		return xaiQuotaRequestError(statusCode, err)
	}
	if !validXAIUserID(user.UserID) {
		return &aoyorouter.ProviderQuota{Error: "invalid quota response"}
	}

	var billing xaiBillingResponse
	statusCode, err = loadXAIQuotaJSON(ctx, &quotaClient, xaiBillingURL, accessToken, user.UserID, &billing)
	if err != nil {
		return xaiQuotaRequestError(statusCode, err)
	}
	if billing.Config == nil {
		return &aoyorouter.ProviderQuota{Error: "quota unavailable"}
	}

	primary := xaiWeeklyQuotaWindow(billing.Config)
	secondary := xaiMonthlyQuotaWindow(billing.Config)
	if primary == nil {
		primary, secondary = secondary, nil
	}
	if primary == nil {
		return &aoyorouter.ProviderQuota{Error: "quota unavailable"}
	}

	planType := strings.TrimSpace(billing.SubscriptionTier)
	if planType == "" {
		planType = xaiPlanType(credentials)
	}
	return &aoyorouter.ProviderQuota{
		Quotas:   []*aoyorouter.ProviderQuotaWindow{primary, secondary},
		PlanType: planType,
	}
}

func loadXAIQuotaJSON(ctx context.Context, client *http.Client, endpoint, accessToken, userID string, target any) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-XAI-Token-Auth", "xai-grok-cli")
	request.Header.Set("x-grok-client-version", xaiClientVersion)
	request.Header.Set("x-grok-client-mode", "headless")
	if userID != "" {
		request.Header.Set("x-userid", userID)
	}

	response, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return response.StatusCode, fmt.Errorf("quota returned status %d", response.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(response.Body, xaiQuotaResponseMaxSize+1))
	if err != nil {
		return 0, err
	}
	if len(data) > xaiQuotaResponseMaxSize {
		return 0, errInvalidXAIQuotaResponse
	}
	if err := json.Unmarshal(data, target); err != nil {
		return 0, errInvalidXAIQuotaResponse
	}
	return response.StatusCode, nil
}

func xaiQuotaRequestError(statusCode int, err error) *aoyorouter.ProviderQuota {
	if statusCode != 0 {
		return &aoyorouter.ProviderQuota{Error: fmt.Sprintf("quota returned status %d", statusCode)}
	}
	if errors.Is(err, errInvalidXAIQuotaResponse) {
		return &aoyorouter.ProviderQuota{Error: "invalid quota response"}
	}
	return &aoyorouter.ProviderQuota{Error: "quota request failed"}
}

func validXAIUserID(userID string) bool {
	if userID == "" || len(userID) > 256 {
		return false
	}
	for index := range len(userID) {
		if userID[index] < 0x21 || userID[index] > 0x7e {
			return false
		}
	}
	return true
}

func xaiWeeklyQuotaWindow(config *xaiBillingConfig) *aoyorouter.ProviderQuotaWindow {
	if config == nil || config.CreditUsagePercent == nil || math.IsNaN(*config.CreditUsagePercent) || math.IsInf(*config.CreditUsagePercent, 0) {
		return nil
	}
	resetAt := config.BillingPeriodEnd
	windowMinutes := int32(7 * 24 * 60)
	if config.CurrentPeriod != nil {
		if strings.TrimSpace(config.CurrentPeriod.End) != "" {
			resetAt = config.CurrentPeriod.End
		}
		windowMinutes = xaiQuotaWindowMinutes(config.CurrentPeriod.Start, config.CurrentPeriod.End, windowMinutes)
	}
	return &aoyorouter.ProviderQuotaWindow{
		Name:          "Weekly",
		UsedPercent:   xaiUsedPercent(*config.CreditUsagePercent),
		ResetsAt:      resetAt,
		WindowMinutes: windowMinutes,
	}
}

func xaiMonthlyQuotaWindow(config *xaiBillingConfig) *aoyorouter.ProviderQuotaWindow {
	if config == nil || config.MonthlyLimit == nil || config.MonthlyLimit.Value == nil || config.Used == nil || config.Used.Value == nil {
		return nil
	}
	limit := *config.MonthlyLimit.Value
	used := *config.Used.Value
	if limit <= 0 || used < 0 || math.IsNaN(limit) || math.IsInf(limit, 0) || math.IsNaN(used) || math.IsInf(used, 0) {
		return nil
	}
	return &aoyorouter.ProviderQuotaWindow{
		Name:          "Monthly",
		UsedPercent:   xaiUsedPercent(used / limit * 100),
		ResetsAt:      config.BillingPeriodEnd,
		WindowMinutes: xaiQuotaWindowMinutes(config.BillingPeriodStart, config.BillingPeriodEnd, 30*24*60),
	}
}

func xaiUsedPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func xaiQuotaWindowMinutes(start, end string, fallback int32) int32 {
	startAt, startErr := time.Parse(time.RFC3339, start)
	endAt, endErr := time.Parse(time.RFC3339, end)
	if startErr != nil || endErr != nil || !endAt.After(startAt) {
		return fallback
	}
	minutes := endAt.Sub(startAt) / time.Minute
	if minutes <= 0 || minutes > time.Duration(math.MaxInt32) {
		return fallback
	}
	return int32(minutes)
}

func xaiPlanType(credentials map[string]any) string {
	for _, key := range []string{"subscription_tier", "subscription_type", "plan_type"} {
		if value, _ := credentials[key].(string); strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "Grok"
}
