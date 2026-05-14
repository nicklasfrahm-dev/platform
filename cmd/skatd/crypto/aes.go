// Package crypto provides encryption primitives for skatd secret values.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// Encryptor encrypts and decrypts byte slices.
type Encryptor interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}

// NewAES256GCM creates an Encryptor keyed from a passphrase via SHA-256.
// The nonce (12 bytes) is prepended to each ciphertext, so identical plaintexts
// produce different ciphertexts on every call.
func NewAES256GCM(passphrase string) (Encryptor, error) {
	sum := sha256.Sum256([]byte(passphrase))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	return &aesGCM{gcm: gcm}, nil
}

type aesGCM struct {
	gcm cipher.AEAD
}

func (a *aesGCM) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, a.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return a.gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func (a *aesGCM) Decrypt(data []byte) ([]byte, error) {
	ns := a.gcm.NonceSize()
	if len(data) < ns {
		return nil, errors.New("ciphertext too short")
	}
	pt, err := a.gcm.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return pt, nil
}

// noop is a pass-through Encryptor used when no ENCRYPTION_KEY is configured.
type noop struct{}

// NewNoop returns an Encryptor that stores values unmodified.
func NewNoop() Encryptor { return noop{} }

func (noop) Encrypt(plaintext []byte) ([]byte, error) { return plaintext, nil }
func (noop) Decrypt(ciphertext []byte) ([]byte, error) { return ciphertext, nil }

// EncryptString encrypts a string and returns a base64-encoded result.
func EncryptString(enc Encryptor, s string) (string, error) {
	ct, err := enc.Encrypt([]byte(s))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ct), nil
}

// DecryptString base64-decodes and decrypts a string previously encrypted by EncryptString.
func DecryptString(enc Encryptor, s string) (string, error) {
	ct, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	pt, err := enc.Decrypt(ct)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
