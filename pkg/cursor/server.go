package cursor

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	cursorAPIHost       = "https://api2.cursor.sh"
	cursorAPIHostHeader = "api2.cursor.sh"

	// maxFrameSize bounds a single Connect-RPC frame payload.
	maxFrameSize = 32 << 20
)

// Server is an OpenAI-compatible HTTP bridge to the Cursor AI backend.
type Server struct {
	cfg      Config
	logger   *slog.Logger
	http     *http.Server
	listener net.Listener
	port     int

	// proxies maps an access token to the outbound proxy URL to use for it.
	// Provider reloads write to it while request handlers read it, hence the
	// lock. It is deliberately not exposed directly: the previous
	// Proxies() *map[string]string handed callers a pointer to an
	// uninitialised map, so the first write panicked.
	proxyMu sync.RWMutex
	proxies map[string]string
}

// SetProxy records the outbound proxy to use for requests made with token.
func (s *Server) SetProxy(token, proxyURL string) {
	if token == "" {
		return
	}
	s.proxyMu.Lock()
	defer s.proxyMu.Unlock()
	s.proxies[token] = proxyURL
}

// ProxyFor returns the proxy registered for token, or "" if there is none.
func (s *Server) ProxyFor(token string) string {
	s.proxyMu.RLock()
	defer s.proxyMu.RUnlock()
	return s.proxies[token]
}

// NewServer creates a bridge server. ProxyURL in cfg routes outbound
// requests to api2.cursor.sh through the given user proxy.
func NewServer(cfg Config, logger *slog.Logger) (*Server, error) {
	cfg = cfg.withDefaults()
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{cfg: cfg, logger: logger, proxies: make(map[string]string)}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", s.handleModels)
	mux.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)

	// Log unmatched endpoints/methods instead of a bare 404.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.logger.Warn("cursor: no handler for request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("remote", r.RemoteAddr))
		writeError(w, http.StatusNotFound, "not found")
	})

	// Bind loopback only: this bridge is an internal helper and BaseURL
	// already advertises 127.0.0.1. Listening on 0.0.0.0 exposed an
	// unauthenticated proxy to the whole network.
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.Port))
	if err != nil {
		return nil, fmt.Errorf("cursor: listen on port %d: %w", cfg.Port, err)
	}
	s.listener = ln
	s.port = ln.Addr().(*net.TCPAddr).Port
	s.http = &http.Server{Handler: mux}
	return s, nil
}

// Port returns the TCP port the bridge is bound to.
func (s *Server) Port() int { return s.port }

// BaseURL returns the local OpenAI-compatible base URL of the bridge.
func (s *Server) BaseURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/v1", s.port)
}

