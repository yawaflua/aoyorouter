package cursor

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Connect envelope flag bits, as used by api2.cursor.sh.
const (
	// flagCompressed marks a payload compressed with connect-content-encoding.
	flagCompressed = 0x01
	// flagEndStream marks the trailing frame, whose payload is JSON and
	// carries the error (if any) that terminated the stream.
	flagEndStream = 0x02
)

// GenerateChatBody builds the Connect-RPC framed body for
// aiserver.v1.ChatService/StreamUnifiedChatWithTools from OpenAI-style
// messages. When there are 3+ messages the protobuf payload is gzipped
// (envelope flag 0x01), matching the original client behaviour.
func GenerateChatBody(messages []OpenAIMessage, modelName string) ([]byte, error) {
	var instruction string
	var formatted []ChatMessage
	for _, m := range messages {
		text := m.Content.Text
		if m.Role == "system" {
			if instruction != "" {
				instruction += "\n"
			}
			instruction += text
			continue
		}
		cm := ChatMessage{
			Content:   text,
			MessageID: uuid.NewString(),
		}
		if m.Role == "user" {
			cm.Role = 1
			cm.ChatModeEnum = 1
		} else {
			cm.Role = 2
		}
		formatted = append(formatted, cm)
	}

	refs := make([]MessageIDRef, 0, len(formatted))
	for _, m := range formatted {
		refs = append(refs, MessageIDRef{
			MessageID: m.MessageID,
			SummaryID: m.SummaryID,
			Role:      m.Role,
		})
	}

	req := &ChatRequest{
		Messages:       formatted,
		Unknown2:       1,
		Instruction:    instruction,
		Unknown4:       1,
		ModelName:      modelName,
		WebTool:        "",
		Unknown13:      1,
		Unknown19:      1,
		ConversationID: uuid.NewString(),
		Metadata: &RequestMetadata{
			OS:        "win32",
			Arch:      "x64",
			Version:   "10.0.22631",
			Path:      `C:\Program Files\PowerShell\7\pwsh.exe`,
			Timestamp: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		},
		Unknown27:    0,
		MessageIDs:   refs,
		LargeContext: 0,
		Unknown38:    0,
		ChatModeEnum: 1,
		Unknown53:    1,
		ChatMode:     "Ask",
	}

	payload, err := EncodeStreamUnifiedChatRequest(req)
	if err != nil {
		return nil, err
	}

	flag := byte(0x00)
	if len(formatted) >= 3 {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(payload); err != nil {
			return nil, err
		}
		if err := zw.Close(); err != nil {
			return nil, err
		}
		payload = buf.Bytes()
		flag = 0x01
	}

	out := make([]byte, 0, len(payload)+5)
	out = append(out, flag)
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(payload)))
	out = append(out, lenBuf[:]...)
	out = append(out, payload...)
	return out, nil
}

// decodeFramePayload gunzips a frame payload when the compressed flag is set.
func decodeFramePayload(flag byte, data []byte) ([]byte, error) {
	if flag&flagCompressed == 0 || len(data) == 0 {
		return data, nil
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}

// StreamError is an error Cursor reported in the Connect end-of-stream frame.
// The API answers with HTTP 200 and describes the failure only here, so this
// is the sole place a rejected request becomes visible.
type StreamError struct {
	// Code is the Connect status, e.g. "resource_exhausted".
	Code string
	// Reason is the Cursor-specific code, e.g. "ERROR_RATE_LIMITED_CHANGEABLE".
	Reason string
	// Title and Detail are the user-facing strings Cursor would render.
	Title  string
	Detail string
}

func (e *StreamError) Error() string {
	parts := make([]string, 0, 3)
	if e.Reason != "" {
		parts = append(parts, e.Reason)
	} else if e.Code != "" {
		parts = append(parts, e.Code)
	}
	switch {
	case e.Detail != "":
		parts = append(parts, e.Detail)
	case e.Title != "":
		parts = append(parts, e.Title)
	}
	if len(parts) == 0 {
		return "cursor: stream failed"
	}
	if hint := reasonHint[e.Reason]; hint != "" {
		parts = append(parts, hint)
	}
	return "cursor: " + strings.Join(parts, ": ")
}

// reasonHint annotates Cursor codes whose own message points the wrong way.
var reasonHint = map[string]string{
	// Cursor reuses this bucket for Auto and the composer-* models, and its
	// text tells you to update the client. Bumping x-cursor-client-version
	// changes nothing: those models moved to a request schema this package
	// does not encode. Pick a named model instead.
	"ERROR_GPT_4_VISION_PREVIEW_RATE_LIMIT": "(this bridge cannot drive Auto/composer-* models; use a named model)",
}

// endStreamFrame is the JSON payload of a flagEndStream frame.
type endStreamFrame struct {
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details []struct {
			Debug struct {
				Error   string `json:"error"`
				Details struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				} `json:"details"`
			} `json:"debug"`
		} `json:"details"`
	} `json:"error"`
}

