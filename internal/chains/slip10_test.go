package chains

import (
	"encoding/hex"
	"testing"
)

// Official SLIP-0010 ed25519 "Test vector 1" (seed 000102030405060708090a0b0c0d0e0f).
// Source: https://github.com/satoshilabs/slips/blob/master/slip-0010.md
func TestSLIP10Ed25519Vector(t *testing.T) {
	seed, _ := hex.DecodeString("000102030405060708090a0b0c0d0e0f")

	cases := []struct {
		path  []uint32
		chain string
		priv  string
		pub   string // 00-prefixed in the spec
	}{
		{
			[]uint32{},
			"90046a93de5380a72b5e45010748567d5ea02bbf6522f979e05c0d8d8ca9fffb",
			"2b4be7f19ee27bbf30c667b642d5f4aa69fd169872f8fc3059c08ebae2eb19e7",
			"00a4b2856bfec510abab89753fac1ac0e1112364e7d250545963f135f2a33188ed",
		},
		{
			[]uint32{hard + 0},
			"8b59aa11380b624e81507a27fedda59fea6d0b779a778918a2fd3590e16e9c69",
			"68e0fe46dfb67e368c75379acec591dad19df3cde26e63b93a8e704f1dade7a3",
			"008c8a13df77a28f3445213a0f432fde644acaa215fc72dcdf300d5efaa85d350c",
		},
		{
			[]uint32{hard + 0, hard + 1},
			"a320425f77d1b5c2505a6b1b27382b37368ee640e3557c315416801243552f14",
			"b1d0bad404bf35da785a64ca1ac54b2617211d2777696fbffaf208f746ae84f2",
			"001932a5270f335bed617d5b935c80aedb1a35bd9fc1e31acafd5372c30f5c1187",
		},
		{
			[]uint32{hard + 0, hard + 1, hard + 2, hard + 2, hard + 1000000000},
			"68789923a0cac2cd5a29172a475fe9e0fb14cd6adb5ad98a3fa70333e7afa230",
			"8f94d394a8e8fd6b1bc2f3f49f5c47e385281d5c17e65324b0f62483e37e8793",
			"003c24da049451555d51a7014a37337aa4e12d41e485abccfa46b47dfb2af54b7a",
		},
	}

	for _, c := range cases {
		k := slip10Master(seed)
		for _, idx := range c.path {
			var err error
			if k, err = k.child(idx); err != nil {
				t.Fatalf("child: %v", err)
			}
		}
		if got := hex.EncodeToString(k.chain); got != c.chain {
			t.Errorf("path %v chain = %s, want %s", c.path, got, c.chain)
		}
		if got := hex.EncodeToString(k.key); got != c.priv {
			t.Errorf("path %v priv = %s, want %s", c.path, got, c.priv)
		}
		_, pub, err := deriveEd25519(seed, c.path)
		if err != nil {
			t.Fatalf("deriveEd25519: %v", err)
		}
		if got := "00" + hex.EncodeToString(pub); got != c.pub {
			t.Errorf("path %v pub = %s, want %s", c.path, got, c.pub)
		}
	}
}
