package google_photo_native

import (
	"crypto/sha1"
	"encoding/hex"
	"io"
	"os"
)

// sha1OfFile hashes a file at path and returns hex-encoded SHA-1. Used only
// for the disguise-wrapper path where the wrapper is materialized to disk
// after CacheFullAndHash has already hashed the payload.
func sha1OfFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
