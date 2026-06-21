// Package prices fetches USD spot prices and multi-period change percentages for
// supported assets from CoinGecko, with a short in-memory cache to stay within
// the public API's rate limits. USD figures are display values only; crypto
// amounts are always handled as integer base units elsewhere (internal/money).
package prices

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	apiBase  = "https://api.coingecko.com/api/v3"
	cacheTTL = 60 * time.Second
)

// symbolToID maps asset tickers to CoinGecko coin IDs.
var symbolToID = map[string]string{
	"BTC":  "bitcoin",
	"ETH":  "ethereum",
	"SOL":  "solana",
	"XRP":  "ripple",
	"USDC": "usd-coin",
	"USDT": "tether",
}

// Quote is the current price and trailing change percentages for one asset.
type Quote struct {
	USD       float64 `json:"usd"`
	Change1d  float64 `json:"change1d"`
	Change7d  float64 `json:"change7d"`
	Change30d float64 `json:"change30d"`
	Change1y  float64 `json:"change1y"`
}

// Client fetches and caches quotes.
type Client struct {
	http  *http.Client
	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	quote Quote
	at    time.Time
}

// New returns a price client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 20 * time.Second}, cache: map[string]cached{}}
}

// Get returns quotes for the given symbols, serving fresh cache entries and
// fetching the rest in a single request.
func (c *Client) Get(ctx context.Context, symbols []string) (map[string]Quote, error) {
	out := make(map[string]Quote, len(symbols))
	var missing []string

	c.mu.Lock()
	for _, s := range symbols {
		s = strings.ToUpper(s)
		if e, ok := c.cache[s]; ok && time.Since(e.at) < cacheTTL {
			out[s] = e.quote
		} else if _, known := symbolToID[s]; known {
			missing = append(missing, s)
		}
	}
	c.mu.Unlock()

	if len(missing) == 0 {
		return out, nil
	}

	ids := make([]string, 0, len(missing))
	idToSymbol := map[string]string{}
	for _, s := range missing {
		id := symbolToID[s]
		ids = append(ids, id)
		idToSymbol[id] = s
	}

	fetched, err := c.fetch(ctx, ids)
	if err != nil {
		// Degrade gracefully: return whatever cache provided rather than failing
		// the whole portfolio because prices are unavailable.
		return out, err
	}

	c.mu.Lock()
	for id, q := range fetched {
		if sym, ok := idToSymbol[id]; ok {
			c.cache[sym] = cached{quote: q, at: time.Now()}
			out[sym] = q
		}
	}
	c.mu.Unlock()
	return out, nil
}

type marketEntry struct {
	ID            string  `json:"id"`
	CurrentPrice  float64 `json:"current_price"`
	Change24h     float64 `json:"price_change_percentage_24h_in_currency"`
	Change7d      float64 `json:"price_change_percentage_7d_in_currency"`
	Change30d     float64 `json:"price_change_percentage_30d_in_currency"`
	Change1y      float64 `json:"price_change_percentage_1y_in_currency"`
}

func (c *Client) fetch(ctx context.Context, ids []string) (map[string]Quote, error) {
	u := fmt.Sprintf("%s/coins/markets?vs_currency=usd&ids=%s&price_change_percentage=24h,7d,30d,1y",
		apiBase, url.QueryEscape(strings.Join(ids, ",")))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Fivepoint/0.1")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("coingecko: http %d", resp.StatusCode)
	}

	var entries []marketEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("coingecko: decode: %w", err)
	}
	out := make(map[string]Quote, len(entries))
	for _, e := range entries {
		out[e.ID] = Quote{
			USD:       e.CurrentPrice,
			Change1d:  e.Change24h,
			Change7d:  e.Change7d,
			Change30d: e.Change30d,
			Change1y:  e.Change1y,
		}
	}
	return out, nil
}