// parseEndStream returns the error carried by an end-of-stream frame, or nil
// when the stream finished normally.
func parseEndStream(payload []byte) error {
	if len(bytes.TrimSpace(payload)) == 0 {
		return nil
	}
	var frame endStreamFrame
	if err := json.Unmarshal(payload, &frame); err != nil {
		// Not JSON we recognise; surface it verbatim rather than dropping it.
		return &StreamError{Detail: string(payload)}
	}
	if frame.Error == nil {
		return nil
	}
	se := &StreamError{Code: frame.Error.Code, Detail: frame.Error.Message}
	if len(frame.Error.Details) > 0 {
		d := frame.Error.Details[0].Debug
		se.Reason = d.Error
		se.Title = d.Details.Title
		if d.Details.Detail != "" {
			se.Detail = d.Details.Detail
		}
	}
	return se
}

// ParseStreamChunk parses Connect-RPC framed data received from the stream
// endpoint and returns concatenated thinking/text content. Frames are one
// flag byte + 4-byte big-endian length + payload; bit 0 marks a gzipped
// payload and bit 1 marks the end-of-stream frame. It returns a [StreamError]
// when the stream ended with an error.
func ParseStreamChunk(chunk []byte) (thinking, text string, err error) {
	var thinkingOut, textOut bytes.Buffer
	for i := 0; i+5 <= len(chunk); {
		flag := chunk[i]
		dataLen := int(binary.BigEndian.Uint32(chunk[i+1 : i+5]))
		if dataLen < 0 || i+5+dataLen > len(chunk) {
			break
		}
		data := chunk[i+5 : i+5+dataLen]
		i += 5 + dataLen

		payload, derr := decodeFramePayload(flag, data)
		if derr != nil {
			continue
		}
		if flag&flagEndStream != 0 {
			if serr := parseEndStream(payload); serr != nil {
				return thinkingOut.String(), textOut.String(), serr
			}
			continue
		}
		resp, derr := DecodeStreamUnifiedChatResponse(payload)
		if derr == nil && resp != nil {
			thinkingOut.WriteString(resp.Thinking)
			textOut.WriteString(resp.Content)
		}
	}
	return thinkingOut.String(), textOut.String(), nil
}

// OpenAIMessage is an OpenAI chat message.
type OpenAIMessage struct {
	Role    string         `json:"role"`
	Content MessageContent `json:"content"`
}

// OpenAIContent is one entry of the structured content-part array.
type OpenAIContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// MessageContent accepts both OpenAI content shapes: the plain string
// ("content": "hi") and the content-part array ("content": [{"type":"text",
// "text":"hi"}]). Cursor only ever consumes plain text, so Text holds the
// flattened form; Parts keeps the original array when one was sent.
type MessageContent struct {
	Text  string
	Parts []OpenAIContent
}

// UnmarshalJSON implements [json.Unmarshaler].
func (c *MessageContent) UnmarshalJSON(data []byte) error {
	// Reset: encoding/json reuses the receiver, and a partially populated
	// slice would keep fields the new payload omits.
	*c = MessageContent{}
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return nil
	}
	switch data[0] {
	case '"':
		return json.Unmarshal(data, &c.Text)
	case '[':
		if err := json.Unmarshal(data, &c.Parts); err != nil {
			return err
		}
		var b strings.Builder
		for _, p := range c.Parts {
			// Non-text parts (images, audio) carry no Text and are dropped:
			// the Cursor chat request has no field to put them in.
			if p.Text == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(p.Text)
		}
		c.Text = b.String()
		return nil
	default:
		return fmt.Errorf("cursor: unsupported message content type %q", data[0])
	}
}

// MarshalJSON implements [json.Marshaler], round-tripping the original shape.
func (c MessageContent) MarshalJSON() ([]byte, error) {
	if c.Parts != nil {
		return json.Marshal(c.Parts)
	}
	return json.Marshal(c.Text)
}

// TextContent builds a MessageContent from a plain string.
func TextContent(text string) MessageContent { return MessageContent{Text: text} }
