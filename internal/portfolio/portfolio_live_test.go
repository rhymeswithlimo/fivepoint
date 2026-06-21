package portfolio

import (
	"context"
	"testing"
	"time"

	bip39 "github.com/tyler-smith/go-bip39"
	"github.com/rhymeswithlimo/fivepoint/internal/chains"
	"github.com/rhymeswithlimo/fivepoint/internal/prices"
)

func TestPortfolioLive(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	seed := bip39.NewSeed("abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about", "")
	b := New(chains.Default(chains.Mainnet, chains.Config{}), prices.New())
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	sum, err := b.ForSeed(ctx, seed)
	if err != nil {
		t.Fatalf("ForSeed: %v", err)
	}
	if len(sum.Assets) != 4 {
		t.Fatalf("want 4 assets, got %d", len(sum.Assets))
	}
	priced := 0
	for _, a := range sum.Assets {
		t.Logf("%-4s amount=%s price=$%.2f value=$%.2f 1d=%.2f%% err=%q",
			a.Symbol, a.Amount, a.PriceUSD, a.ValueUSD, a.Change1d, a.Error)
		if a.PriceUSD > 0 {
			priced++
		}
	}
	if priced < 3 {
		t.Fatalf("expected at least 3 assets priced, got %d", priced)
	}
}
