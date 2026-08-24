package cursor

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	loginPageURL    = "https://www.cursor.com/loginDeepControl"
	loginControlURL = "https://www.cursor.com/api/auth/loginDeepCallbackControl"
	authPollURL     = "https://api2.cursor.sh/auth/poll"
	cursorUA        = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Cursor/0.48.6 Chrome/132.0.6834.210 Electron/34.3.4 Safari/537.36"
)

// LoginResult is the outcome of a completed login flow.
type LoginResult struct {
	// AccessToken is the raw Cursor access token.
	AccessToken string
	// AuthID is the auth identity ("provider|userId").
	AuthID string
	// Cookie is the ready-to-use Cursor cookie:
	// "userId%3A%3AaccessToken" when AuthID contains a user id,
	// otherwise just the access token.
	Cookie string
}

// LoginFlow drives the Cursor deep-link login: the user opens LoginURL in a
// browser, approves, and Poll returns the resulting cookie.
type LoginFlow struct {
	client    *http.Client
	uuid      string
	verifier  string
	challenge string
	// LoginURL is the page the user must open to authorize.
	LoginURL string
}

// NewLoginFlow creates a login flow. proxyURL optionally routes the
// polling/control requests through a user proxy.
func NewLoginFlow(proxyURL string) (*LoginFlow, error) {
	client, err := buildHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}

	verifierBytes := make([]byte, 43)
	if _, err := rand.Read(verifierBytes); err != nil {
		return nil, fmt.Errorf("cursor login: generate verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	challengeSum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeSum[:])

	id := uuid.NewString()
	return &LoginFlow{
		client:    client,
		uuid:      id,
		verifier:  verifier,
		challenge: challenge,
		LoginURL:  fmt.Sprintf("%s?challenge=%s&uuid=%s&mode=login", loginPageURL, challenge, id),
	}, nil
}

// UUID returns the flow identifier used by loginDeepCallbackControl.
func (f *LoginFlow) UUID() string { return f.uuid }

// Challenge returns the PKCE challenge sent to loginDeepCallbackControl.
func (f *LoginFlow) Challenge() string { return f.challenge }

// PollOnce performs a single auth/poll attempt. ok is false while the user
// has not completed the login yet.
func (f *LoginFlow) PollOnce(ctx context.Context) (*LoginResult, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s?uuid=%s&verifier=%s", authPollURL, f.uuid, f.verifier), nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", cursorUA)
	req.Header.Set("Accept", "*/*")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false, nil
	}

	var data struct {
		AccessToken string `json:"accessToken"`
		AuthID      string `json:"authId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, false, fmt.Errorf("cursor login: decode poll response: %w", err)
	}
	if data.AccessToken == "" {
		return nil, false, nil
	}

	result := &LoginResult{AccessToken: data.AccessToken, AuthID: data.AuthID}
	if parts := strings.SplitN(data.AuthID, "|", 2); len(parts) > 1 && parts[1] != "" {
		result.Cookie = parts[1] + "%3A%3A" + data.AccessToken
	} else {
		result.Cookie = data.AccessToken
	}
	return result, true, nil
}

// Poll polls until the login completes, the context is cancelled, or
// maxAttempts is reached (interval between attempts).
func (f *LoginFlow) Poll(ctx context.Context, interval time.Duration, maxAttempts int) (*LoginResult, error) {
	for range maxAttempts {
		result, ok, err := f.PollOnce(ctx)
		if err != nil {
			return nil, err
		}
		if ok {
			return result, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
	return nil, fmt.Errorf("cursor login: timeout waiting for authorization")
}

// ExchangeSessionToken converts a WorkosCursorSessionToken (from the
// cursor.com browser cookie) into a client cookie via the
// loginDeepCallbackControl endpoint. This is the /cursor/loginDeepControl
// route from the original JS project.
func ExchangeSessionToken(ctx context.Context, sessionToken, proxyURL string) (*LoginResult, error) {
	flow, err := NewLoginFlow(proxyURL)
	if err != nil {
		return nil, err
	}

	body, _ := json.Marshal(map[string]string{
		"uuid":      flow.UUID(),
		"challenge": flow.Challenge(),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginControlURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/132.0.6834.210 Safari/537.36")
	req.Header.Set("Cookie", "WorkosCursorSessionToken="+sessionToken)

	resp, err := flow.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cursor login: deep control: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cursor login: deep control returned status %d", resp.StatusCode)
	}

	return flow.Poll(ctx, time.Second, 20)
}
