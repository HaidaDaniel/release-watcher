package id

import (
	"crypto/rand"
	"encoding/hex"
)

// New returns a random prefixed identifier such as watch_9f2c....
func New(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("release-watcher: crypto/rand failed: " + err.Error())
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}
