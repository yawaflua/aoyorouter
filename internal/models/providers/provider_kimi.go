package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/yawaflua/aoyorouter/internal/models"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
)

type KimiProvider struct {
	logger *slog.Logger
}

func NewKimiProvider(logger *slog.Logger) *KimiProvider {
	return &KimiProvider{
		logger: logger,
	}
}

// RemoveProviderConfig implements [ProviderConfig].
func (k *KimiProvider) RemoveProviderConfig(cfg *config.Config, provider *models.Provider) {
	for index, configured := range cfg.OpenAICompatibility {
		if configured.Name == provider.Name && configured.BaseURL == provider.BaseUrl && configured.APIKeyEntries[0].APIKey == provider.ClientSecret {
			cfg.OpenAICompatibility = append(cfg.OpenAICompatibility[:index], cfg.OpenAICompatibility[index+1:]...)
			return
		}
	}
}

// AddProviderConfig implements [providers.ProviderConfig].
func (k *KimiProvider) AddProviderConfig(ctx context.Context, cfg *config.Config, provider *models.Provider) {
	if strings.HasPrefix(provider.ClientSecret, "oauth:") {
		return
	}

	cfg.OpenAICompatibility = append(cfg.OpenAICompatibility, config.OpenAICompatibility{Name: "kimi", BaseURL: provider.BaseUrl, Prefix: "kimi", APIKeyEntries: []config.OpenAICompatibilityAPIKey{{APIKey: provider.ClientSecret}}})
}

// GetOAuthDefinition implements [providers.ProviderConfig].
func (k *KimiProvider) GetOAuthDefinition() *ProviderOAuthDefinition {
	return &ProviderOAuthDefinition{
		Provider:           "kimi",
		CredentialProvider: "kimi",
		Endpoint:           "/v0/management/kimi-auth-url",
		DefaultURL:         "https://api.kimi.com/coding",
	}
}

// LoadQuota implements [providers.ProviderConfig].
func (k *KimiProvider) LoadQuota(ctx context.Context, credentials map[string]any, useProxy bool, proxyURL string) *aoyorouter.ProviderQuota {

	return k.loadKimiQuota(ctx, credentials, useProxy, proxyURL)
}

const kimiUsageURL = "https://api.kimi.com/coding/v1/usages"

type kimiUsageResponse struct {
	Usage  *kimiUsageDetail `json:"usage"`
	Limits []kimiUsageLimit `json:"limits"`
}

type kimiUsageLimit struct {
	Window kimiUsageWindow  `json:"window"`
	Detail *kimiUsageDetail `json:"detail"`
}

type kimiUsageWindow struct {
	Duration int    `json:"duration"`
	TimeUnit string `json:"timeUnit"`
}

type kimiUsageDetail struct {
	Used      any    `json:"used"`
	Limit     any    `json:"limit"`
	ResetTime string `json:"resetTime"`
}

