package id

import (
	"crypto/rand"
	"encoding/hex"
)

func New(prefix string) string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return prefix + "_" + hex.EncodeToString(raw[:])
}
