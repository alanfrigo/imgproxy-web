package client

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// Signer computes imgproxy URL signatures (HMAC-SHA256, base64url, no padding).
// Mirrors imgproxy's security/signature.go: HMAC(salt || path, key)[:size].
type Signer struct {
	keys  [][]byte
	salts [][]byte
	size  int
}

// NewSigner parses space-separated hex strings (matching IMGPROXY_KEY/IMGPROXY_SALT
// envs). Returns nil if either is empty (signature disabled).
func NewSigner(keysHex, saltsHex string, size int) (*Signer, error) {
	keysHex = strings.TrimSpace(keysHex)
	saltsHex = strings.TrimSpace(saltsHex)
	if keysHex == "" || saltsHex == "" {
		return nil, nil
	}
	keyParts := strings.Fields(keysHex)
	saltParts := strings.Fields(saltsHex)
	if len(keyParts) != len(saltParts) {
		return nil, fmt.Errorf("key/salt count mismatch: %d keys vs %d salts", len(keyParts), len(saltParts))
	}
	keys := make([][]byte, len(keyParts))
	salts := make([][]byte, len(saltParts))
	for i, k := range keyParts {
		b, err := hex.DecodeString(k)
		if err != nil {
			return nil, fmt.Errorf("decode key %d: %w", i, err)
		}
		keys[i] = b
	}
	for i, s := range saltParts {
		b, err := hex.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("decode salt %d: %w", i, err)
		}
		salts[i] = b
	}
	if size <= 0 || size > 32 {
		size = 32
	}
	return &Signer{keys: keys, salts: salts, size: size}, nil
}

// Sign returns the base64url signature for the given path (e.g. "/rs:fit:800/plain/...").
// Path must start with "/". Uses the first configured key/salt pair.
func (s *Signer) Sign(path string) string {
	if s == nil {
		return "_"
	}
	mac := hmac.New(sha256.New, s.keys[0])
	mac.Write(s.salts[0])
	mac.Write([]byte(path))
	sum := mac.Sum(nil)
	if s.size < 32 {
		sum = sum[:s.size]
	}
	return base64.RawURLEncoding.EncodeToString(sum)
}
