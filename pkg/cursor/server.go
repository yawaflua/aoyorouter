package cursor

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	cursorAPIHost       = "https://api2.cursor.sh"
	cursorAPIHostHeader = "api2.cursor.sh"
)

// Server is an OpenAI-compatible HTTP bridge to the Cursor AI backend.
type Server struct {
	cfg    Config
	client *http.Client
	logger *slog.Logger
	http   *http.Server
}

// NewServer creates a bridge server. ProxyURL in cfg routes outbound
// requests to api2.cursor.sh through the given user proxy.
func NewServer(cfg Config, logger *slog.Logger) (*Server, error) {
	cfg = cfg.withDefaults()
	if logger == nil {
		logger = slog.Default()
	}
	client, err := buildHTTPClient(cfg.ProxyURL)
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, client: client, logger: logger}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", s.handleModels)
	mux.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)

	s.http = &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", cfg.Port),
		Handler: mux,
	}
	return s, nil
}

// BaseURL returns the local OpenAI-compatible base URL of the bridge.
func (s *Server) BaseURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/v1", s.cfg.Port)
}

// ListenAndServe starts the bridge and blocks until the server stops.
func (s *Server) ListenAndServe() error {
	s.logger.Info("cursor bridge listening", slog.Int("port", s.cfg.Port), slog.String("proxy", s.cfg.ProxyURL))
	return s.http.ListenAndServe()
}

// Shutdown gracefully stops the bridge.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

// ---------- auth helpers ----------

// extractToken picks one token from the Authorization header. Supports
// comma-separated cookie pools (random pick) and the `userX::token` /
// `userX%3A%3Atoken` cookie formats.
func extractToken(r *http.Request) string {
	bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	keys := strings.Split(bearer, ",")
	for i := range keys {
		keys[i] = strings.TrimSpace(keys[i])
	}
	token := keys[0]
	if len(keys) > 1 {
		token = keys[rand.Intn(len(keys))]
	}
	if strings.Contains(token, "%3A%3A") {
		token = strings.SplitN(token, "%3A%3A", 2)[1]
	} else if strings.Contains(token, "::") {
		token = strings.SplitN(token, "::", 2)[1]
	}
	return strings.TrimSpace(token)
}

func (s *Server) cursorHeaders(checksum, token string, streaming bool) http.Header {
	if checksum == "" {
		checksum = Checksum(token)
	}
	h := http.Header{}
	h.Set("authorization", "Bearer "+token)
	h.Set("connect-protocol-version", "1")
	h.Set("user-agent", "connect-es/1.6.1")
	h.Set("x-cursor-checksum", checksum)
	h.Set("x-cursor-client-version", s.cfg.CursorClientVersion)
	h.Set("x-cursor-config-version", uuid.NewString())
	h.Set("x-cursor-timezone", "Asia/Shanghai")
	h.Set("x-ghost-mode", "true")
	h.Set("Host", cursorAPIHostHeader)
	if streaming {
		h.Set("connect-accept-encoding", "gzip")
		h.Set("connect-content-encoding", "gzip")
		h.Set("content-type", "application/connect+proto")
		h.Set("x-amzn-trace-id", "Root="+uuid.NewString())
		h.Set("x-client-key", Hashed64Hex(token, ""))
		h.Set("x-request-id", uuid.NewString())
		h.Set("x-session-id", sessionID(token))
	} else {
		h.Set("accept-encoding", "gzip")
		h.Set("content-type", "application/proto")
	}
	return h
}

type ModelResponse struct {
	Id      string
	Created string
	Object  string
	OwnedBy string
}

func (s *Server) HandleModels(ctx context.Context, token string, checksum string) ([]ModelResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cursorAPIHost+"/aiserver.v1.AiService/AvailableModels", nil)
	if err != nil {
		return nil, err
	}
	req.Header = s.cursorHeaders(checksum, token, false)

	resp, err := s.client.Do(req)
	if err != nil {
		s.logger.Error("cursor: AvailableModels request failed", slog.Any("err", err))
		return nil, err
	}
	defer resp.Body.Close()
	body, err := decodeMaybeGzip(resp)
	if err != nil {
		return nil, err
	}

	names, err := DecodeAvailableModels(body)
	if err != nil {
		s.logger.Error("cursor: decode AvailableModels failed", slog.Any("err", err), slog.String("raw", string(body)))
		return nil, err
	}

	now := time.Now().UTC().String()
	var data []ModelResponse
	for _, name := range names {
		data = append(data, ModelResponse{
			Id:      name,
			Created: now,
			Object:  "model",
			OwnedBy: "cursor",
		})
	}
	return data, nil
}

