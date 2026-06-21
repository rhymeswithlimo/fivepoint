// Package vault encrypts Fivepoint's secrets (mnemonics and private keys) at
// rest. A random Data Encryption Key (DEK) encrypts the secret payload with
// AES-256-GCM; the DEK is then wrapped two ways:
//
//   - Passphrase (root of trust): KEK = Argon2id(passphrase). Always required to
//     fully control the wallet.
//   - 6-digit PIN (convenience only): KEK = Argon2id(pinSecret‖PIN), where the
//     256-bit pinSecret lives only in the OS keychain. Without the keychain
//     secret the short PIN cannot be brute-forced from the vault file, and after
//     too many failures the pinSecret is deleted (lockout), leaving the
//     passphrase path untouched.
//
// Secrets exist in plaintext only in memory while unlocked, are zeroized on
// lock, and the vault file is written atomically with a .bak so a crash mid-save
// can never corrupt the only copy of a user's keys.
package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	formatVersion   = 1
	pinSecretAcct   = "pin-secret"
	maxPINFailures  = 5
	defaultInitData = "{}"
)

// Additional-authenticated-data labels bind each ciphertext to its role so a
// wrap blob can never be substituted for another field.
var (
	aadPayload  = []byte("fivepoint/payload/v1")
	aadPassWrap = []byte("fivepoint/passwrap/v1")
	aadPINWrap  = []byte("fivepoint/pinwrap/v1")
)

// Errors returned by the vault.
var (
	ErrVaultExists      = errors.New("vault: a vault already exists at this path")
	ErrVaultNotFound    = errors.New("vault: no vault at this path")
	ErrLocked           = errors.New("vault: locked")
	ErrWrongPassphrase  = errors.New("vault: wrong passphrase")
	ErrWrongPIN         = errors.New("vault: wrong PIN")
	ErrPINNotSet        = errors.New("vault: PIN unlock is not configured")
	ErrPINLocked        = errors.New("vault: PIN disabled after too many attempts; use passphrase")
)

type wrap struct {
	KDF  kdfParams `json:"kdf"`
	Blob []byte    `json:"blob"` // AES-256-GCM(DEK), nonce-prefixed
}

type envelope struct {
	Version        int    `json:"version"`
	PayloadBlob    []byte `json:"payload"` // AES-256-GCM(secret) under DEK, nonce-prefixed
	PassphraseWrap wrap   `json:"passphraseWrap"`
	PinWrap        *wrap  `json:"pinWrap,omitempty"`
	PINFailures    int    `json:"pinFailures"`
}

// Vault is a handle to an on-disk encrypted vault. It is safe for concurrent use.
type Vault struct {
	path string
	kc   Keychain

	mu        sync.Mutex
	env       envelope
	dek       []byte // nil when locked
	secret    []byte // decrypted payload; nil when locked
	lockAfter time.Duration
	timer     *time.Timer
	onLock    func()
}

// Exists reports whether a vault file is present at path.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Create initializes a new vault protected by passphrase and returns it in the
// unlocked state. It fails if a vault already exists at path.
func Create(path, passphrase string, kc Keychain) (*Vault, error) {
	if kc == nil {
		kc = OSKeychain()
	}
	if Exists(path) {
		return nil, ErrVaultExists
	}
	if passphrase == "" {
		return nil, errors.New("vault: passphrase must not be empty")
	}

	dek := randomBytes(keyLen)
	secret := []byte(defaultInitData)

	payloadBlob, err := seal(dek, secret, aadPayload)
	if err != nil {
		return nil, err
	}
	pw, err := makePassphraseWrap(passphrase, dek)
	if err != nil {
		return nil, err
	}

	v := &Vault{
		path:   path,
		kc:     kc,
		dek:    dek,
		secret: secret,
		env: envelope{
			Version:        formatVersion,
			PayloadBlob:    payloadBlob,
			PassphraseWrap: pw,
		},
	}
	if err := v.save(); err != nil {
		return nil, err
	}
	v.touchLocked()
	return v, nil
}

