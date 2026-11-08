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
	"strings"
	"testing"
)

func legacyLayoutFixture(t *testing.T, version string) ([]byte, []byte, []byte) {
	t.Helper()
	plain := []byte(`{"clientId":"synthetic-public-client","refreshToken":"synthetic-only","config":{"apiKey":"synthetic-only"}}`)
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	wrapped, err := protect(key)
	if err != nil {
		t.Fatal(err)
	}
	state, err := json.Marshal(map[string]any{"os_crypt": map[string]any{"encrypted_key": base64.StdEncoding.EncodeToString(append([]byte("DPAPI"), wrapped...))}})
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	blob := append(append([]byte(version), nonce...), gcm.Seal(nil, nonce, plain, nil)...)
	return state, blob, plain
}
func legacyLayoutWrite(t *testing.T, dir, relative string, body []byte) {
	t.Helper()
	file := filepath.Join(dir, relative)
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, body, 0600); err != nil {
		t.Fatal(err)
	}
}
func TestLegacyBothProfileLayoutsUseRealWindowsDPAPI(t *testing.T) {
	for _, version := range []string{"v10", "v11"} {
		for _, layout := range []string{"Local State", filepath.Join("electron", "Local State")} {
			t.Run(version+"/"+layout, func(t *testing.T) {
				dir := t.TempDir()
				state, blob, plain := legacyLayoutFixture(t, version)
				legacyLayoutWrite(t, dir, layout, state)
				legacyLayoutWrite(t, dir, "frontier-tokens.enc", blob)
				original := append([]byte(nil), blob...)
				decoded, err := New(dir).Decrypt(blob)
				if err != nil || !bytes.Equal(decoded, plain) {
					t.Fatalf("legacy Windows profile import failed: %v", err)
				}
				clear(decoded)
				if !bytes.Equal(blob, original) {
					t.Fatal("decrypt modified its encrypted input")
				}
				onDisk, err := os.ReadFile(filepath.Join(dir, "frontier-tokens.enc"))
				if err != nil || !bytes.Equal(onDisk, original) {
					t.Fatal("original credential file changed")
				}
				onDisk, err = os.ReadFile(filepath.Join(dir, layout))
				if err != nil || !bytes.Equal(onDisk, state) {
					t.Fatal("original Local State changed")
				}
				if _, err = os.Stat(filepath.Join(dir, "frontier-go-tokens.enc")); !os.IsNotExist(err) {
					t.Fatal("decrypt unexpectedly created a migrated token file")
				}
			})
		}
	}
}
func TestLegacyProfileFallbackAuthenticatesTheMatchingKey(t *testing.T) {
	for _, invalidFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "different-valid-key", true: "malformed-electron-state"}[invalidFirst], func(t *testing.T) {
			dir := t.TempDir()
			matching, blob, plain := legacyLayoutFixture(t, "v10")
			other, _, _ := legacyLayoutFixture(t, "v10")
			if invalidFirst {
				other = []byte("malformed synthetic Local State")
			}
			legacyLayoutWrite(t, dir, filepath.Join("electron", "Local State"), other)
			legacyLayoutWrite(t, dir, "Local State", matching)
			decoded, err := New(dir).Decrypt(blob)
			if err != nil || !bytes.Equal(decoded, plain) {
				t.Fatalf("authenticated root-layout fallback failed: %v", err)
			}
			clear(decoded)
		})
	}
}
func TestLegacyMissingMalformedOversizedAndTamperedProfilesPreserveOriginals(t *testing.T) {
	valid, blob, _ := legacyLayoutFixture(t, "v10")
	for name, state := range map[string][]byte{"missing": nil, "invalid-json": []byte("{"), "missing-key": []byte(`{"os_crypt":{}}`), "invalid-base64": []byte(`{"os_crypt":{"encrypted_key":"!"}}`), "unprotected-key": []byte(`{"os_crypt":{"encrypted_key":"bm90LWRwYXBp"}}`), "invalid-dpapi": []byte(`{"os_crypt":{"encrypted_key":"RFBBUEl4"}}`), "oversized": bytes.Repeat([]byte("x"), 4*1024*1024+1), "tampered": valid} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if state != nil {
				legacyLayoutWrite(t, dir, filepath.Join("electron", "Local State"), state)
			}
			encrypted := append([]byte(nil), blob...)
			if name == "tampered" {
				encrypted[len(encrypted)-1] ^= 1
			}
			legacyLayoutWrite(t, dir, "integration-state.bin", encrypted)
			if decoded, err := New(dir).Decrypt(encrypted); err == nil || len(decoded) != 0 {
				clear(decoded)
				t.Fatal("invalid protected profile accepted")
			}
			after, err := os.ReadFile(filepath.Join(dir, "integration-state.bin"))
			if err != nil || !bytes.Equal(after, encrypted) {
				t.Fatal("failed migration changed the original encrypted file")
			}
			if state != nil {
				after, err = os.ReadFile(filepath.Join(dir, "electron", "Local State"))
				if err != nil || !bytes.Equal(after, state) {
					t.Fatal("failed migration changed Local State")
				}
			}
			if _, err = os.Stat(filepath.Join(dir, "integration-go-state.bin")); !os.IsNotExist(err) {
				t.Fatal("failed decrypt created a replacement credential file")
			}
		})
	}
}
func TestLegacyLocalStateCannotEscapeProfileRoot(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	state, blob, _ := legacyLayoutFixture(t, "v10")
	legacyLayoutWrite(t, outside, "Local State", state)
	if err := os.MkdirAll(filepath.Join(dir, "electron"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "Local State"), filepath.Join(dir, "electron", "Local State")); err != nil {
		t.Skip("Windows symlink privilege is unavailable")
	}
	decoded, err := New(dir).Decrypt(blob)
	if err == nil || len(decoded) != 0 {
		clear(decoded)
		t.Fatal("legacy key outside selected profile was accepted")
	}
	if !strings.Contains(err.Error(), "ключ") {
		t.Fatal("unexpected confinement diagnostic")
	}
}
