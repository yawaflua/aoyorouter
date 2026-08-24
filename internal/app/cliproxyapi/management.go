package cliproxyapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/yawaflua/aoyorouter/internal/config"
)

type Management struct {
	config *config.C
	logger *slog.Logger
}

type ManagementAuthorization struct {
	Status    string `json:"status"`
	URL       string `json:"url"`
	State     string `json:"state"`
	Flow      string `json:"flow"`
	UserCode  string `json:"user_code"`
	ExpiresIn int32  `json:"expires_in"`
	Error     string `json:"error"`
}

func NewManagement(config *config.C, logger *slog.Logger) *Management {
	return &Management{
		config: config,
		logger: logger,
	}
}

func (a *Management) ManagementJSON(ctx context.Context, method, path string, query url.Values, body any, output any) error {
	endpoint := fmt.Sprintf("http://%s:%d%s", a.config.HTTP.Host, a.config.HTTP.Port+1, path)
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+a.config.InitialPassword)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var apiError struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &apiError)
		if apiError.Error != "" {
			return fmt.Errorf("%s", apiError.Error)
		}
		return fmt.Errorf("management API returned status %d", response.StatusCode)
	}
	if output != nil && len(data) > 0 {
		if err := json.Unmarshal(data, output); err != nil {
			return err
		}
	}
	return nil
}