// Open loads an existing (locked) vault from disk.
func Open(path string, kc Keychain) (*Vault, error) {
	if kc == nil {
		kc = OSKeychain()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrVaultNotFound
		}
		return nil, err
	}
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("vault: corrupt vault file: %w", err)
	}
	return &Vault{path: path, kc: kc, env: env}, nil
}

func makePassphraseWrap(passphrase string, dek []byte) (wrap, error) {
	kdf := newKDFParams()
	kek := kdf.deriveKey([]byte(passphrase))
	defer zero(kek)
	blob, err := seal(kek, dek, aadPassWrap)
	if err != nil {
		return wrap{}, err
	}
	return wrap{KDF: kdf, Blob: blob}, nil
}

// UnlockWithPassphrase decrypts the vault using the master passphrase.
func (v *Vault) UnlockWithPassphrase(passphrase string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	kek := v.env.PassphraseWrap.KDF.deriveKey([]byte(passphrase))
	defer zero(kek)

	dek, err := open(kek, v.env.PassphraseWrap.Blob, aadPassWrap)
	if err != nil {
		return ErrWrongPassphrase
	}
	if err := v.finishUnlock(dek); err != nil {
		return err
	}
	// A successful passphrase unlock clears any accumulated PIN failures.
	if v.env.PINFailures != 0 {
		v.env.PINFailures = 0
		_ = v.save()
	}
	return nil
}

// UnlockWithPIN decrypts the vault using the convenience PIN. It requires the
// keychain pinSecret; repeated failures disable the PIN entirely.
func (v *Vault) UnlockWithPIN(pin string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.env.PinWrap == nil {
		return ErrPINNotSet
	}
	if v.env.PINFailures >= maxPINFailures {
		v.disablePIN()
		return ErrPINLocked
	}

	pinSecret, err := v.kc.Get(pinSecretAcct)
	if errors.Is(err, ErrSecretNotFound) {
		// The keychain secret is gone (e.g. prior lockout); PIN cannot work.
		v.env.PinWrap = nil
		_ = v.save()
		return ErrPINNotSet
	}
	if err != nil {
		return err
	}
	defer zero(pinSecret)

	kek := v.env.PinWrap.KDF.deriveKey(append(append([]byte{}, pinSecret...), []byte(pin)...))
	defer zero(kek)

	dek, err := open(kek, v.env.PinWrap.Blob, aadPINWrap)
	if err != nil {
		v.env.PINFailures++
		if v.env.PINFailures >= maxPINFailures {
			v.disablePIN()
			_ = v.save()
			return ErrPINLocked
		}
		_ = v.save()
		return ErrWrongPIN
	}

	if err := v.finishUnlock(dek); err != nil {
		return err
	}
	if v.env.PINFailures != 0 {
		v.env.PINFailures = 0
		_ = v.save()
	}
	return nil
}

// finishUnlock decrypts the payload with the recovered DEK and records the
// unlocked state. Caller holds v.mu.
func (v *Vault) finishUnlock(dek []byte) error {
	secret, err := open(dek, v.env.PayloadBlob, aadPayload)
	if err != nil {
		zero(dek)
		return fmt.Errorf("vault: payload decryption failed: %w", err)
	}
	v.dek = dek
	v.secret = secret
	v.resetTimerLocked()
	return nil
}

// disablePIN removes the PIN unlock path. Caller holds v.mu. The passphrase
// path is never affected.
func (v *Vault) disablePIN() {
	_ = v.kc.Delete(pinSecretAcct)
	v.env.PinWrap = nil
	v.env.PINFailures = 0
}

// SetPIN configures (or replaces) the convenience PIN. The vault must be
// unlocked so the DEK is available to re-wrap.
func (v *Vault) SetPIN(pin string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.dek == nil {
		return ErrLocked
	}
	if len(pin) < 4 {
		return errors.New("vault: PIN too short")
	}

	pinSecret := randomBytes(secretLen)
	defer zero(pinSecret)
	if err := v.kc.Set(pinSecretAcct, pinSecret); err != nil {
		return fmt.Errorf("vault: storing PIN secret: %w", err)
	}

	kdf := newKDFParams()
	kek := kdf.deriveKey(append(append([]byte{}, pinSecret...), []byte(pin)...))
	defer zero(kek)
	blob, err := seal(kek, v.dek, aadPINWrap)
	if err != nil {
		return err
	}
	v.env.PinWrap = &wrap{KDF: kdf, Blob: blob}
	v.env.PINFailures = 0
	return v.save()
}

