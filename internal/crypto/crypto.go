package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

var aead interface {
	NonceSize() int
	Seal(dst, nonce, plaintext, additionalData []byte) []byte
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
}

// Init decodes a hex-encoded 32-byte key and initialises the package-level
// XChaCha20-Poly1305 AEAD. Call once at startup.
func Init(hexKey string) error {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return fmt.Errorf("crypto: invalid hex key: %w", err)
	}
	if len(key) != chacha20poly1305.KeySize {
		return fmt.Errorf("crypto: key must be %d bytes, got %d", chacha20poly1305.KeySize, len(key))
	}
	a, err := chacha20poly1305.NewX(key)
	if err != nil {
		return fmt.Errorf("crypto: %w", err)
	}
	aead = a
	return nil
}

// Enabled reports whether Init has been called successfully.
func Enabled() bool { return aead != nil }

// Encrypt seals plaintext and returns a base64-encoded blob (nonce || ciphertext).
func Encrypt(plaintext []byte) (string, error) {
	if aead == nil {
		return "", errors.New("crypto: not initialised")
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("crypto: nonce: %w", err)
	}
	sealed := aead.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt: base64-decodes, splits nonce and ciphertext, opens.
func Decrypt(blob string) ([]byte, error) {
	if aead == nil {
		return nil, errors.New("crypto: not initialised")
	}
	raw, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		return nil, fmt.Errorf("crypto: base64: %w", err)
	}
	if len(raw) < aead.NonceSize() {
		return nil, errors.New("crypto: ciphertext too short")
	}
	nonce, ciphertext := raw[:aead.NonceSize()], raw[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return plaintext, nil
}
