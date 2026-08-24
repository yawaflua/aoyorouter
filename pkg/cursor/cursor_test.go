package cursor

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func httptestRequest(auth string) *http.Request {
	r, _ := http.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("Authorization", auth)
	return r
}

func TestChecksumFormat(t *testing.T) {
	sum := Checksum("test-token")
	// base64(6 bytes)=8 chars + 64 hex + "/" + 64 hex
	if len(sum) != 8+64+1+64 {
		t.Fatalf("unexpected checksum length %d: %q", len(sum), sum)
	}
}

func TestHashed64Hex(t *testing.T) {
	got := Hashed64Hex("abc", "")
	if len(got) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(got))
	}
	if Hashed64Hex("abc", "salt") == got {
		t.Fatal("salt must change the hash")
	}
}

func TestGenerateChatBodyFraming(t *testing.T) {
	msgs := []OpenAIMessage{
		{Role: "system", Content: TextContent("be brief")},
		{Role: "user", Content: TextContent("hi")},
	}
	body, err := GenerateChatBody(msgs, "claude-3-7-sonnet")
	if err != nil {
		t.Fatal(err)
	}
	if len(body) < 5 {
		t.Fatal("body too short")
	}
	if body[0] != 0x00 {
		t.Fatalf("2 messages must not be gzipped, got magic %x", body[0])
	}
	l := binary.BigEndian.Uint32(body[1:5])
	if int(l) != len(body)-5 {
		t.Fatalf("length mismatch: header=%d actual=%d", l, len(body)-5)
	}
}

