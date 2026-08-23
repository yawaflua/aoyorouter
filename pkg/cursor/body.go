package cursor

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
)

// GenerateChatBody builds the Connect-RPC framed body for
// aiserver.v1.ChatService/StreamUnifiedChatWithTools from OpenAI-style
// messages. When there are 3+ messages the protobuf payload is gzipped
// (envelope flag 0x01), matching the original client behaviour.
func GenerateChatBody(messages []OpenAIMessage, modelName string) ([]byte, error) {
	var instruction string
	var formatted []ChatMessage
	for _, m := range messages {
		if m.Role == "system" {
			if instruction != "" {
				instruction += "\n"
			}
			instruction += m.Content
			continue
		}
		cm := ChatMessage{
			Content:   m.Content,
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

// ParseStreamChunk parses Connect-RPC framed data received from the stream
// endpoint and returns concatenated thinking/text content.
// Frames: 1 magic byte (0=proto, 1=gzip proto, 2=json, 3=gzip json) +
// 4-byte big-endian length + payload.
func ParseStreamChunk(chunk []byte) (thinking, text string, err error) {
	var thinkingOut, textOut bytes.Buffer
	for i := 0; i+5 <= len(chunk); {
		magic := chunk[i]
		dataLen := int(binary.BigEndian.Uint32(chunk[i+1 : i+5]))
		if dataLen < 0 || i+5+dataLen > len(chunk) {
			break
		}
		data := chunk[i+5 : i+5+dataLen]

		switch magic {
		case 0, 1:
			payload := data
			if magic == 1 {
				zr, zerr := gzip.NewReader(bytes.NewReader(data))
				if zerr != nil {
					i += 5 + dataLen
					continue
				}
				payload, zerr = io.ReadAll(zr)
				zr.Close()
				if zerr != nil {
					i += 5 + dataLen
					continue
				}
			}
			resp, derr := DecodeStreamUnifiedChatResponse(payload)
			if derr == nil && resp != nil {
				thinkingOut.WriteString(resp.Thinking)
				textOut.WriteString(resp.Content)
			}
		case 2, 3:
			// JSON control messages — ignored (logged upstream in the JS version).
		}
		i += 5 + dataLen
	}
	return thinkingOut.String(), textOut.String(), nil
}

// OpenAIMessage is a minimal OpenAI chat message (string content only).
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

var _ = fmt.Sprintf // reserved
