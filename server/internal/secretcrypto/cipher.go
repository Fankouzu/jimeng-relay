package secretcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const ciphertextPrefix = "v1:"

type Cipher interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

type AESCipher struct {
	aead cipher.AEAD
	rnd  io.Reader
}

func NewAESCipher(key []byte) (*AESCipher, error) {
	return NewAESCipherWithRandom(key, rand.Reader)
}

func NewAESCipherWithRandom(key []byte, rnd io.Reader) (*AESCipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("aes-256-gcm key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create aes block: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create aes-gcm: %w", err)
	}
	if rnd == nil {
		rnd = rand.Reader
	}
	return &AESCipher{aead: aead, rnd: rnd}, nil
}

func (c *AESCipher) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(c.rnd, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	sealed := c.aead.Seal(nil, nonce, []byte(plaintext), nil)
	combined := append(nonce, sealed...)
	return ciphertextPrefix + base64.StdEncoding.EncodeToString(combined), nil
}

func (c *AESCipher) Decrypt(ciphertext string) (string, error) {
	v := strings.TrimSpace(ciphertext)
	if !strings.HasPrefix(v, ciphertextPrefix) {
		return "", fmt.Errorf("ciphertext prefix is invalid")
	}
	enc := strings.TrimPrefix(v, ciphertextPrefix)
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	nonceSize := c.aead.NonceSize()
	if len(raw) < nonceSize {
		return "", fmt.Errorf("ciphertext is too short")
	}
	nonce := raw[:nonceSize]
	payload := raw[nonceSize:]
	plain, err := c.aead.Open(nil, nonce, payload, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt ciphertext: %w", err)
	}
	return string(plain), nil
}
