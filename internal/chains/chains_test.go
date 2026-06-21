package chains

import (
	bip39 "github.com/tyler-smith/go-bip39"
	"testing"
)

// testMnemonic is the standard all-zero-entropy BIP-39 mnemonic used across the
// ecosystem for derivation test vectors.
const testMnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

func testSeed() []byte { return bip39.NewSeed(testMnemonic, "") }

// Derivation vectors are taken from independent, authoritative sources (BIP-84
// spec; go-ethereum / iancoleman BIP-39 tool) so we verify against what external
// wallets show for the same seed, not against our own implementation.
func TestEVMDerivationVector(t *testing.T) {
	addr, err := NewEthereum(Mainnet, "").DeriveAddress(testSeed(), 0)
	if err != nil {
		t.Fatalf("DeriveAddress: %v", err)
	}
	const want = "0x9858EfFD232B4033E47d90003D41EC34EcaEda94" // m/44'/60'/0'/0/0
	if addr != want {
		t.Fatalf("EVM address = %s, want %s", addr, want)
	}
}

func TestBitcoinDerivationVector(t *testing.T) {
	addr, err := NewBitcoin(Mainnet, "").DeriveAddress(testSeed(), 0)
	if err != nil {
		t.Fatalf("DeriveAddress: %v", err)
	}
	// Directly from the BIP-84 specification (Account 0, first receiving address).
	const want = "bc1qcr8te4kr609gcawutmrza0j4xv80jy8z306fyu" // m/84'/0'/0'/0/0
	if addr != want {
		t.Fatalf("BTC address = %s, want %s", addr, want)
	}
}

func TestSolanaDerivationVector(t *testing.T) {
	addr, err := NewSolana(Mainnet, "").DeriveAddress(testSeed(), 0)
	if err != nil {
		t.Fatalf("DeriveAddress: %v", err)
	}
	// m/44'/501'/0'/0' (Phantom). SLIP-0010 verified against the spec vectors
	// (slip10_test.go); this final address corroborated against the hdwallet lib.
	const want = "HAgk14JpMQLgt6rVgv7cBQFJWFto5Dqxi472uT3DKpqk"
	if addr != want {
		t.Fatalf("SOL address = %s, want %s", addr, want)
	}
}

func TestXRPDerivationVector(t *testing.T) {
	addr, err := NewXRP(Mainnet, "").DeriveAddress(testSeed(), 0)
	if err != nil {
		t.Fatalf("DeriveAddress: %v", err)
	}
	// m/44'/144'/0'/0/0. Correct by construction: secp256k1 derivation is proven
	// by the EVM/BTC vectors and the address encoding by ACCOUNT_ZERO/ONE
	// (xrp_test.go). Pinned here as a regression guard.
	const want = "rHsMGQEkVNJmpGWs8XUBoTBiAAbwxZN5v3"
	if addr != want {
		t.Fatalf("XRP address = %s, want %s", addr, want)
	}
}
