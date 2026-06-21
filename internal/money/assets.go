package money

import "strings"

// Asset is static metadata about a coin or token. Decimals is the number of
// fractional digits the asset uses on-chain and is the single source of truth
// for converting between base units and human-readable values.
type Asset struct {
	Symbol   string
	Name     string
	Decimals int
}

// knownAssets holds the assets Fivepoint understands out of the box. It is kept
// deliberately small for v1 (the chains the user chose); tokens discovered at
// runtime (e.g. arbitrary ERC-20s) carry their own decimals from the chain.
var knownAssets = map[string]Asset{
	"BTC":  {Symbol: "BTC", Name: "Bitcoin", Decimals: 8},
	"ETH":  {Symbol: "ETH", Name: "Ethereum", Decimals: 18},
	"SOL":  {Symbol: "SOL", Name: "Solana", Decimals: 9},
	"XRP":  {Symbol: "XRP", Name: "XRP", Decimals: 6},
	"USDC": {Symbol: "USDC", Name: "USD Coin", Decimals: 6},
	"USDT": {Symbol: "USDT", Name: "Tether", Decimals: 6},
}

// Lookup returns the metadata for a known asset symbol (case-insensitive).
func Lookup(symbol string) (Asset, bool) {
	a, ok := knownAssets[strings.ToUpper(strings.TrimSpace(symbol))]
	return a, ok
}

// DecimalsFor returns the decimal precision for a known symbol, or false if the
// symbol is unknown. Callers must supply decimals explicitly for unknown tokens.
func DecimalsFor(symbol string) (int, bool) {
	if a, ok := Lookup(symbol); ok {
		return a.Decimals, true
	}
	return 0, false
}
