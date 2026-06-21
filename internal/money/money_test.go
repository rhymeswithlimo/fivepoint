package money

import (
	"errors"
	"math/big"
	"testing"
)

func TestParseDecimalRoundTrip(t *testing.T) {
	cases := []struct {
		in       string
		decimals int
		rawWant  string
		strWant  string
	}{
		{"0.12", 8, "12000000", "0.12"},
		{"1", 8, "100000000", "1"},
		{"0", 8, "0", "0"},
		{"0.00000001", 8, "1", "0.00000001"},      // 1 satoshi
		{"21000000", 8, "2100000000000000", "21000000"},
		{"1.5", 18, "1500000000000000000", "1.5"}, // ETH
		{"0.000000000000000001", 18, "1", "0.000000000000000001"}, // 1 wei
		{"1,234.5", 6, "1234500000", "1234.5"},    // grouping separators tolerated
		{"-3", 8, "-300000000", "-3"},
		{".5", 8, "50000000", "0.5"},
		{"123.", 8, "12300000000", "123"},
	}
	for _, c := range cases {
		a, err := ParseDecimal(c.in, c.decimals)
		if err != nil {
			t.Fatalf("ParseDecimal(%q,%d): unexpected error %v", c.in, c.decimals, err)
		}
		if got := a.Raw().String(); got != c.rawWant {
			t.Errorf("ParseDecimal(%q,%d) raw = %s, want %s", c.in, c.decimals, got, c.rawWant)
		}
		if got := a.String(); got != c.strWant {
			t.Errorf("ParseDecimal(%q,%d) String = %s, want %s", c.in, c.decimals, got, c.strWant)
		}
	}
}

func TestParseDecimalRejectsTooPrecise(t *testing.T) {
	if _, err := ParseDecimal("0.123456789", 8); !errors.Is(err, ErrTooPrecise) {
		t.Fatalf("want ErrTooPrecise for 9 dp on 8-dp asset, got %v", err)
	}
	// Exactly at the limit is fine.
	if _, err := ParseDecimal("0.12345678", 8); err != nil {
		t.Fatalf("8 dp on 8-dp asset should parse, got %v", err)
	}
}

func TestParseDecimalRejectsGarbage(t *testing.T) {
	bad := []string{"", "abc", "1.2.3", "1e9", "0x10", "--1", "1.2x"}
	for _, s := range bad {
		if _, err := ParseDecimal(s, 8); err == nil {
			t.Errorf("ParseDecimal(%q) should have failed", s)
		}
	}
}

func TestArithmetic(t *testing.T) {
	a, _ := ParseDecimal("0.12", 8)
	b, _ := ParseDecimal("0.05", 8)

	sum, err := a.Add(b)
	if err != nil || sum.String() != "0.17" {
		t.Fatalf("Add = %s (err %v), want 0.17", sum.String(), err)
	}
	diff, err := a.Sub(b)
	if err != nil || diff.String() != "0.07" {
		t.Fatalf("Sub = %s (err %v), want 0.07", diff.String(), err)
	}
	if cmp, _ := a.Cmp(b); cmp != 1 {
		t.Fatalf("Cmp(0.12, 0.05) = %d, want 1", cmp)
	}
}

func TestDecimalsMismatch(t *testing.T) {
	a := New(big.NewInt(1), 8)
	b := New(big.NewInt(1), 18)
	if _, err := a.Add(b); !errors.Is(err, ErrDecimalsMismatch) {
		t.Fatalf("Add across decimals should error with ErrDecimalsMismatch, got %v", err)
	}
}

func TestRawIsCopied(t *testing.T) {
	src := big.NewInt(100)
	a := New(src, 8)
	src.SetInt64(999) // mutate caller's int
	if a.Raw().Int64() != 100 {
		t.Fatalf("New did not copy raw value; got %d", a.Raw().Int64())
	}
	out := a.Raw()
	out.SetInt64(7) // mutate returned int
	if a.Raw().Int64() != 100 {
		t.Fatalf("Raw() did not return a copy; amount mutated to %d", a.Raw().Int64())
	}
}

func TestKnownAssets(t *testing.T) {
	for sym, want := range map[string]int{"BTC": 8, "ETH": 18, "SOL": 9, "XRP": 6, "USDC": 6} {
		if d, ok := DecimalsFor(sym); !ok || d != want {
			t.Errorf("DecimalsFor(%s) = %d,%v want %d,true", sym, d, ok, want)
		}
	}
	if _, ok := DecimalsFor("DOGE"); ok {
		t.Errorf("DOGE should be unknown in v1 registry")
	}
}
