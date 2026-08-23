package cursor

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io"
	"net/http"
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
		{Role: "system", Content: "be brief"},
		{Role: "user", Content: "hi"},
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
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "b"},
		{Role: "user", Content: "c"},
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
