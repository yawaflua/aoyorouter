package cliproxyapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

type EffortLevel string

const (
	EffortLow    EffortLevel = "low"
	EffortMedium EffortLevel = "medium"
	EffortHigh   EffortLevel = "high"
)

var effortSuffixes = []EffortLevel{EffortLow, EffortMedium, EffortHigh}

func parseEffortModel(model string) (base string, effort EffortLevel, ok bool) {
	for _, level := range effortSuffixes {
		suffix := "-" + string(level)
		if strings.HasSuffix(model, suffix) {
			return strings.TrimSuffix(model, suffix), level, true
		}
	}
	return model, "", false
}

// effortModel renders the model in the thinking suffix syntax CLIProxyAPI parses
// natively, e.g. "claude/claude-opus-4-8" + high -> "claude/claude-opus-4-8(high)".
//
// The upstream resolves the suffix per provider: adaptive thinking plus
// output_config.effort for Claude, reasoning_effort for OpenAI-compatible
// providers, and so on. It also checks whether the model supports thinking at
// all and clamps budgets to the model's range, so we deliberately do not touch
// thinking, reasoning_effort or max_tokens here.
func effortModel(base string, effort EffortLevel) string {
	return base + "(" + string(effort) + ")"
}

func isRewritePath(path string) bool {
	path = strings.TrimSuffix(path, "/")
	return strings.HasSuffix(path, "/v1/chat/completions") ||
		strings.HasSuffix(path, "/v1/messages") ||
		strings.HasSuffix(path, "/v1beta/messages")
}

func EffortPresetMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Debug("effort preset middleware hit", "method", r.Method, "path", r.URL.Path, "raw_path", r.URL.RawPath)
		path := strings.TrimSuffix(r.URL.Path, "/")
		if r.Method != http.MethodPost || !isRewritePath(path) {
			logger.Debug("effort preset skipped: path/method mismatch", "method", r.Method, "path", r.URL.Path)
			next.ServeHTTP(w, r)
			return
		}

		if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
			logger.Debug("effort preset skipped: gzip encoded request body", "path", r.URL.Path)
			next.ServeHTTP(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		_ = r.Body.Close()
		if err != nil {
			logger.Debug("effort preset skipped: failed to read body", "path", r.URL.Path, "error", err)
			r.Body = io.NopCloser(bytes.NewReader(body))
			next.ServeHTTP(w, r)
			return
		}
		if len(body) == 0 {
			r.Body = io.NopCloser(bytes.NewReader(body))
			next.ServeHTTP(w, r)
			return
		}

		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			logger.Debug("effort preset skipped: body is not valid JSON", "path", r.URL.Path, "error", err)
			r.Body = io.NopCloser(bytes.NewReader(body))
			next.ServeHTTP(w, r)
			return
		}

		model, ok := payload["model"].(string)
		if !ok {
			logger.Debug("effort preset skipped: model field missing or not string", "path", r.URL.Path)
			r.Body = io.NopCloser(bytes.NewReader(body))
			next.ServeHTTP(w, r)
			return
		}

		base, effort, matched := parseEffortModel(model)
		if !matched {
			logger.Debug("effort preset skipped: model does not have effort suffix", "path", r.URL.Path, "model", model)
			r.Body = io.NopCloser(bytes.NewReader(body))
			next.ServeHTTP(w, r)
			return
		}

		rewritten := effortModel(base, effort)
		payload["model"] = rewritten

		modified, err := json.Marshal(payload)
		if err != nil {
			logger.Warn("effort preset failed to marshal modified body", "path", r.URL.Path, "error", err)
			r.Body = io.NopCloser(bytes.NewReader(body))
			next.ServeHTTP(w, r)
			return
		}

		logger.Debug("effort preset applied",
			"original_model", model,
			"rewritten_model", rewritten,
			"effort", effort,
		)

		r.Body = io.NopCloser(bytes.NewReader(modified))
		r.ContentLength = int64(len(modified))
		r.Header.Set("Content-Length", fmt.Sprintf("%d", len(modified)))
		next.ServeHTTP(w, r)
	})
}

func ModifyEffortModelsResponse(resp *http.Response) error {
	if resp == nil || resp.Request == nil {
		return nil
	}
	if !strings.HasSuffix(resp.Request.URL.Path, "/v1/models") {
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()

	modified, err := ExpandEffortModels(body)
	if err != nil {
		resp.Body = io.NopCloser(bytes.NewReader(body))
		return nil
	}

	resp.Body = io.NopCloser(bytes.NewReader(modified))
	resp.ContentLength = int64(len(modified))
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(modified)))
	return nil
}

func ModifyEffortModelsResponseWithLogger(logger *slog.Logger) func(*http.Response) error {
	return func(resp *http.Response) error {
		if resp == nil || resp.Request == nil {
			return nil
		}
		logger.Debug("effort models response modifier hit", "path", resp.Request.URL.Path, "status", resp.StatusCode)
		return ModifyEffortModelsResponse(resp)
	}
}

func ExpandEffortModels(raw []byte) ([]byte, error) {
	var response map[string]any
	if err := json.Unmarshal(raw, &response); err != nil {
		return raw, nil
	}
	if response["object"] != "list" {
		return raw, nil
	}

	data, ok := response["data"].([]any)
	if !ok || len(data) == 0 {
		return raw, nil
	}

	var expanded []map[string]any
	for _, rawItem := range data {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		expanded = append(expanded, copyMap(item))
		id, _ := item["id"].(string)
		root, _ := item["root"].(string)
		for _, level := range effortSuffixes {
			variant := copyMap(item)
			variant["id"] = id + "-" + string(level)
			if root != "" {
				variant["root"] = root + "-" + string(level)
			}
			expanded = append(expanded, variant)
		}
	}

	result := copyMap(response)
	result["data"] = expanded
	return json.Marshal(result)
}

func copyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(m))
	for k, v := range m {
		cloned[k] = v
	}
	return cloned
}