// RemovePIN disables PIN unlock and clears the keychain secret.
func (v *Vault) RemovePIN() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.disablePIN()
	return v.save()
}

// HasPIN reports whether a PIN unlock path is configured.
func (v *Vault) HasPIN() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.env.PinWrap != nil
}

// Secret returns a copy of the decrypted payload. The vault must be unlocked.
func (v *Vault) Secret() ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.secret == nil {
		return nil, ErrLocked
	}
	v.resetTimerLocked()
	return append([]byte(nil), v.secret...), nil
}

// SetSecret replaces the payload and persists it under the existing DEK.
func (v *Vault) SetSecret(b []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.dek == nil {
		return ErrLocked
	}
	blob, err := seal(v.dek, b, aadPayload)
	if err != nil {
		return err
	}
	v.env.PayloadBlob = blob
	v.secret = append([]byte(nil), b...)
	v.resetTimerLocked()
	return v.save()
}

// VerifyPassphrase reports whether passphrase is correct, without changing the
// lock state. Used to re-authenticate before exposing a secret (seed backup).
func (v *Vault) VerifyPassphrase(passphrase string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	kek := v.env.PassphraseWrap.KDF.deriveKey([]byte(passphrase))
	defer zero(kek)
	dek, err := open(kek, v.env.PassphraseWrap.Blob, aadPassWrap)
	if err != nil {
		return false
	}
	zero(dek)
	return true
}

// IsUnlocked reports whether secrets are currently available in memory.
func (v *Vault) IsUnlocked() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.dek != nil
}

// Lock zeroizes in-memory secrets and stops the auto-lock timer.
func (v *Vault) Lock() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.lockLocked()
}

func (v *Vault) lockLocked() {
	if v.dek != nil {
		zero(v.dek)
		v.dek = nil
	}
	if v.secret != nil {
		zero(v.secret)
		v.secret = nil
	}
	if v.timer != nil {
		v.timer.Stop()
		v.timer = nil
	}
}

// SetAutoLock arranges for the vault to lock after d of inactivity, calling
// onLock (if non-nil) when it does. d <= 0 disables auto-lock.
func (v *Vault) SetAutoLock(d time.Duration, onLock func()) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.lockAfter = d
	v.onLock = onLock
	v.resetTimerLocked()
}

// Touch resets the auto-lock countdown to signal user activity.
func (v *Vault) Touch() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.resetTimerLocked()
}

// touchLocked acquires the mutex then resets the timer (used post-construction).
func (v *Vault) touchLocked() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.resetTimerLocked()
}

// resetTimerLocked restarts the idle auto-lock timer. Caller holds v.mu.
func (v *Vault) resetTimerLocked() {
	if v.timer != nil {
		v.timer.Stop()
		v.timer = nil
	}
	if v.lockAfter <= 0 || v.dek == nil {
		return
	}
	onLock := v.onLock
	v.timer = time.AfterFunc(v.lockAfter, func() {
		v.mu.Lock()
		v.lockLocked()
		v.mu.Unlock()
		if onLock != nil {
			onLock()
		}
	})
}

// save writes the envelope to disk atomically, preserving a .bak of the prior
// version. Caller holds v.mu. A crash at any point leaves either the previous
// vault (at path) or its backup (.bak) intact — never a truncated file.
func (v *Vault) save() error {
	data, err := json.MarshalIndent(&v.env, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(v.path), 0o700); err != nil {
		return err
	}

	tmp := v.path + ".tmp"
	if err := writeFileSync(tmp, data); err != nil {
		return err
	}
	if Exists(v.path) {
		if err := copyFileSync(v.path, v.path+".bak"); err != nil {
			return err
		}
	}
	// Atomic replace (Go uses MoveFileEx with REPLACE_EXISTING on Windows).
	return os.Rename(tmp, v.path)
}

func writeFileSync(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func copyFileSync(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return writeFileSync(dst, data)
}
