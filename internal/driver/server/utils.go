package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yawaflua/aoyorouter/internal/models"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func quotaResetStrategy(period models.QuotaPeriod) aoyorouter.QuotaResetStrategy {
	switch period {
	case models.QuotaPeriodMinute:
		return aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_MINUTES
	case models.QuotaPeriodHour:
		return aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_HOURLY
	case models.QuotaPeriodDay:
		return aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_DAILY
	case models.QuotaPeriodWeek:
		return aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_WEEKLY
	case models.QuotaPeriodMonth:
		return aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_MONTHLY
	default:
		return aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_FOREVER
	}
}

func quotaPeriod(strategy aoyorouter.QuotaResetStrategy) (models.QuotaPeriod, time.Duration, error) {
	switch strategy {
	case aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_MINUTES:
		return models.QuotaPeriodMinute, time.Minute, nil
	case aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_HOURLY:
		return models.QuotaPeriodHour, time.Hour, nil
	case aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_DAILY:
		return models.QuotaPeriodDay, 24 * time.Hour, nil
	case aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_WEEKLY:
		return models.QuotaPeriodWeek, 7 * 24 * time.Hour, nil
	case aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_MONTHLY:
		return models.QuotaPeriodMonth, 30 * 24 * time.Hour, nil
	case aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_FOREVER:
		return models.QuotaPeriodForever, 0, nil
	default:
		return "", 0, status.Error(codes.InvalidArgument, "unsupported quota_reset_strategy")
	}
}

func providerOAuthReady(clientSecret string, credentials map[string]any) bool {
	if !strings.HasPrefix(clientSecret, "oauth:") {
		return false
	}
	if clientSecret != "oauth:pending" {
		return true
	}
	return providerCredentialsCompleted(credentials)
}

func validateProvider(name string, providerType aoyorouter.ProviderType, secret string) error {
	if name == "" || secret == "" {
		return status.Error(codes.InvalidArgument, "name and client_secret are required")
	}

	switch providerType {
	case aoyorouter.ProviderType_PROVIDER_TYPE_UNSPECIFIED:
		return status.Error(codes.InvalidArgument, "unsupported provider type")

	case aoyorouter.ProviderType_PROVIDER_TYPE_CUSTOM, aoyorouter.ProviderType_PROVIDER_TYPE_OPENAI, aoyorouter.ProviderType_PROVIDER_TYPE_ANTHROPIC, aoyorouter.ProviderType_PROVIDER_TYPE_KIMI, aoyorouter.ProviderType_PROVIDER_TYPE_GROK, aoyorouter.ProviderType_PROVIDER_TYPE_ANTIGRAVITY:
		return nil

	default:
		return status.Error(codes.InvalidArgument, "unsupported provider type")
	}
}

func removeString(values []string, target string) []string {
	result := values[:0]
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}

func proxyHTTPClient(useProxy bool, proxyURL string) (*http.Client, error) {
	if !useProxy {
		return http.DefaultClient, nil
	}

	parsed, err := url.Parse(strings.TrimSpace(proxyURL))
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("invalid proxy URL %q", proxyURL)
	}

	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("unsupported default HTTP transport")
	}
	transport = transport.Clone()

	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		transport.Proxy = http.ProxyURL(parsed)
	case "socks", "socks5", "socks5h":
		if strings.EqualFold(parsed.Scheme, "socks") {
			parsed.Scheme = "socks5"
		}
		transport.Proxy = http.ProxyURL(parsed)
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q", parsed.Scheme)
	}

	return &http.Client{Transport: transport}, nil
}
