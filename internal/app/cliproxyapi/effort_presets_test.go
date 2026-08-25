package cliproxyapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExpandEffortModels(t *testing.T) {
	input := []byte(`{"object":"list","data":[{"id":"claude/claude-opus-4","object":"model","owned_by":"anthropic"}]}`)
	output, err := ExpandEffortModels(input)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatal(err)
	}

	if len(result.Data) != 4 {
		t.Fatalf("expected 4 models, got %d", len(result.Data))
	}
	expected := []string{
		"claude/claude-opus-4",
		"claude/claude-opus-4-low",
		"claude/claude-opus-4-medium",
		"claude/claude-opus-4-high",
	}
	for i, id := range expected {
		if result.Data[i].ID != id {
			t.Errorf("expected %q at index %d, got %q", id, i, result.Data[i].ID)
		}
	}
}

func TestParseEffortModel(t *testing.T) {
	cases := []struct {
		model   string
		base    string
		effort  EffortLevel
		matched bool
	}{
		{"claude/claude-opus-4-8-high", "claude/claude-opus-4-8", EffortHigh, true},
		{"custom/gpt-4o-low", "custom/gpt-4o", EffortLow, true},
		{"gemini-2.5-pro-medium", "gemini-2.5-pro", EffortMedium, true},
		{"claude/claude-opus-4-8", "claude/claude-opus-4-8", "", false},
	}
	for _, tc := range cases {
		base, effort, matched := parseEffortModel(tc.model)
		if base != tc.base || effort != tc.effort || matched != tc.matched {
			t.Errorf("parseEffortModel(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.model, base, effort, matched, tc.base, tc.effort, tc.matched)
		}
	}
}

func TestEffortModel(t *testing.T) {
	if got := effortModel("claude/claude-opus-4-8", EffortHigh); got != "claude/claude-opus-4-8(high)" {
		t.Errorf("expected claude/claude-opus-4-8(high), got %q", got)
	}
	if got := effortModel("custom/gpt-4o", EffortLow); got != "custom/gpt-4o(low)" {
		t.Errorf("expected custom/gpt-4o(low), got %q", got)
	}
}

func TestEffortPresetMiddleware(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})

	handler := EffortPresetMiddleware(logger, upstream)
	payload := []byte(`{"model":"claude/claude-opus-4-8-low","messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["model"] != "claude/claude-opus-4-8(low)" {
		t.Errorf("expected model claude/claude-opus-4-8(low), got %v", result["model"])
	}
	// Thinking config is the upstream's job — we must not inject it ourselves.
	if _, ok := result["thinking"]; ok {
		t.Errorf("unexpected thinking field in proxied body")
	}
	if _, ok := result["reasoning_effort"]; ok {
		t.Errorf("unexpected reasoning_effort field in proxied body")
	}
}

func TestEffortPresetMiddlewareMessagesPath(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})

	handler := EffortPresetMiddleware(logger, upstream)
	payload := []byte(`{"model":"claude/claude-opus-4-8-high","messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages?beta=true", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["model"] != "claude/claude-opus-4-8(high)" {
		t.Errorf("expected model claude/claude-opus-4-8(high), got %v", result["model"])
	}
}

func TestEffortPresetMiddlewarePreservesBodyFields(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})

	handler := EffortPresetMiddleware(logger, upstream)
	payload := []byte(`{"model":"claude/claude-opus-4-8-medium","max_tokens":4096,"temperature":0.7,"stream":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["model"] != "claude/claude-opus-4-8(medium)" {
		t.Errorf("expected model claude/claude-opus-4-8(medium), got %v", result["model"])
	}
	if result["max_tokens"] != 4096.0 {
		t.Errorf("expected max_tokens to stay 4096, got %v", result["max_tokens"])
	}
	if result["temperature"] != 0.7 {
		t.Errorf("expected temperature to stay 0.7, got %v", result["temperature"])
	}
	if result["stream"] != true {
		t.Errorf("expected stream to stay true, got %v", result["stream"])
	}
}
