package wallet

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rhymeswithlimo/fivepoint/internal/vault"
)

// SecretKind distinguishes how a wallet's keys are derived.
type SecretKind string

const (
	// KindMnemonic is an HD wallet backed by a BIP-39 mnemonic.
	KindMnemonic SecretKind = "mnemonic"
	// KindPrivateKey is a single imported private key.
	KindPrivateKey SecretKind = "privatekey"
)

var (
	// ErrNotFound is returned when no wallet matches an ID.
	ErrNotFound = errors.New("wallet: not found")
	// ErrEmptyName is returned when a wallet name is blank.
	ErrEmptyName = errors.New("wallet: name must not be empty")
	// ErrNoSeed is returned when a wallet has no HD seed (imported private key).
	ErrNoSeed = errors.New("wallet: wallet has no HD seed (imported private key)")
)

// wallet is the full record, including secret material. It is only ever
// serialized into the encrypted vault payload — never exposed to the frontend.
type wallet struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Icon       string     `json:"icon"`
	Color      string     `json:"color"`
	CreatedAt  time.Time  `json:"createdAt"`
	Kind       SecretKind `json:"kind"`
	Mnemonic   string     `json:"mnemonic,omitempty"`
	Passphrase string     `json:"passphrase,omitempty"` // optional BIP-39 passphrase
	PrivateKey string     `json:"privateKey,omitempty"`
}

// store is the decrypted vault payload schema.
type store struct {
	Wallets []wallet `json:"wallets"`
}

// Info is the non-secret view of a wallet, safe to hand to the UI.
type Info struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Icon      string     `json:"icon"`
	Color     string     `json:"color"`
	CreatedAt time.Time  `json:"createdAt"`
	Kind      SecretKind `json:"kind"`
}

func (w wallet) info() Info {
	return Info{ID: w.ID, Name: w.Name, Icon: w.Icon, Color: w.Color, CreatedAt: w.CreatedAt, Kind: w.Kind}
}

// Manager performs wallet CRUD against an unlocked vault. Every read decrypts
// the payload and every write re-encrypts and atomically persists it.
type Manager struct {
	v *vault.Vault
}

// NewManager wraps an (unlocked) vault.
func NewManager(v *vault.Vault) *Manager { return &Manager{v: v} }

func (m *Manager) load() (*store, error) {
	b, err := m.v.Secret()
	if err != nil {
		return nil, err
	}
	var s store
	if len(b) == 0 {
		return &s, nil
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("wallet: decoding store: %w", err)
	}
	return &s, nil
}

func (m *Manager) persist(s *store) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return m.v.SetSecret(b)
}

// Create generates a brand-new HD wallet and returns its metadata together with
// the mnemonic, so the UI can show the one-time backup screen. The mnemonic is
// stored encrypted in the vault.
func (m *Manager) Create(name, icon, color string, strength MnemonicStrength) (Info, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Info{}, "", ErrEmptyName
	}
	mnemonic, err := GenerateMnemonic(strength)
	if err != nil {
		return Info{}, "", err
	}
	w := wallet{
		ID:        newID(),
		Name:      name,
		Icon:      icon,
		Color:     color,
		CreatedAt: time.Now().UTC(),
		Kind:      KindMnemonic,
		Mnemonic:  mnemonic,
	}
	if err := m.add(w); err != nil {
		return Info{}, "", err
	}
	return w.info(), mnemonic, nil
}

// ImportMnemonic adds an existing BIP-39 wallet.
func (m *Manager) ImportMnemonic(name, icon, color, mnemonic, passphrase string) (Info, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Info{}, ErrEmptyName
	}
	mnemonic = strings.TrimSpace(mnemonic)
	if !ValidateMnemonic(mnemonic) {
		return Info{}, ErrInvalidMnemonic
	}
	w := wallet{
		ID:         newID(),
		Name:       name,
		Icon:       icon,
		Color:      color,
		CreatedAt:  time.Now().UTC(),
		Kind:       KindMnemonic,
		Mnemonic:   mnemonic,
		Passphrase: passphrase,
	}
	if err := m.add(w); err != nil {
		return Info{}, err
	}
	return w.info(), nil
}

// ImportPrivateKey adds a single-key wallet. The key string is chain-specific
// (hex for EVM, WIF for BTC, base58 for Solana) and is validated when the
// relevant chain derives from it.
func (m *Manager) ImportPrivateKey(name, icon, color, key string) (Info, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Info{}, ErrEmptyName
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return Info{}, errors.New("wallet: private key must not be empty")
	}
	w := wallet{
		ID:         newID(),
		Name:       name,
		Icon:       icon,
		Color:      color,
		CreatedAt:  time.Now().UTC(),
		Kind:       KindPrivateKey,
		PrivateKey: key,
	}
	if err := m.add(w); err != nil {
		return Info{}, err
	}
	return w.info(), nil
}

func (m *Manager) add(w wallet) error {
	s, err := m.load()
	if err != nil {
		return err
	}
	s.Wallets = append(s.Wallets, w)
	return m.persist(s)
}

// List returns metadata for all wallets (no secrets).
func (m *Manager) List() ([]Info, error) {
	s, err := m.load()
	if err != nil {
		return nil, err
	}
	out := make([]Info, len(s.Wallets))
	for i, w := range s.Wallets {
		out[i] = w.info()
	}
	return out, nil
}

// RevealSeed returns the mnemonic (or private key) for a wallet so the user can
// back it up. This is the one path that exposes a secret; callers must gate it
// behind a fresh authentication prompt.
func (m *Manager) RevealSeed(id string) (string, error) {
	s, err := m.load()
	if err != nil {
		return "", err
	}
	for _, w := range s.Wallets {
		if w.ID == id {
			if w.Kind == KindPrivateKey {
				return w.PrivateKey, nil
			}
			return w.Mnemonic, nil
		}
	}
	return "", ErrNotFound
}

// Seed returns the BIP-39 seed for an HD (mnemonic) wallet, for address
// derivation. Imported single-key wallets have no seed (ErrNoSeed). The returned
// bytes are sensitive; callers should zeroize them after use.
func (m *Manager) Seed(id string) ([]byte, error) {
	s, err := m.load()
	if err != nil {
		return nil, err
	}
	for _, w := range s.Wallets {
		if w.ID == id {
			if w.Kind != KindMnemonic {
				return nil, ErrNoSeed
			}
			return SeedFromMnemonic(w.Mnemonic, w.Passphrase)
		}
	}
	return nil, ErrNotFound
}

// Rename changes a wallet's display name.
func (m *Manager) Rename(id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrEmptyName
	}
	s, err := m.load()
	if err != nil {
		return err
	}
	for i := range s.Wallets {
		if s.Wallets[i].ID == id {
			s.Wallets[i].Name = name
			return m.persist(s)
		}
	}
	return ErrNotFound
}

// Delete removes a wallet and its secret material from the vault.
func (m *Manager) Delete(id string) error {
	s, err := m.load()
	if err != nil {
		return err
	}
	for i := range s.Wallets {
		if s.Wallets[i].ID == id {
			s.Wallets = append(s.Wallets[:i], s.Wallets[i+1:]...)
			return m.persist(s)
		}
	}
	return ErrNotFound
}

// newID returns a random 128-bit hex identifier.
func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("wallet: crypto/rand failure: %v", err))
	}
	return hex.EncodeToString(b)
}
