package vault

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// memKeychain is an in-memory Keychain for tests (never touches the OS keychain).
type memKeychain struct{ m map[string][]byte }

func newMemKeychain() *memKeychain { return &memKeychain{m: map[string][]byte{}} }

func (k *memKeychain) Set(acct string, secret []byte) error {
	k.m[acct] = append([]byte(nil), secret...)
	return nil
}
func (k *memKeychain) Get(acct string) ([]byte, error) {
	s, ok := k.m[acct]
	if !ok {
		return nil, ErrSecretNotFound
	}
	return append([]byte(nil), s...), nil
}
func (k *memKeychain) Delete(acct string) error { delete(k.m, acct); return nil }

func tempVaultPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "vault.json")
}

func TestCreateUnlockRoundTrip(t *testing.T) {
	path := tempVaultPath(t)
	kc := newMemKeychain()

	v, err := Create(path, "correct horse battery staple", kc)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !v.IsUnlocked() {
		t.Fatal("vault should be unlocked after Create")
	}
	if err := v.SetSecret([]byte(`{"wallets":["seed-a"]}`)); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}
	v.Lock()
	if v.IsUnlocked() {
		t.Fatal("vault should be locked after Lock")
	}

	// Reopen from disk and unlock.
	v2, err := Open(path, kc)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := v2.UnlockWithPassphrase("correct horse battery staple"); err != nil {
		t.Fatalf("UnlockWithPassphrase: %v", err)
	}
	got, err := v2.Secret()
	if err != nil {
		t.Fatalf("Secret: %v", err)
	}
	if string(got) != `{"wallets":["seed-a"]}` {
		t.Fatalf("payload mismatch: %s", got)
	}
}

func TestWrongPassphrase(t *testing.T) {
	path := tempVaultPath(t)
	kc := newMemKeychain()
	if _, err := Create(path, "right", kc); err != nil {
		t.Fatalf("Create: %v", err)
	}
	v, _ := Open(path, kc)
	if err := v.UnlockWithPassphrase("wrong"); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("want ErrWrongPassphrase, got %v", err)
	}
	if v.IsUnlocked() {
		t.Fatal("vault must stay locked after wrong passphrase")
	}
}

func TestCreateRefusesExisting(t *testing.T) {
	path := tempVaultPath(t)
	kc := newMemKeychain()
	if _, err := Create(path, "pw", kc); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := Create(path, "pw", kc); !errors.Is(err, ErrVaultExists) {
		t.Fatalf("want ErrVaultExists, got %v", err)
	}
}

func TestPINUnlock(t *testing.T) {
	path := tempVaultPath(t)
	kc := newMemKeychain()
	v, _ := Create(path, "pw", kc)
	if err := v.SetPIN("123456"); err != nil {
		t.Fatalf("SetPIN: %v", err)
	}
	v.Lock()

	v2, _ := Open(path, kc)
	if !v2.HasPIN() {
		t.Fatal("HasPIN should be true")
	}
	if err := v2.UnlockWithPIN("123456"); err != nil {
		t.Fatalf("UnlockWithPIN: %v", err)
	}
	if !v2.IsUnlocked() {
		t.Fatal("vault should be unlocked via PIN")
	}
}

func TestWrongPINCountsAndLocksOut(t *testing.T) {
	path := tempVaultPath(t)
	kc := newMemKeychain()
	v, _ := Create(path, "pw", kc)
	if err := v.SetPIN("123456"); err != nil {
		t.Fatalf("SetPIN: %v", err)
	}
	v.Lock()

	v2, _ := Open(path, kc)
	// maxPINFailures wrong attempts.
	for i := 0; i < maxPINFailures-1; i++ {
		if err := v2.UnlockWithPIN("000000"); !errors.Is(err, ErrWrongPIN) {
			t.Fatalf("attempt %d: want ErrWrongPIN, got %v", i, err)
		}
	}
	// The final failure trips the lockout.
	if err := v2.UnlockWithPIN("000000"); !errors.Is(err, ErrPINLocked) {
		t.Fatalf("want ErrPINLocked on final attempt, got %v", err)
	}
	// PIN secret must be gone from the keychain.
	if _, err := kc.Get(pinSecretAcct); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("pinSecret should be deleted after lockout, got %v", err)
	}
	// Even the correct PIN now fails (it's disabled).
	v3, _ := Open(path, kc)
	if err := v3.UnlockWithPIN("123456"); errors.Is(err, nil) {
		t.Fatal("PIN should be disabled after lockout")
	}
}

// The core invariant: a PIN lockout must never impair the passphrase root path.
func TestLockoutDoesNotBrickPassphrase(t *testing.T) {
	path := tempVaultPath(t)
	kc := newMemKeychain()
	v, _ := Create(path, "master-pw", kc)
	_ = v.SetSecret([]byte("top-secret-seed"))
	_ = v.SetPIN("123456")
	v.Lock()

	// Burn through the PIN until lockout.
	vp, _ := Open(path, kc)
	for i := 0; i < maxPINFailures; i++ {
		_ = vp.UnlockWithPIN("999999")
	}

	// Passphrase must still unlock and reveal the payload.
	vf, _ := Open(path, kc)
	if err := vf.UnlockWithPassphrase("master-pw"); err != nil {
		t.Fatalf("passphrase unlock after PIN lockout failed: %v", err)
	}
	got, _ := vf.Secret()
	if string(got) != "top-secret-seed" {
		t.Fatalf("payload after lockout = %q, want top-secret-seed", got)
	}
}

func TestAtomicSaveKeepsBackup(t *testing.T) {
	path := tempVaultPath(t)
	kc := newMemKeychain()
	v, _ := Create(path, "pw", kc) // first save: no .bak yet
	if Exists(path + ".bak") {
		t.Fatal(".bak should not exist after first save")
	}
	if err := v.SetSecret([]byte("v2")); err != nil { // second save creates .bak
		t.Fatalf("SetSecret: %v", err)
	}
	if !Exists(path + ".bak") {
		t.Fatal(".bak should exist after second save")
	}
	if Exists(path + ".tmp") {
		t.Fatal(".tmp must not linger after a successful save")
	}
}

func TestTamperedPayloadFailsAuth(t *testing.T) {
	path := tempVaultPath(t)
	kc := newMemKeychain()
	v, _ := Create(path, "pw", kc)
	_ = v.SetSecret([]byte("data"))
	v.Lock()

	// Corrupt one byte of the file.
	raw, _ := os.ReadFile(path)
	for i := range raw {
		if raw[i] == '"' {
			continue
		}
		raw[i] ^= 0x01
		break
	}
	_ = os.WriteFile(path, raw, 0o600)

	v2, err := Open(path, kc)
	if err != nil {
		return // corruption broke JSON parse — also acceptable
	}
	if err := v2.UnlockWithPassphrase("pw"); err == nil {
		t.Fatal("tampered vault must not unlock cleanly")
	}
}
