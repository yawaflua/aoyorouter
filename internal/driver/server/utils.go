package server

import (
	"fmt"
	"net"
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

// removeString returns values without target. It allocates rather than reusing
// values' backing array, which the previous values[:0] form clobbered — the
// caller's slice was silently rewritten in place.
func removeString(values []string, target string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}

func normalizedUniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
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

	// Bound the connection-establishment phases only. A blanket
	// http.Client.Timeout is deliberately absent: these clients also carry
	// long-lived streaming responses, and it would sever them mid-stream.
	transport.DialContext = (&net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = 60 * time.Second
	transport.ExpectContinueTimeout = 1 * time.Second

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
