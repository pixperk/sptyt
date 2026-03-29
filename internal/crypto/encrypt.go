package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
)

var encryptionKey []byte

func init() {
	key := os.Getenv("TOKEN_ENCRYPTION_KEY")
	if key != "" {
		decoded, err := base64.StdEncoding.DecodeString(key)
		if err != nil || len(decoded) != 32 {
			panic("TOKEN_ENCRYPTION_KEY must be a base64-encoded 32-byte key")
		}
		encryptionKey = decoded
	}
}

// Enabled returns whether token encryption is configured.
func Enabled() bool {
	return len(encryptionKey) == 32
}

// Encrypt encrypts plaintext using AES-256-GCM. Returns base64-encoded ciphertext.
func Encrypt(plaintext string) (string, error) {
	if !Enabled() {
		return plaintext, nil
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return "enc:" + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts base64-encoded AES-256-GCM ciphertext. Handles unencrypted plaintext gracefully.
func Decrypt(ciphertext string) (string, error) {
	if !Enabled() {
		return ciphertext, nil
	}

	// Handle unencrypted legacy tokens
	if len(ciphertext) < 4 || ciphertext[:4] != "enc:" {
		return ciphertext, nil
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext[4:])
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}
