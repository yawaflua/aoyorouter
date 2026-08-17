package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

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