func (k *KimiProvider) loadKimiQuota(ctx context.Context, credentials map[string]any, useProxy bool, proxyURL string) *aoyorouter.ProviderQuota {
	accessToken, _ := credentials["access_token"].(string)
	if strings.TrimSpace(accessToken) == "" {
		
		return &aoyorouter.ProviderQuota{Error: "Kimi credentials are incomplete"}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, kimiUsageURL, nil)
	if err != nil {
		k.logger.Error("failed to create request", "error", err)
		return &aoyorouter.ProviderQuota{Error: "quota unavailable"}
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/json")
	if deviceID, _ := credentials["device_id"].(string); strings.TrimSpace(deviceID) != "" {
		request.Header.Set("X-Msh-Device-Id", deviceID)
	}

	server, err := proxyHTTPClient(useProxy, proxyURL)
	if err != nil {
		k.logger.Error("failed to create proxy client", "error", err)
		return &aoyorouter.ProviderQuota{Error: "quota request failed"}
	}
	response, err := server.Do(request)
	if err != nil {
		k.logger.Error("failed to send request", "error", err)
		return &aoyorouter.ProviderQuota{Error: "quota request failed"}
	}
	defer response.Body.Close()

	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		k.logger.Error("failed to read response", "error", err)
		return &aoyorouter.ProviderQuota{Error: "quota request failed"}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &aoyorouter.ProviderQuota{Error: kimiQuotaError(response.StatusCode, data)}
	}

	var usage kimiUsageResponse
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&usage); err != nil {
		k.logger.Error("failed to decode response", "error", err)
		return &aoyorouter.ProviderQuota{Error: "invalid quota response"}
	}

	windows := make([]*aoyorouter.ProviderQuotaWindow, 0, len(usage.Limits)+1)
	for _, limit := range usage.Limits {
		if window := kimiQuotaWindowToProto(limit.Detail, limit.Window); window != nil {
			windows = append(windows, window)
		}
	}
	if len(windows) == 0 {
		if window := kimiQuotaWindowToProto(usage.Usage, kimiUsageWindow{Duration: 1, TimeUnit: "TIME_UNIT_WEEK"}); window != nil {
			windows = append(windows, window)
		}
	}
	if len(windows) == 0 {
		return &aoyorouter.ProviderQuota{Error: "quota unavailable"}
	}

	quota := &aoyorouter.ProviderQuota{Quotas: windows, PlanType: kimiPlanType(credentials)}
	return quota
}

func kimiQuotaWindowToProto(detail *kimiUsageDetail, window kimiUsageWindow) *aoyorouter.ProviderQuotaWindow {
	if detail == nil {
		return nil
	}
	used, usedOK := kimiNumberFloat(detail.Used)
	limit, limitOK := kimiNumberFloat(detail.Limit)
	if !usedOK && !limitOK {
		return nil
	}

	usedPercent := 0.0
	if limit > 0 {
		usedPercent = used / limit * 100
	}
	return &aoyorouter.ProviderQuotaWindow{
		UsedPercent:   usedPercent,
		ResetsAt:      detail.ResetTime,
		WindowMinutes: kimiWindowMinutes(window),
	}
}

func kimiNumberFloat(value any) (float64, bool) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(number, 64)
		return parsed, err == nil
	case float64:
		return number, true
	default:
		return 0, false
	}
}

func kimiWindowMinutes(window kimiUsageWindow) int32 {
	if window.Duration <= 0 {
		return 0
	}
	minutes := int64(window.Duration)
	switch window.TimeUnit {
	case "TIME_UNIT_HOUR":
		minutes *= 60
	case "TIME_UNIT_DAY":
		minutes *= 24 * 60
	case "TIME_UNIT_WEEK":
		minutes *= 7 * 24 * 60
	case "TIME_UNIT_MINUTE":
	default:
		return 0
	}
	if minutes > 1<<31-1 {
		return 0
	}
	return int32(minutes)
}

func kimiQuotaError(statusCode int, data []byte) string {
	var response struct {
		Message string `json:"message"`
		Error   any    `json:"error"`
		Details []struct {
			Debug struct {
				Reason           string `json:"reason"`
				LocalizedMessage struct {
					Message string `json:"message"`
				} `json:"localizedMessage"`
			} `json:"debug"`
		} `json:"details"`
	}
	if json.Unmarshal(data, &response) == nil {
		for _, detail := range response.Details {
			if detail.Debug.Reason == "REASON_FEATURE_NO_PERMISSION" {
				return "Kimi Code quota requires an active subscription"
			}
			if detail.Debug.LocalizedMessage.Message != "" {
				return detail.Debug.LocalizedMessage.Message
			}
		}
		if response.Message != "" {
			return response.Message
		}
		if message := kimiErrorMessage(response.Error); message != "" {
			return message
		}
	}
	return fmt.Sprintf("quota returned status %d", statusCode)
}

func kimiErrorMessage(value any) string {
	switch errorValue := value.(type) {
	case string:
		return errorValue
	case map[string]any:
		message, _ := errorValue["message"].(string)
		return message
	default:
		return ""
	}
}

func kimiPlanType(credentials map[string]any) string {
	if value, _ := credentials["user_level_name"].(string); strings.TrimSpace(value) != "" {
		return value
	}
	return "Kimi Code"
}
