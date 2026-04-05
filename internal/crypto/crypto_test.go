package crypto

import (
	"testing"
)

func TestRoundTrip(t *testing.T) {
	// 32-byte key as 64 hex chars
	if err := Init("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	plaintext := []byte("AIzaSyTestKey12345")
	blob, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if blob == "" {
		t.Fatal("Encrypt returned empty blob")
	}

	got, err := Decrypt(blob)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(got) != string(plaintext) {
		t.Errorf("got %q, want %q", got, plaintext)
	}
}

func TestInitBadKey(t *testing.T) {
	// Reset for this test
	aead = nil

	if err := Init("tooshort"); err == nil {
		t.Fatal("expected error for short key")
	}
	if Enabled() {
		t.Fatal("should not be enabled after bad Init")
	}
}

func TestEncryptNotInitialised(t *testing.T) {
	aead = nil
	if _, err := Encrypt([]byte("test")); err == nil {
		t.Fatal("expected error when not initialised")
	}
}

func TestDecryptBadBlob(t *testing.T) {
	if err := Init("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := Decrypt("not-valid-base64!!!"); err == nil {
		t.Fatal("expected error for bad base64")
	}
	if _, err := Decrypt("AAAA"); err == nil {
		t.Fatal("expected error for short ciphertext")
	}
}
