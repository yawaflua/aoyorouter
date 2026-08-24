package cursor

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Live diagnostics against api2.cursor.sh. All of these skip unless a token is
// supplied:
//
//	CURSOR_TOKEN=... go test ./pkg/cursor -run TestLive -v
//
// CURSOR_MODEL selects the model (default: "default", Cursor's Auto) and
// CURSOR_MODELS is a comma-separated list for TestLiveModelMatrix.
func liveToken(t *testing.T) string {
	t.Helper()
	token := os.Getenv("CURSOR_TOKEN")
	if token == "" {
		t.Skip("set CURSOR_TOKEN to run live tests")
	}
	return token
}

func liveServer(t *testing.T) *Server {
	t.Helper()
	s, err := NewServer(Config{Port: 0}, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Shutdown(context.Background()) })
	return s
}

func liveModel() string {
	if m := os.Getenv("CURSOR_MODEL"); m != "" {
		return m
	}
	return "default"
}

func TestLiveModels(t *testing.T) {
	token := liveToken(t)
	models, err := liveServer(t).HandleModels(context.Background(), token, "")
	if err != nil {
		t.Fatalf("HandleModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("no models returned")
	}
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.Id
	}
	t.Logf("%d models: %s", len(ids), strings.Join(ids, " "))
}

// TestLiveChatCompletions drives the bridge handler end to end and prints the
// SSE it produced. A rejection by Cursor must surface as a readable error, not
// as an empty stream.
func TestLiveChatCompletions(t *testing.T) {
	token := liveToken(t)
	model := liveModel()

	body, _ := json.Marshal(map[string]any{
		"model":    model,
		"stream":   true,
		"messages": []map[string]any{{"role": "user", "content": "hi"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	liveServer(t).handleChatCompletions(rec, req)

	out, _ := io.ReadAll(rec.Result().Body)
	t.Logf("model=%s status=%d\n%s", model, rec.Code, out)
	if rec.Code == http.StatusOK && len(out) == 0 {
		t.Fatal("200 with an empty body: the failure was swallowed")
	}
}

// TestLiveModelMatrix reports, per model, whether the account can run it.
// Useful after a plan change to see what opened up:
//
//	CURSOR_TOKEN=... CURSOR_MODELS=default,claude-4.5-sonnet go test \
//	    ./pkg/cursor -run TestLiveModelMatrix -v
func TestLiveModelMatrix(t *testing.T) {
	token := liveToken(t)
	models := strings.Split(os.Getenv("CURSOR_MODELS"), ",")
	if len(models) == 1 && models[0] == "" {
		models = []string{"default", "composer-2.5", "claude-4.5-sonnet", "gpt-5"}
	}
	s := liveServer(t)
	for _, m := range models {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		t.Logf("%-34s -> %s", m, s.probeModel(token, m))
		time.Sleep(400 * time.Millisecond)
	}
}

// probeModel sends a one-word prompt and summarises what came back.
func (s *Server) probeModel(token, model string) string {
	body, err := GenerateChatBody([]OpenAIMessage{{Role: "user", Content: TextContent("hi")}}, model)
	if err != nil {
		return "build-error: " + err.Error()
	}
	req, _ := http.NewRequest(http.MethodPost,
		cursorAPIHost+"/aiserver.v1.ChatService/StreamUnifiedChatWithTools", bytes.NewReader(body))
	req.Header = s.cursorHeaders("", token, true)

	resp, err := s.client.Do(req)
	if err != nil {
		return "http-error: " + err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "HTTP " + resp.Status
	}

	var text strings.Builder
	if err := readFrames(resp.Body, func(_, t string) { text.WriteString(t) }); err != nil {
		return err.Error()
	}
	if text.Len() == 0 {
		return "OK but empty"
	}
	return "OK: " + text.String()
}

// TestLiveEndpoints checks which chat RPCs the API still exposes. Cursor 3.x
// added ChatService/StreamUnifiedChat, whose request schema differs from the
// StreamUnifiedChatWithTools message this package encodes.
func TestLiveEndpoints(t *testing.T) {
	token := liveToken(t)
	body, err := GenerateChatBody([]OpenAIMessage{{Role: "user", Content: TextContent("hi")}}, liveModel())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		"/aiserver.v1.ChatService/StreamUnifiedChatWithTools",
		"/aiserver.v1.ChatService/StreamUnifiedChat",
	} {
		t.Logf("%-52s -> %s", strings.TrimPrefix(p, "/aiserver.v1."), probeRPC(token, p, body))
		time.Sleep(400 * time.Millisecond)
	}
}

// probeRPC posts body to an RPC path and returns a raw summary of the reply,
// including the end-of-stream frame verbatim.
func probeRPC(token, path string, body []byte) string {
	req, _ := http.NewRequest(http.MethodPost, cursorAPIHost+path, bytes.NewReader(body))
	req.Header = probeHeaders(token)
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return "http-error: " + err.Error()
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "HTTP " + resp.Status + " " + string(raw[:min(len(raw), 200)])
	}

	var out []string
	var text strings.Builder
	for i := 0; i+5 <= len(raw); {
		flag := raw[i]
		n := int(binary.BigEndian.Uint32(raw[i+1 : i+5]))
		if i+5+n > len(raw) {
			break
		}
		data := raw[i+5 : i+5+n]
		i += 5 + n
		if flag&flagCompressed != 0 && n > 0 {
			if zr, e := gzip.NewReader(bytes.NewReader(data)); e == nil {
				if d, e2 := io.ReadAll(zr); e2 == nil {
					data = d
				}
				zr.Close()
			}
		}
		if flag&flagEndStream != 0 {
			out = append(out, "eos:"+string(data))
			continue
		}
		if c, _ := DecodeStreamUnifiedChatResponse(data); c != nil {
			text.WriteString(c.Content)
		}
	}
	if text.Len() > 0 {
		return "OK text=" + text.String()
	}
	if len(out) == 0 {
		return "empty"
	}
	return strings.Join(out, "; ")
}

func probeHeaders(token string) http.Header {
	h := http.Header{}
	h.Set("authorization", "Bearer "+token)
	h.Set("connect-protocol-version", "1")
	h.Set("user-agent", "connect-es/1.6.1")
	h.Set("x-cursor-checksum", Checksum(token))
	h.Set("x-cursor-client-version", DefaultClientVersion)
	h.Set("x-cursor-config-version", uuid.NewString())
	h.Set("x-cursor-timezone", "Asia/Shanghai")
	h.Set("x-ghost-mode", "true")
	h.Set("connect-accept-encoding", "gzip")
	h.Set("connect-content-encoding", "gzip")
	h.Set("content-type", "application/connect+proto")
	h.Set("x-amzn-trace-id", "Root="+uuid.NewString())
	h.Set("x-client-key", Hashed64Hex(token, ""))
	h.Set("x-request-id", uuid.NewString())
	h.Set("x-session-id", sessionID(token))
	return h
}
