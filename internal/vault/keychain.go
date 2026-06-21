package vault

import (
	"encoding/base64"
	"errors"

	"github.com/zalando/go-keyring"
)

// keychainService is the namespace under which Fivepoint stores OS-keychain
// secrets (Windows Credential Manager / macOS Keychain / Linux Secret Service).
const keychainService = "Fivepoint"

// ErrSecretNotFound indicates the requested keychain entry does not exist
// (e.g. the PIN was never set, or lockout deleted the pinSecret).
var ErrSecretNotFound = errors.New("vault: keychain secret not found")

// Keychain abstracts OS secure storage so the vault can be tested without
// touching the real system keychain.
type Keychain interface {
	Set(account string, secret []byte) error
	Get(account string) ([]byte, error)
	Delete(account string) error
}

// osKeychain is the production Keychain backed by the operating system.
type osKeychain struct{}

// OSKeychain returns the OS-backed keychain implementation. On Windows entries
// are protected by DPAPI (readable only by the logged-in user); this guards
// against the vault file being stolen to another machine, but not against
// malware running as the same user — which is why it only ever holds the PIN
// secret, never the passphrase-derived root.
func OSKeychain() Keychain { return osKeychain{} }

func (osKeychain) Set(account string, secret []byte) error {
	return keyring.Set(keychainService, account, base64.StdEncoding.EncodeToString(secret))
}

func (osKeychain) Get(account string) ([]byte, error) {
	s, err := keyring.Get(keychainService, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, ErrSecretNotFound
	}
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(s)
}

func (osKeychain) Delete(account string) error {
	err := keyring.Delete(keychainService, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil // already gone
	}
	return err
}
