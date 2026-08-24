package cursor

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"
)

// Hashed64Hex returns the sha256 hex digest of input+salt.
// Used for x-client-key and machine IDs.
func Hashed64Hex(input, salt string) string {
	h := sha256.Sum256([]byte(input + salt))
	return hex.EncodeToString(h[:])
}

// obfuscateBytes mirrors the byte obfuscation from the Cursor client
// used inside the x-cursor-checksum value.
func obfuscateBytes(b []byte) {
	t := byte(165)
	for i := range b {
		b[i] = (b[i] ^ t) + byte(i%256)
		t = b[i]
	}
}

// Checksum builds the x-cursor-checksum header value for the given token.
//
// The timestamp block must match the Cursor client byte for byte or the API
// answers every chat request with ERROR_UNAUTHORIZED. Two details are easy to
// get wrong: the counter is floor(unixMillis/1e6) — a ~16.7 minute tick, not a
// second counter — and the six bytes are not a plain big-endian encoding, they
// repeat the low half of the counter in the client's own order.
func Checksum(token string) string {
	machineID := Hashed64Hex(token, "machineId")
	macMachineID := Hashed64Hex(token, "macMachineId")

	ts := uint32(time.Now().UnixMilli() / 1e6)
	b := []byte{
		byte(ts >> 8),
		byte(ts),
		byte(ts >> 24),
		byte(ts >> 16),
		byte(ts >> 8),
		byte(ts),
	}
	obfuscateBytes(b)
	return base64.StdEncoding.EncodeToString(b) + machineID + "/" + macMachineID
}
