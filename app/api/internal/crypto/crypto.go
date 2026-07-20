// Package crypto provides AES-256-GCM encryption helpers for data at rest,
// shared by the Google OAuth token store and the runtime settings store.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// DecodeKey decodes a base64-encoded 32-byte encryption key. An empty string
// yields a nil key, which disables encryption (development mode).
func DecodeKey(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode encryption key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes (got %d)", len(key))
	}
	return key, nil
}

// Encrypt seals plaintext with AES-256-GCM using key. When key is empty the
// plaintext is returned unchanged so the app still works without a configured
// encryption key (development mode).
func Encrypt(key, plaintext []byte) ([]byte, error) {
	if len(key) == 0 {
		return plaintext, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt reverses Encrypt. When key is empty the ciphertext is returned
// unchanged (development mode).
func Decrypt(key, ciphertext []byte) ([]byte, error) {
	if len(key) == 0 {
		return ciphertext, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return aead.Open(nil, nonce, ct, nil)
}
