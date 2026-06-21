package wallet

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rhymeswithlimo/fivepoint/internal/vault"
)

const trezorMnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

// Standard BIP-39 (Trezor) test vector for the all-zero entropy mnemonic with
// passphrase "TREZOR".
const trezorSeedHex = "c55257c360c07c72029aebc1b53c05ed0362ada38ead3e3e9efa3708e53495531f09a6987599d18264c1e1c92f2cf141630c7a3c4ab7c81b2f001698e7463b04"

func TestSeedVector(t *testing.T) {
	seed, err := SeedFromMnemonic(trezorMnemonic, "TREZOR")
	if err != nil {
		t.Fatalf("SeedFromMnemonic: %v", err)
	}
	if got := hex.EncodeToString(seed); got != trezorSeedHex {
		t.Fatalf("seed = %s\nwant   %s", got, trezorSeedHex)
	}
}

func TestGenerateMnemonic(t *testing.T) {
	for _, tc := range []struct {
		strength MnemonicStrength
		words    int
	}{{Strength12, 12}, {Strength24, 24}} {
		mn, err := GenerateMnemonic(tc.strength)
		if err != nil {
			t.Fatalf("GenerateMnemonic(%d): %v", tc.strength, err)
		}
		if n := len(strings.Fields(mn)); n != tc.words {
			t.Errorf("strength %d gave %d words, want %d", tc.strength, n, tc.words)
		}
		if !ValidateMnemonic(mn) {
			t.Errorf("generated mnemonic failed validation: %q", mn)
		}
	}
}

func TestMnemonicUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		mn, err := GenerateMnemonic(Strength12)
		if err != nil {
			t.Fatal(err)
		}
		if seen[mn] {
			t.Fatal("GenerateMnemonic produced a duplicate — RNG is broken")
		}
		seen[mn] = true
	}
}

func TestValidateMnemonicRejectsBad(t *testing.T) {
	if ValidateMnemonic("not a real mnemonic phrase at all here ok") {
		t.Error("invalid phrase should not validate")
	}
}

// newTestManager creates an unlocked vault backed manager. No PIN is set, so the
// OS keychain is never touched.
func newTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vault.json")
	v, err := vault.Create(path, "test-pw", nil)
	if err != nil {
		t.Fatalf("vault.Create: %v", err)
	}
	return NewManager(v), path
}

func TestManagerCreateListReveal(t *testing.T) {
	m, _ := newTestManager(t)

	info, mnemonic, err := m.Create("Rainy Day", "alien", "magenta", Strength12)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.Name != "Rainy Day" || info.Kind != KindMnemonic || info.ID == "" {
		t.Fatalf("unexpected info: %+v", info)
	}
	if !ValidateMnemonic(mnemonic) {
		t.Fatalf("returned mnemonic invalid: %q", mnemonic)
	}

	list, err := m.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %v (err %v), want 1 wallet", list, err)
	}

	got, err := m.RevealSeed(info.ID)
	if err != nil || got != mnemonic {
		t.Fatalf("RevealSeed = %q (err %v), want %q", got, err, mnemonic)
	}
}

func TestManagerImportAndDelete(t *testing.T) {
	m, _ := newTestManager(t)

	info, err := m.ImportMnemonic("Imported", "alien", "lime", trezorMnemonic, "")
	if err != nil {
		t.Fatalf("ImportMnemonic: %v", err)
	}
	if _, err := m.ImportMnemonic("Bad", "", "", "totally invalid phrase", ""); err != ErrInvalidMnemonic {
		t.Fatalf("want ErrInvalidMnemonic, got %v", err)
	}

	if err := m.Rename(info.ID, "Renamed"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	list, _ := m.List()
	if list[0].Name != "Renamed" {
		t.Fatalf("rename did not persist: %+v", list[0])
	}

	if err := m.Delete(info.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if list, _ := m.List(); len(list) != 0 {
		t.Fatalf("wallet not deleted: %v", list)
	}
}

// Secrets must never appear in the on-disk vault file in plaintext.
func TestSecretsEncryptedAtRest(t *testing.T) {
	m, path := newTestManager(t)
	_, err := m.ImportMnemonic("Secret", "", "", trezorMnemonic, "")
	if err != nil {
		t.Fatalf("ImportMnemonic: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read vault: %v", err)
	}
	if strings.Contains(string(raw), "abandon") {
		t.Fatal("mnemonic found in plaintext in vault file — payload not encrypted")
	}
}
