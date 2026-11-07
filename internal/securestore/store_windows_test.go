//go:build windows

package securestore

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDPAPIAndLegacyV10(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	plain := []byte(`{"refreshToken":"synthetic-token-only","name":"Тест"}`)
	encrypted, err := s.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, plain) {
		t.Fatal("plaintext credential")
	}
	decoded, err := s.Decrypt(encrypted)
	if err != nil || !bytes.Equal(decoded, plain) {
		t.Fatalf("roundtrip %v", err)
	}
	key := make([]byte, 32)
	rand.Read(key)
	wrapped, err := protect(key)
	if err != nil {
		t.Fatal(err)
	}
	state, _ := json.Marshal(map[string]any{"os_crypt": map[string]any{"encrypted_key": base64.StdEncoding.EncodeToString(append([]byte("DPAPI"), wrapped...))}})
	os.WriteFile(filepath.Join(dir, "Local State"), state, 0600)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, 12)
	rand.Read(nonce)
	legacy := append(append([]byte("v10"), nonce...), gcm.Seal(nil, nonce, plain, nil)...)
	decoded, err = s.Decrypt(legacy)
	if err != nil || !bytes.Equal(decoded, plain) {
		t.Fatalf("legacy v10 compatibility %v", err)
	}
	legacy[len(legacy)-1] ^= 1
	if _, err = s.Decrypt(legacy); err == nil {
		t.Fatal("tampered legacy accepted")
	}
	if _, err = s.Decrypt([]byte("v10")); err == nil {
		t.Fatal("short legacy accepted")
	}
}
