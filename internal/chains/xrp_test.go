package chains

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil/base58"
)

// xrpEncode replicates the address-encoding step in isolation so it can be
// checked against XRPL's documented special addresses.
func xrpEncode(accountID []byte) string {
	return remapAlphabet(base58.CheckEncode(accountID, 0x00), btcAlphabet, rippleAlphabet)
}

// ACCOUNT_ZERO / ACCOUNT_ONE are well-known XRPL constants: the base58 addresses
// for the all-zero and all-but-one-zero account IDs. They validate the alphabet
// and base58check pipeline independently of key derivation.
func TestXRPAddressEncoding(t *testing.T) {
	zero := make([]byte, 20)
	if got := xrpEncode(zero); got != "rrrrrrrrrrrrrrrrrrrrrhoLvTp" {
		t.Fatalf("ACCOUNT_ZERO = %s, want rrrrrrrrrrrrrrrrrrrrrhoLvTp", got)
	}
	one := make([]byte, 20)
	one[19] = 1
	if got := xrpEncode(one); got != "rrrrrrrrrrrrrrrrrrrrBZbvji" {
		t.Fatalf("ACCOUNT_ONE = %s, want rrrrrrrrrrrrrrrrrrrrBZbvji", got)
	}
}

func TestRippleAlphabetIsPermutation(t *testing.T) {
	if len(rippleAlphabet) != 58 {
		t.Fatalf("ripple alphabet length = %d, want 58", len(rippleAlphabet))
	}
	seen := map[rune]bool{}
	for _, c := range rippleAlphabet {
		seen[c] = true
	}
	for _, c := range btcAlphabet {
		if !seen[c] {
			t.Fatalf("ripple alphabet missing %q from the base58 set", c)
		}
	}
}
