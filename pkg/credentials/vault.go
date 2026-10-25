// Package credentials stores secrets in a private encrypted file on macOS.
// It shares the Agent file-vault format; product/environment isolation is carried
// by service and account. Other platforms retain their native credential store.
// The key file is protected by the OS user account, not against same-user malware.
package credentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	gokeyring "github.com/zalando/go-keyring"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Vault struct {
	Dir       string
	LegacyGet func(string, string) (string, error)
}
type vaultFile struct {
	Entries map[string]*string `json:"entries"`
}

func DefaultDir() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(root) {
		return "", errors.New("credential root must be absolute")
	}
	return filepath.Join(root, "eos", "credentials"), nil
}
func defaultVault() (*Vault, error) {
	dir, err := DefaultDir()
	return &Vault{Dir: dir, LegacyGet: legacyGet}, err
}
func Get(service, account string) (string, error) {
	if runtime.GOOS != "darwin" {
		return gokeyring.Get(service, account)
	}
	v, err := defaultVault()
	if err != nil {
		return "", err
	}
	return v.Get(service, account)
}
func Set(service, account, value string) error {
	if runtime.GOOS != "darwin" {
		return gokeyring.Set(service, account, value)
	}
	v, err := defaultVault()
	if err != nil {
		return err
	}
	return v.Set(service, account, value)
}
func Delete(service, account string) error {
	if runtime.GOOS != "darwin" {
		return gokeyring.Delete(service, account)
	}
	v, err := defaultVault()
	if err != nil {
		return err
	}
	return v.Delete(service, account)
}
func entryKey(service, account string) string { return service + "\x1f" + account }
func readPrivate(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("credential path is not a regular file")
	}
	return os.ReadFile(path)
}
func (v *Vault) load() (vaultFile, error) {
	file := vaultFile{Entries: map[string]*string{}}
	raw, err := readPrivate(filepath.Join(v.Dir, "credentials.enc"))
	if errors.Is(err, os.ErrNotExist) {
		return file, nil
	}
	if err != nil {
		return file, err
	}
	if json.Unmarshal(raw, &file) != nil || file.Entries == nil {
		return file, errors.New("credential vault corrupt")
	}
	return file, nil
}
func (v *Vault) save(file vaultFile) error {
	raw, err := json.Marshal(file)
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(v.Dir, "credentials.enc"), raw)
}
func atomicWrite(path string, raw []byte) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), ".credential-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer func() { _ = f.Close(); _ = os.Remove(name) }()
	if _, err = f.Write(raw); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}
func (v *Vault) cipher(file vaultFile) (cipher.AEAD, error) {
	path := filepath.Join(v.Dir, "machine-id")
	raw, err := readPrivate(path)
	if errors.Is(err, os.ErrNotExist) {
		for _, value := range file.Entries {
			if value != nil {
				return nil, errors.New("credential key file missing")
			}
		}
		id := make([]byte, 16)
		if _, err = rand.Read(id); err != nil {
			return nil, err
		}
		raw = []byte(hex.EncodeToString(id))
		if err = atomicWrite(path, raw); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	id, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(id) != 16 {
		return nil, errors.New("machine-id corrupt")
	}
	key := sha256.Sum256(append(id, []byte("eos-filevault-v1")...))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func (v *Vault) encrypt(file vaultFile, service, account, value string) (string, error) {
	aead, err := v.cipher(file)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	blob := aead.Seal(nonce, nonce, []byte(value), []byte(entryKey(service, account)))
	return "enc:v2:" + hex.EncodeToString(blob), nil
}
func (v *Vault) decrypt(file vaultFile, service, account, entry string) (string, error) {
	var aad []byte
	var encoded string
	switch {
	case strings.HasPrefix(entry, "enc:v2:"):
		encoded = strings.TrimPrefix(entry, "enc:v2:")
		aad = []byte(entryKey(service, account))
	case strings.HasPrefix(entry, "enc:v1:"):
		encoded = strings.TrimPrefix(entry, "enc:v1:")
		aad = []byte(service)
	default:
		return "", errors.New("unknown credential format")
	}
	blob, err := hex.DecodeString(encoded)
	if err != nil || len(blob) < 28 {
		return "", errors.New("credential ciphertext corrupt")
	}
	aead, err := v.cipher(file)
	if err != nil {
		return "", err
	}
	plain, err := aead.Open(nil, blob[:12], blob[12:], aad)
	if err != nil {
		return "", errors.New("credential decryption failed")
	}
	return string(plain), nil
}
func (v *Vault) Get(service, account string) (string, error) {
	unlock, err := lockDirectory(v.Dir)
	if err != nil {
		return "", err
	}
	defer unlock()
	file, err := v.load()
	if err != nil {
		return "", err
	}
	key := entryKey(service, account)
	if value, known := file.Entries[key]; known {
		if value == nil {
			return "", nil
		}
		return v.decrypt(file, service, account, *value)
	}
	if v.LegacyGet == nil {
		return "", nil
	}
	value, err := v.LegacyGet(service, account)
	if err != nil && !errors.Is(err, gokeyring.ErrNotFound) {
		return "", fmt.Errorf("credential migration requires sign-in: %w", err)
	}
	var entry *string
	if err == nil {
		encrypted, e := v.encrypt(file, service, account, value)
		if e != nil {
			return "", e
		}
		entry = &encrypted
	}
	file.Entries[key] = entry
	if err = v.save(file); err != nil {
		return "", err
	}
	return value, nil
}
func (v *Vault) Set(service, account, value string) error {
	unlock, err := lockDirectory(v.Dir)
	if err != nil {
		return err
	}
	defer unlock()
	file, err := v.load()
	if err != nil {
		return err
	}
	encrypted, err := v.encrypt(file, service, account, value)
	if err != nil {
		return err
	}
	file.Entries[entryKey(service, account)] = &encrypted
	return v.save(file)
}
func (v *Vault) Delete(service, account string) error {
	unlock, err := lockDirectory(v.Dir)
	if err != nil {
		return err
	}
	defer unlock()
	file, err := v.load()
	if err != nil {
		return err
	}
	file.Entries[entryKey(service, account)] = nil
	return v.save(file)
}
