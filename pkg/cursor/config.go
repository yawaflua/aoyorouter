package cursor

import (
	"context"
	"crypto/sha1"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/proxy"
)

// Config configures the cursor bridge server.
type Config struct {
	// Port to listen on (e.g. 3010). If 0, defaults to 3010.
	Port int
	// CursorClientVersion is sent as x-cursor-client-version.
	CursorClientVersion string
	// ProxyURL is an optional outbound proxy URL (http://, https://, socks5://)
	// used for requests to api2.cursor.sh. Empty means direct connection.
	ProxyURL string
}

func (c *Config) withDefaults() Config {
	out := *c
	if out.Port == 0 {
		out.Port = 3010
	}
	if out.CursorClientVersion == "" {
		out.CursorClientVersion = "2.6.21"
	}
	return out
}

// uuidV5 replicates uuid v5 (SHA-1, name-based, RFC 4122 §4.3) used by the
// JS client to derive the x-session-id.
func uuidV5(namespace uuid.UUID, name string) uuid.UUID {
	h := sha1.New()
	h.Write(namespace[:])
	h.Write([]byte(name))
	sum := h.Sum(nil)
	var u uuid.UUID
	copy(u[:], sum[:16])
	u[6] = (u[6] & 0x0f) | 0x50 // version 5
	u[8] = (u[8] & 0x3f) | 0x80 // variant RFC4122
	return u
}

// sessionID derives the x-session-id like uuidv5(authToken, uuidv5.DNS).
func sessionID(token string) string {
	dns := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	return uuidV5(dns, token).String()
}

func buildHTTPClient(proxyURL string) (*http.Client, error) {
	transport := &http.Transport{
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	if proxyURL != "" {
		u, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy url: %w", err)
		}
		switch u.Scheme {
		case "http", "https":
			transport.Proxy = http.ProxyURL(u)
		case "socks5", "socks5h":
			var auth *proxy.Auth
			if u.User != nil {
				password, _ := u.User.Password()
				auth = &proxy.Auth{User: u.User.Username(), Password: password}
			}
			dialer, err := proxy.SOCKS5("tcp", u.Host, auth, proxy.Direct)
			if err != nil {
				return nil, fmt.Errorf("socks5 proxy: %w", err)
			}
			transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			}
		default:
			return nil, fmt.Errorf("unsupported proxy scheme %q (want http, https or socks5)", u.Scheme)
		}
	}

	return &http.Client{Transport: transport}, nil
}