// ListenAndServe serves the bridge on the port bound by NewServer and blocks
// until the server stops.
func (s *Server) ListenAndServe() error {
	s.logger.Info("cursor bridge listening", slog.Int("port", s.port), slog.String("proxy", s.cfg.ProxyURL))
	return s.http.Serve(s.listener)
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
	h.Set("x-cursor-client-version", DefaultClientVersion)
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

func (s *Server) HandleModels(ctx context.Context, token string, checksum string, proxyURL string) ([]ModelResponse, error) {
	if proxyURL == "" {
		proxyURL = s.ProxyFor(token)
	}
	client, err := buildHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}
	defer client.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cursorAPIHost+"/aiserver.v1.AiService/AvailableModels", nil)
	if err != nil {
		return nil, err
	}
	req.Header = s.cursorHeaders(checksum, token, false)

	resp, err := client.Do(req)
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

	names, err := s.HandleModels(ctx, token, r.Header.Get("x-cursor-checksum"), s.ProxyFor(token))
	if err != nil {
		writeError(w, http.StatusBadGateway, "cursor: "+err.Error())
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
	defer r.Body.Close()
	var reqBody chatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		s.logger.Warn("cursor: bad request body", slog.Any("err", err))
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
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
		writeError(w, http.StatusInternalServerError, "cursor: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	client, err := buildHTTPClient(s.ProxyFor(token))
	if err != nil {
		s.logger.Error("cursor: build http client failed", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cursorAPIHost+"/aiserver.v1.ChatService/StreamUnifiedChatWithTools",
		bytes.NewReader(cursorBody))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	req.Header = s.cursorHeaders(r.Header.Get("x-cursor-checksum"), token, true)

	resp, err := client.Do(req)
	if err != nil {
		s.logger.Error("cursor: StreamUnifiedChatWithTools failed", slog.Any("err", err))
		writeError(w, http.StatusBadGateway, "cursor: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		s.logger.Error("cursor: StreamUnifiedChatWithTools rejected",
			slog.Int("status", resp.StatusCode),
			slog.String("body", string(detail)))
		msg := resp.Status
		if len(detail) > 0 {
			msg += ": " + string(detail)
		}
		writeError(w, resp.StatusCode, "cursor: "+msg)
		return
	}

	if reqBody.Stream {
		s.streamResponse(w, resp.Body, reqBody.Model)
		return
	}
	s.collectResponse(w, resp.Body, reqBody.Model)
}

// thinkingTagger wraps runs of reasoning output in <thinking> tags while
// passing normal text through untouched.
type thinkingTagger struct{ open bool }

func (t *thinkingTagger) next(thinking, text string) string {
	var b strings.Builder
	if thinking != "" {
		if !t.open {
			b.WriteString("<thinking>\n")
			t.open = true
		}
		b.WriteString(thinking)
	}
	if text != "" {
		b.WriteString(t.close())
		b.WriteString(text)
	}
	return b.String()
}

// close emits the terminator if a thinking block is still open.
func (t *thinkingTagger) close() string {
	if !t.open {
		return ""
	}
	t.open = false
	return "\n</thinking>\n"
}

// streamResponse forwards Connect-RPC frames as OpenAI SSE chunks.
//
// SSE headers are written lazily: Cursor reports rejections in the very first
// frame, so as long as nothing has been sent we can still answer with a real
// HTTP error status instead of an empty 200 stream that callers can only
// report as "stream ended without receiving any events".
func (s *Server) streamResponse(w http.ResponseWriter, body io.Reader, model string) {
	flusher, _ := w.(http.Flusher)
	responseID := "chatcmpl-" + uuid.NewString()
	started := false

	send := func(choice map[string]any) {
		if !started {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)
			started = true
		}
		data, _ := json.Marshal(map[string]any{
			"id":      responseID,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{choice},
		})
		fmt.Fprintf(w, "data: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}
	}
	emit := func(content string) {
		if content == "" {
			return
		}
		if !started {
			send(map[string]any{
				"index": 0,
				"delta": map[string]any{"role": "assistant", "content": ""},
			})
		}
		send(map[string]any{
			"index": 0,
			"delta": map[string]any{"content": content},
		})
	}

	var tagger thinkingTagger
	err := readFrames(body, func(thinking, text string) {
		emit(tagger.next(thinking, text))
	})
	if err != nil {
		s.logger.Error("cursor: stream error", slog.Any("err", err), slog.String("model", model))
		if !started {
			status, msg := errorStatus(err)
			writeError(w, status, msg)
			return
		}
		// Already streaming: the only way left to signal failure is an
		// in-band error event before closing the stream.
		errData, _ := json.Marshal(map[string]any{
			"error": map[string]any{"message": err.Error(), "type": "upstream_error"},
		})
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", errData)
	}
	emit(tagger.close())
	send(map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"})
	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

// collectResponse aggregates the full stream and returns one OpenAI response.
func (s *Server) collectResponse(w http.ResponseWriter, body io.Reader, model string) {
	var tagger thinkingTagger
	var content strings.Builder
	err := readFrames(body, func(thinking, text string) {
		content.WriteString(tagger.next(thinking, text))
	})
	if err != nil {
		s.logger.Error("cursor: collect error", slog.Any("err", err), slog.String("model", model))
		status, msg := errorStatus(err)
		writeError(w, status, msg)
		return
	}
	content.WriteString(tagger.close())

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

// errorStatus maps an upstream failure to an HTTP status and a message safe
// to hand back to the caller.
func errorStatus(err error) (int, string) {
	var se *StreamError
	if !errors.As(err, &se) {
		return http.StatusBadGateway, err.Error()
	}
	status := http.StatusBadGateway
	switch se.Code {
	case "unauthenticated":
		status = http.StatusUnauthorized
	case "permission_denied":
		status = http.StatusForbidden
	case "resource_exhausted":
		status = http.StatusTooManyRequests
	case "invalid_argument":
		status = http.StatusBadRequest
	case "unavailable":
		status = http.StatusServiceUnavailable
	}
	return status, se.Error()
}

// readFrames reads the Connect-RPC framed stream incrementally and calls fn
// for each decoded chunk's thinking/text payloads. It returns a [StreamError]
// when Cursor terminated the stream with an error.
func readFrames(body io.Reader, fn func(thinking, text string)) error {
	reader := bufio.NewReader(body)
	for {
		var header [5]byte
		if _, err := io.ReadFull(reader, header[:]); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil
			}
			return err
		}
		flag := header[0]
		// Check the wire value before narrowing to int: a corrupt or hostile
		// frame can claim up to 4 GiB and make(<that>) takes the process down
		// with an unrecoverable OOM.
		rawLen := binary.BigEndian.Uint32(header[1:])
		if rawLen > maxFrameSize {
			return fmt.Errorf("cursor: frame too large: %d bytes (max %d)", rawLen, maxFrameSize)
		}
		dataLen := int(rawLen)

		var data []byte
		if dataLen > 0 {
			data = make([]byte, dataLen)
			if _, err := io.ReadFull(reader, data); err != nil {
				return err
			}
		}

		payload, err := decodeFramePayload(flag, data)
		if err != nil {
			return fmt.Errorf("cursor: decompress frame: %w", err)
		}
		if flag&flagEndStream != 0 {
			// The end-of-stream frame is where Cursor reports rejections
			// (plan limits, outdated client, bad token). Never drop it.
			return parseEndStream(payload)
		}
		if len(payload) == 0 {
			continue
		}
		chunk, err := DecodeStreamUnifiedChatResponse(payload)
		if err != nil || chunk == nil {
			continue
		}
		fn(chunk.Thinking, chunk.Content)
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

// writeError replies with an OpenAI-shaped error object so that callers
// treating this bridge as an OpenAI endpoint can parse the message.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"message": msg,
			"type":    "cursor_bridge_error",
			"code":    status,
		},
	})
}
