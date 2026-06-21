package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

// Cryptographic constants. These define the on-disk format; changing them
// requires a format migration. Argon2id parameters are stored per-wrap in the
// envelope so they can be raised over time without breaking old vaults.
const (
	keyLen    = 32 // AES-256
	nonceLen  = 12 // GCM standard nonce
	saltLen   = 16
	secretLen = 32 // 256-bit keychain pinSecret

	argonTime    = 3
	argonMemory  = 64 * 1024 // 64 MiB
	argonThreads = 4
)

// kdfParams records the Argon2id settings used to derive a key, so a vault can
// always be reopened even if defaults change later.
type kdfParams struct {
	Salt    []byte `json:"salt"`
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"` // KiB
	Threads uint8  `json:"threads"`
	KeyLen  uint32 `json:"keyLen"`
}

func newKDFParams() kdfParams {
	return kdfParams{
		Salt:    randomBytes(saltLen),
		Time:    argonTime,
		Memory:  argonMemory,
		Threads: argonThreads,
		KeyLen:  keyLen,
	}
}

// deriveKey runs Argon2id over the given secret material. For the passphrase
// path `material` is the passphrase bytes; for the PIN path it is
// pinSecret‖PIN (argon2.IDKey has no separate pepper parameter, so the
// high-entropy keychain secret is concatenated into the password position).
func (p kdfParams) deriveKey(material []byte) []byte {
	return argon2.IDKey(material, p.Salt, p.Time, p.Memory, p.Threads, p.KeyLen)
}

// randomBytes returns n cryptographically random bytes or panics. Key material
// must never come from a weak RNG, so failure here is fatal by design.
func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		panic(fmt.Sprintf("vault: crypto/rand failure: %v", err))
	}
	return b
}

// seal encrypts plaintext with AES-256-GCM under key, binding aad as additional
// authenticated data and prepending a fresh random nonce to the ciphertext.
// A new nonce is generated for every call; nonces are never reused.
func seal(key, plaintext, aad []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := randomBytes(nonceLen)
	return gcm.Seal(nonce, nonce, plaintext, aad), nil
}

// open reverses seal. A wrong key, tampered ciphertext, or mismatched aad all
// surface as an authentication error.
func open(key, ciphertext, aad []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < nonceLen {
		return nil, fmt.Errorf("vault: ciphertext too short")
	}
	nonce, body := ciphertext[:nonceLen], ciphertext[nonceLen:]
	return gcm.Open(nil, nonce, body, aad)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != keyLen {
		return nil, fmt.Errorf("vault: key must be %d bytes, got %d", keyLen, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// zero overwrites a byte slice in place. Best-effort: Go's GC may have copied
// the buffer, but holding sensitive keys for no longer than necessary still
// shrinks the window in which they sit in process memory.
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
