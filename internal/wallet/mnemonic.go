// Package wallet handles HD wallet creation and storage: BIP-39 mnemonic
// generation/import, seed derivation, and the encrypted wallet store that lives
// inside the vault. Chain-specific address derivation and signing live in the
// chains package.
package wallet

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"

	bip39 "github.com/tyler-smith/go-bip39"
)

// MnemonicStrength is the entropy size, in bits, for a generated mnemonic.
type MnemonicStrength int

const (
	// Strength12 produces a 12-word mnemonic (128 bits of entropy).
	Strength12 MnemonicStrength = 128
	// Strength24 produces a 24-word mnemonic (256 bits of entropy).
	Strength24 MnemonicStrength = 256
)

// ErrInvalidMnemonic is returned when an imported phrase fails BIP-39 checks.
var ErrInvalidMnemonic = errors.New("wallet: invalid mnemonic phrase")

// GenerateMnemonic creates a new BIP-39 mnemonic. Entropy is drawn directly from
// crypto/rand here — never math/rand — because a predictable seed means
// predictable, drainable keys.
func GenerateMnemonic(strength MnemonicStrength) (string, error) {
	if strength != Strength12 && strength != Strength24 {
		return "", fmt.Errorf("wallet: unsupported mnemonic strength %d", strength)
	}
	entropy := make([]byte, int(strength)/8)
	if _, err := io.ReadFull(rand.Reader, entropy); err != nil {
		return "", fmt.Errorf("wallet: crypto/rand failure: %w", err)
	}
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return "", err
	}
	return mnemonic, nil
}

// ValidateMnemonic reports whether a phrase is a valid BIP-39 mnemonic
// (wordlist membership + checksum).
func ValidateMnemonic(mnemonic string) bool {
	return bip39.IsMnemonicValid(mnemonic)
}

// SeedFromMnemonic derives the 64-byte BIP-39 seed from a mnemonic and an
// optional passphrase ("25th word"). The mnemonic is validated first.
func SeedFromMnemonic(mnemonic, passphrase string) ([]byte, error) {
	seed, err := bip39.NewSeedWithErrorChecking(mnemonic, passphrase)
	if err != nil {
		return nil, ErrInvalidMnemonic
	}
	return seed, nil
}
