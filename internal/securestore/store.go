// Package securestore stores credentials for the current Windows logon. Legacy
// v10 records use the profile's DPAPI-protected Chromium AES key; new records use
// a self-contained DPAPI blob and do not require a browser runtime.
package securestore

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const Prefix = "EDCP-DPAPI1:"

type Store struct{ DataDir string }

func New(dataDir string) *Store  { return &Store{DataDir: dataDir} }
func (s *Store) Available() bool { return available() }
func (s *Store) Encrypt(plain []byte) ([]byte, error) {
	if len(plain) > 16*1024*1024 {
		return nil, errors.New("Слишком большой защищённый файл.")
	}
	body, err := protect(plain)
	if err != nil {
		return nil, err
	}
	return append([]byte(Prefix), body...), nil
}
func (s *Store) Decrypt(blob []byte) ([]byte, error) {
	if len(blob) > 16*1024*1024 {
		return nil, errors.New("Слишком большой защищённый файл.")
	}
	if bytes.HasPrefix(blob, []byte(Prefix)) {
		return unprotect(blob[len(Prefix):])
	}
	if bytes.HasPrefix(blob, []byte("v10")) || bytes.HasPrefix(blob, []byte("v11")) {
		if len(blob) < 3+12+16 {
			return nil, errors.New("Повреждён защищённый файл.")
		}
		raw, err := os.ReadFile(filepath.Join(s.DataDir, "Local State"))
		if err != nil {
			return nil, errors.New("Не найден ключ прежнего профиля Windows.")
		}
		if len(raw) > 4*1024*1024 {
			return nil, errors.New("Некорректный профиль Windows.")
		}
		var state struct {
			Crypt struct {
				Key string `json:"encrypted_key"`
			} `json:"os_crypt"`
		}
		if json.Unmarshal(raw, &state) != nil {
			return nil, errors.New("Некорректный профиль Windows.")
		}
		protected, err := base64.StdEncoding.DecodeString(state.Crypt.Key)
		if err != nil || !bytes.HasPrefix(protected, []byte("DPAPI")) {
			return nil, errors.New("Неподдерживаемый ключ прежнего профиля Windows.")
		}
		key, err := unprotect(protected[5:])
		if err != nil {
			return nil, err
		}
		defer clear(key)
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, errors.New("Некорректный ключ прежнего профиля Windows.")
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		plain, err := gcm.Open(nil, blob[3:15], blob[15:], nil)
		if err != nil {
			return nil, errors.New("Не удалось открыть данные прежнего профиля Windows.")
		}
		return plain, nil
	}
	return unprotect(blob)
}
func AtomicWrite(file string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(file), ".copilot-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, file)
}
