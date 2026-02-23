package secretcrypto

import (
	"bytes"
	"testing"
)

func TestAESCipher_RoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	rnd := bytes.NewReader(bytes.Repeat([]byte{0x01}, 64))
	c, err := NewAESCipherWithRandom(key, rnd)
	if err != nil {
		t.Fatalf("NewAESCipherWithRandom: %v", err)
	}

	ct, err := c.Encrypt("secret-value")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	pt, err := c.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if pt != "secret-value" {
		t.Fatalf("unexpected plaintext: %q", pt)
	}
}

func TestAESCipher_InvalidCiphertext(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	c, err := NewAESCipher(key)
	if err != nil {
		t.Fatalf("NewAESCipher: %v", err)
	}
	if _, err := c.Decrypt("invalid"); err == nil {
		t.Fatalf("expected decrypt error")
	}
}