// ---------- /v1/models ----------
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	names, err := s.HandleModels(ctx, token, r.Header.Get("x-cursor-checksum"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	out := map[string]any{"object": "list"}
	out["data"] = names
	writeJSON(w, http.StatusOK, out)
}

// ---------- /v1/chat/completions ----------

type chatCompletionRequest struct {
	Model    string          `json:"model"`
	Messages []OpenAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var reqBody chatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	token := extractToken(r)
	if len(reqBody.Messages) == 0 || token == "" {
		writeError(w, http.StatusBadRequest, "invalid request. Messages should be a non-empty array and authorization is required")
		return
	}

	cursorBody, err := GenerateChatBody(reqBody.Messages, reqBody.Model)
	if err != nil {
		s.logger.Error("cursor: build request body failed", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cursorAPIHost+"/aiserver.v1.ChatService/StreamUnifiedChatWithTools",
		bytes.NewReader(cursorBody))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	req.Header = s.cursorHeaders(r.Header.Get("x-cursor-checksum"), token, true)

	resp, err := s.client.Do(req)
	if err != nil {
		s.logger.Error("cursor: StreamUnifiedChatWithTools failed", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		writeError(w, resp.StatusCode, resp.Status)
		return
	}

	if reqBody.Stream {
		s.streamResponse(w, resp.Body, reqBody.Model)
		return
	}
	s.collectResponse(w, resp.Body, reqBody.Model)
}

// streamResponse forwards Connect-RPC frames as OpenAI SSE chunks.
func (s *Server) streamResponse(w http.ResponseWriter, body io.Reader, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, _ := w.(http.Flusher)
	responseID := "chatcmpl-" + uuid.NewString()

	thinkingStart, thinkingEnd := "<thinking>", "</thinking>"
	emit := func(content string) {
		if content == "" {
			return
		}
		chunk := map[string]any{
			"id":      responseID,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{{
				"index": 0,
				"delta": map[string]any{"content": content},
			}},
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}
	}

	err := readFrames(body, func(thinking, text string) {
		var content strings.Builder
		if thinkingStart != "" && len(thinking) > 0 {
			content.WriteString(thinkingStart + "\n")
			thinkingStart = ""
		}
		content.WriteString(thinking)
		if thinkingEnd != "" && len(thinking) == 0 && len(text) != 0 && thinkingStart == "" {
			content.WriteString("\n" + thinkingEnd + "\n")
			thinkingEnd = ""
		}
		content.WriteString(text)
		emit(content.String())
	})
	if err != nil {
		s.logger.Error("cursor: stream error", slog.Any("err", err))
		errData, _ := json.Marshal(map[string]any{"error": "stream processing error"})
		fmt.Fprintf(w, "data: %s\n\n", errData)
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

// collectResponse aggregates the full stream and returns one OpenAI response.
func (s *Server) collectResponse(w http.ResponseWriter, body io.Reader, model string) {
	thinkingStart, thinkingEnd := "<thinking>", "</thinking>"
	var content strings.Builder
	err := readFrames(body, func(thinking, text string) {
		if thinkingStart != "" && len(thinking) > 0 {
			content.WriteString(thinkingStart + "\n")
			thinkingStart = ""
		}
		content.WriteString(thinking)
		if thinkingEnd != "" && len(thinking) == 0 && len(text) != 0 && thinkingStart == "" {
			content.WriteString("\n" + thinkingEnd + "\n")
			thinkingEnd = ""
		}
		content.WriteString(text)
	})
	if err != nil {
		s.logger.Error("cursor: collect error", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":      "chatcmpl-" + uuid.NewString(),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": content.String()},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
	})
}

// readFrames reads the Connect-RPC framed stream incrementally and calls
// fn for each decoded chunk's thinking/text payloads.
func readFrames(body io.Reader, fn func(thinking, text string)) error {
	reader := bufio.NewReader(body)
	for {
		header := make([]byte, 5)
		if _, err := io.ReadFull(reader, header); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil
			}
			return err
		}
		magic := header[0]
		dataLen := int(header[1])<<24 | int(header[2])<<16 | int(header[3])<<8 | int(header[4])
		if dataLen <= 0 {
			continue
		}
		data := make([]byte, dataLen)
		if _, err := io.ReadFull(reader, data); err != nil {
			return err
		}
		thinking, text, err := ParseStreamChunk(append(header, data...))
		if err != nil {
			return err
		}
		_ = magic
		fn(thinking, text)
	}
}

// decodeMaybeGzip reads the full response body, transparently decompressing
// gzip. Needed because we set accept-encoding: gzip ourselves in
// cursorHeaders, so net/http does not auto-decompress.
func decodeMaybeGzip(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(body) > 2 && body[0] == 0x1f && body[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		return io.ReadAll(zr)
	}
	return body, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}
