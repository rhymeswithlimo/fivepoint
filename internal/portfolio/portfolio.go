// Package portfolio turns a wallet's seed into a valued, multi-asset portfolio:
// it derives an address per chain, fetches balances concurrently, prices them in
// USD, and computes value-weighted change percentages over several periods.
//
// USD amounts are display values (float64); the underlying crypto balances are
// always exact integer base units (internal/money).
package portfolio

import (
	"context"
	"sort"
	"strconv"
	"sync"

	"github.com/rhymeswithlimo/fivepoint/internal/chains"
	"github.com/rhymeswithlimo/fivepoint/internal/prices"
)

// Asset is one chain's holding within a portfolio.
type Asset struct {
	Symbol    string  `json:"symbol"`
	Name      string  `json:"name"`
	Address   string  `json:"address"`
	Amount    string  `json:"amount"`   // native units, decimal string
	PriceUSD  float64 `json:"priceUsd"`
	ValueUSD  float64 `json:"valueUsd"`
	Change1d  float64 `json:"change1d"`
	Change7d  float64 `json:"change7d"`
	Change30d float64 `json:"change30d"`
	Change1y  float64 `json:"change1y"`
	Error     string  `json:"error,omitempty"` // set if balance lookup failed
}

// Summary is a fully valued portfolio for a wallet.
type Summary struct {
	TotalUSD    float64 `json:"totalUsd"`
	ChangeUSD1d float64 `json:"changeUsd1d"`
	Change1d    float64 `json:"change1d"`
	Change7d    float64 `json:"change7d"`
	Change30d   float64 `json:"change30d"`
	Change1y    float64 `json:"change1y"`
	Assets      []Asset `json:"assets"`
}

// Builder assembles portfolios from a fixed set of chains and a price source.
type Builder struct {
	chains []chains.Chain
	prices *prices.Client
}

// New returns a portfolio builder.
func New(chs []chains.Chain, p *prices.Client) *Builder {
	return &Builder{chains: chs, prices: p}
}

// ForSeed builds the valued portfolio for a BIP-39 seed. A failure on one chain
// (network/RPC) degrades that asset to a zero balance with an Error rather than
// failing the whole portfolio.
func (b *Builder) ForSeed(ctx context.Context, seed []byte) (Summary, error) {
	type result struct {
		asset Asset
	}
	results := make([]result, len(b.chains))

	var wg sync.WaitGroup
	for i, ch := range b.chains {
		wg.Add(1)
		go func(i int, ch chains.Chain) {
			defer wg.Done()
			a := Asset{Symbol: ch.Symbol(), Name: ch.Name()}
			addr, err := ch.DeriveAddress(seed, 0)
			if err != nil {
				a.Error = err.Error()
				a.Amount = "0"
				results[i] = result{a}
				return
			}
			a.Address = addr
			bal, err := ch.Balance(ctx, addr)
			if err != nil {
				a.Error = err.Error()
				a.Amount = "0"
				results[i] = result{a}
				return
			}
			a.Amount = bal.String()
			results[i] = result{a}
		}(i, ch)
	}
	wg.Wait()

	// Price everything in one (cached) request.
	symbols := make([]string, 0, len(b.chains))
	for _, ch := range b.chains {
		symbols = append(symbols, ch.Symbol())
	}
	quotes, _ := b.prices.Get(ctx, symbols) // price failure → zero-valued, non-fatal

	var sum Summary
	for i := range results {
		a := results[i].asset
		q := quotes[a.Symbol]

		// amount (decimal string) -> float for valuation only.
		amt, _ := strconv.ParseFloat(a.Amount, 64)
		a.PriceUSD = q.USD
		a.ValueUSD = amt * q.USD
		a.Change1d, a.Change7d, a.Change30d, a.Change1y = q.Change1d, q.Change7d, q.Change30d, q.Change1y

		sum.Assets = append(sum.Assets, a)
		sum.TotalUSD += a.ValueUSD
		// Value-weighted change: contribution = value * pct/100.
		sum.ChangeUSD1d += a.ValueUSD * q.Change1d / 100
		sum.Change7d += a.ValueUSD * q.Change7d / 100
		sum.Change30d += a.ValueUSD * q.Change30d / 100
		sum.Change1y += a.ValueUSD * q.Change1y / 100
	}

	// Convert weighted USD contributions back to portfolio-level percentages.
	if sum.TotalUSD > 0 {
		sum.Change1d = sum.ChangeUSD1d / sum.TotalUSD * 100
		sum.Change7d = sum.Change7d / sum.TotalUSD * 100
		sum.Change30d = sum.Change30d / sum.TotalUSD * 100
		sum.Change1y = sum.Change1y / sum.TotalUSD * 100
	}

	// Largest holdings first.
	sort.SliceStable(sum.Assets, func(i, j int) bool {
		return sum.Assets[i].ValueUSD > sum.Assets[j].ValueUSD
	})
	return sum, nil
}
