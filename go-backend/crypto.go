package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5" //nolint:gosec
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
)

// MD5 returns the hex-encoded MD5 hash of the input string
func MD5(s string) string {
	h := md5.New() //nolint:gosec
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// AESCrypto provides AES-256-GCM encryption/decryption compatible with the Java implementation
type AESCrypto struct {
	key []byte
}

// NewAESCrypto creates an AES crypto instance using SHA-256 of the secret as the key
func NewAESCrypto(secret string) *AESCrypto {
	h := sha256.Sum256([]byte(secret))
	return &AESCrypto{key: h[:]}
}

// Encrypt encrypts data using AES-256-GCM, returning base64(nonce+ciphertext)
func (a *AESCrypto) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(a.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize()) // 12 bytes
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	// Format: nonce + ciphertext (tag is appended by GCM)
	combined := make([]byte, len(nonce)+len(ciphertext))
	copy(combined[:len(nonce)], nonce)
	copy(combined[len(nonce):], ciphertext)

	return base64.StdEncoding.EncodeToString(combined), nil
}

// Decrypt decrypts base64(nonce+ciphertext) using AES-256-GCM
func (a *AESCrypto) Decrypt(encryptedBase64 string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encryptedBase64)
	if err != nil {
		return "", fmt.Errorf("base64 decode error: %w", err)
	}

	block, err := aes.NewCipher(a.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize() // 12
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("gcm decrypt error: %w", err)
	}

	return string(plaintext), nil
}