func TestGenerateChatBodyGzip(t *testing.T) {
	msgs := []OpenAIMessage{
		{Role: "user", Content: TextContent("a")},
		{Role: "assistant", Content: TextContent("b")},
		{Role: "user", Content: TextContent("c")},
	}
	body, err := GenerateChatBody(msgs, "claude-3-7-sonnet")
	if err != nil {
		t.Fatal(err)
	}
	if body[0] != 0x01 {
		t.Fatalf("3+ messages must be gzipped, got magic %x", body[0])
	}
	l := binary.BigEndian.Uint32(body[1:5])
	payload := body[5:]
	if int(l) != len(payload) {
		t.Fatalf("length mismatch: header=%d actual=%d", l, len(payload))
	}
	zr, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("payload is not valid gzip: %v", err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	// The raw proto must parse back.
	if _, err := parseFields(raw); err != nil {
		t.Fatalf("gunzipped proto doesn't parse: %v", err)
	}
}

func TestStreamChunkRoundTrip(t *testing.T) {
	// Build a fake response proto: message{ content="hello", thinking{content="th"} }
	var thinking []byte
	thinking = appendString(thinking, 1, "th")
	var message []byte
	message = appendString(message, 1, "hello")
	message = appendMessage(message, 25, thinking)
	var resp []byte
	resp = appendMessage(resp, 2, message)

	chunk, err := makeFrame(0x00, resp)
	if err != nil {
		t.Fatal(err)
	}
	th, text, err := ParseStreamChunk(chunk)
	if err != nil {
		t.Fatal(err)
	}
	if th != "th" || text != "hello" {
		t.Fatalf("got thinking=%q text=%q", th, text)
	}
}

func TestStreamChunkGzipRoundTrip(t *testing.T) {
	var message []byte
	message = appendString(message, 1, "world")
	var resp []byte
	resp = appendMessage(resp, 2, message)

	chunk, err := makeFrame(0x01, resp)
	if err != nil {
		t.Fatal(err)
	}
	_, text, err := ParseStreamChunk(chunk)
	if err != nil {
		t.Fatal(err)
	}
	if text != "world" {
		t.Fatalf("got %q", text)
	}
}

func TestDecodeAvailableModels(t *testing.T) {
	// AvailableModelsResponse{ models: [{name:"m1"},{name:"m2"}] }
	var m1, m2, resp []byte
	m1 = appendString(m1, 1, "m1")
	m2 = appendString(m2, 1, "m2")
	resp = appendMessage(resp, 2, m1)
	resp = appendMessage(resp, 2, m2)

	names, err := DecodeAvailableModels(resp)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "m1" || names[1] != "m2" {
		t.Fatalf("got %v", names)
	}
}

func TestExtractToken(t *testing.T) {
	cases := map[string]string{
		"user_1::token123":     "token123",
		"user_1%3A%3Atoken123": "token123",
		"raw-token":            "raw-token",
		"a::x, b::y":           "", // random pick, checked below
	}
	for in, want := range cases {
		r := httptestRequest("Bearer " + in)
		got := extractToken(r)
		if want == "" {
			if got != "x" && got != "y" {
				t.Fatalf("pool pick: got %q", got)
			}
			continue
		}
		if got != want {
			t.Fatalf("extractToken(%q) = %q, want %q", in, got, want)
		}
	}
}

func makeFrame(magic byte, payload []byte) ([]byte, error) {
	data := payload
	if magic == 0x01 || magic == 0x03 {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(payload); err != nil {
			return nil, err
		}
		if err := zw.Close(); err != nil {
			return nil, err
		}
		data = buf.Bytes()
	}
	out := []byte{magic, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(out[1:], uint32(len(data)))
	return append(out, data...), nil
}

func TestMessageContentShapes(t *testing.T) {
	var msg OpenAIMessage
	if err := json.Unmarshal([]byte(`{"role":"user","content":"hi"}`), &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Content.Text != "hi" {
		t.Fatalf("string content: got %q", msg.Content.Text)
	}

	if err := json.Unmarshal([]byte(
		`{"role":"user","content":[{"type":"text","text":"a"},{"type":"text","text":"b"}]}`), &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Content.Text != "a\nb" {
		t.Fatalf("array content: got %q", msg.Content.Text)
	}

	// Non-text parts carry no text and must not leak JSON into the prompt.
	if err := json.Unmarshal([]byte(
		`{"role":"user","content":[{"type":"image_url"},{"type":"text","text":"c"}]}`), &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Content.Text != "c" {
		t.Fatalf("mixed content: got %q", msg.Content.Text)
	}
}

// The prompt sent to Cursor must be the plain text, never a JSON encoding of
// the OpenAI content parts.
func TestGenerateChatBodySendsPlainText(t *testing.T) {
	body, err := GenerateChatBody([]OpenAIMessage{
		{Role: "user", Content: TextContent("hi")},
	}, "default")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte(`"type":"text"`)) || bytes.Contains(body, []byte(`[{`)) {
		t.Fatalf("body carries JSON-encoded content: %q", body)
	}
	if !bytes.Contains(body, []byte("hi")) {
		t.Fatalf("body missing prompt text: %q", body)
	}
}

func TestParseStreamChunkSurfacesEndStreamError(t *testing.T) {
	payload := []byte(`{"error":{"code":"resource_exhausted","message":"Error","details":[` +
		`{"debug":{"error":"ERROR_RATE_LIMITED_CHANGEABLE","details":` +
		`{"title":"Named models unavailable","detail":"Free plans can only use Auto."}}}]}}`)
	frame, err := makeFrame(0x02, payload)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = ParseStreamChunk(frame)
	if err == nil {
		t.Fatal("expected an error from the end-of-stream frame")
	}
	var se *StreamError
	if !errors.As(err, &se) {
		t.Fatalf("expected *StreamError, got %T", err)
	}
	if se.Reason != "ERROR_RATE_LIMITED_CHANGEABLE" || se.Code != "resource_exhausted" {
		t.Fatalf("got %+v", se)
	}
	if !strings.Contains(err.Error(), "Free plans can only use Auto.") {
		t.Fatalf("error message lost the detail: %v", err)
	}
}

func TestParseStreamChunkEmptyEndStreamIsNotAnError(t *testing.T) {
	frame, err := makeFrame(0x02, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ParseStreamChunk(frame); err != nil {
		t.Fatalf("clean end-of-stream reported an error: %v", err)
	}
}

func TestReadFramesReturnsStreamError(t *testing.T) {
	var message []byte
	message = appendString(message, 1, "hello")
	var resp []byte
	resp = appendMessage(resp, 2, message)
	dataFrame, err := makeFrame(0x00, resp)
	if err != nil {
		t.Fatal(err)
	}
	endFrame, err := makeFrame(0x02, []byte(`{"error":{"code":"unauthenticated","message":"User is unauthorized"}}`))
	if err != nil {
		t.Fatal(err)
	}

	var got string
	err = readFrames(bytes.NewReader(append(dataFrame, endFrame...)), func(_, text string) { got += text })
	if got != "hello" {
		t.Fatalf("content before the error was dropped: %q", got)
	}
	var se *StreamError
	if !errors.As(err, &se) || se.Code != "unauthenticated" {
		t.Fatalf("got %v", err)
	}
	if status, _ := errorStatus(err); status != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", status)
	}
}

func TestThinkingTagger(t *testing.T) {
	var tg thinkingTagger
	var b strings.Builder
	b.WriteString(tg.next("re", ""))
	b.WriteString(tg.next("asoning", ""))
	b.WriteString(tg.next("", "answer"))
	b.WriteString(tg.close())
	if got := b.String(); got != "<thinking>\nreasoning\n</thinking>\nanswer" {
		t.Fatalf("got %q", got)
	}

	// A stream that never leaves the thinking block must still be closed.
	var tg2 thinkingTagger
	out := tg2.next("only", "") + tg2.close()
	if out != "<thinking>\nonly\n</thinking>\n" {
		t.Fatalf("got %q", out)
	}

	// Plain text must pass through untouched.
	var tg3 thinkingTagger
	if out := tg3.next("", "plain") + tg3.close(); out != "plain" {
		t.Fatalf("got %q", out)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	s, err := NewServer(Config{Port: 0}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Shutdown(context.Background()) })
	return s
}

func TestServerBindsEphemeralPort(t *testing.T) {
	s := newTestServer(t)
	if s.Port() == 0 {
		t.Fatal("port not assigned")
	}
	if want := fmt.Sprintf("http://127.0.0.1:%d/v1", s.Port()); s.BaseURL() != want {
		t.Fatalf("BaseURL = %q, want %q", s.BaseURL(), want)
	}
	// A second server must not collide with the first.
	if other := newTestServer(t); other.Port() == s.Port() {
		t.Fatalf("both servers bound port %d", s.Port())
	}
}

// The SSE stream must be well formed: a role delta, the content, an explicit
// finish_reason and [DONE]. A bare content chunk is what made downstream
// clients report "stream ended without receiving any events".
func TestStreamResponseSSEShape(t *testing.T) {
	var message []byte
	message = appendString(message, 1, "hello")
	var resp []byte
	resp = appendMessage(resp, 2, message)
	data, err := makeFrame(0x00, resp)
	if err != nil {
		t.Fatal(err)
	}
	end, err := makeFrame(0x02, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	newTestServer(t).streamResponse(rec, bytes.NewReader(append(data, end...)), "default")

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, body)
	}
	for _, want := range []string{`"role":"assistant"`, `"content":"hello"`, `"finish_reason":"stop"`, "data: [DONE]"} {
		if !strings.Contains(body, want) {
			t.Fatalf("SSE missing %s:\n%s", want, body)
		}
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}
}

// When Cursor rejects the request the caller must get a real HTTP error with
// the upstream message, not an empty 200 stream.
func TestStreamResponseUpstreamErrorBecomesHTTPError(t *testing.T) {
	end, err := makeFrame(0x02, []byte(`{"error":{"code":"resource_exhausted","message":"Error","details":[`+
		`{"debug":{"error":"ERROR_RATE_LIMITED_CHANGEABLE","details":{"detail":"Free plans can only use Auto."}}}]}}`))
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	newTestServer(t).streamResponse(rec, bytes.NewReader(end), "claude-4.5-sonnet")

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Free plans can only use Auto.") {
		t.Fatalf("upstream detail not surfaced: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "[DONE]") {
		t.Fatalf("error response must not be an SSE stream: %s", rec.Body.String())
	}
}
