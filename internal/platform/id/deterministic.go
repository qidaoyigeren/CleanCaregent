package id

import (
	"crypto/sha256"
	"encoding/hex"
)

func DeterministicUUID(value string) string {
	sum := sha256.Sum256([]byte(value))
	bytes := sum[:16]
	bytes[6] = (bytes[6] & 0x0f) | 0x50
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	raw := hex.EncodeToString(bytes)
	return raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:32]
}
