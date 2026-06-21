package chains

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"

	"github.com/btcsuite/btcd/btcutil/base58"
	"github.com/rhymeswithlimo/fivepoint/internal/money"
	"golang.org/x/crypto/ripemd160"
)

// Public XRP Ledger JSON-RPC endpoints (mainnet and the Testnet altnet).
const (
	mainnetXRPEndpoint = "https://xrplcluster.com/"
	testnetXRPEndpoint = "https://s.altnet.rippletest.net:51234/"
)

// Bitcoin and Ripple share the same 58-character set in different orders; XRP
// addresses are standard base58check re-mapped into the Ripple ordering.
const (
	btcAlphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	// Ripple's base58 dictionary — a specific (non-obvious) permutation of the
	// same 58 characters. Verified against xrpl.org and ACCOUNT_ZERO (xrp_test.go).
	rippleAlphabet = "rpshnaf39wBUDNEGHJKLM4PQRST7VWXYZ2bcdeCg65jkm8oFqi1tuvAxyz"
)

// XRP derives XRP Ledger addresses and reads native XRP balances.
type XRP struct {
	net      Network
	endpoint string
}

// NewXRP returns the XRP chain for the network. An empty endpoint uses the
// default for that network.
func NewXRP(net Network, endpoint string) *XRP {
	if endpoint == "" {
		if net == Testnet {
			endpoint = testnetXRPEndpoint
		} else {
			endpoint = mainnetXRPEndpoint
		}
	}
	return &XRP{net: net, endpoint: endpoint}
}

func (x *XRP) Symbol() string   { return "XRP" }
func (x *XRP) Name() string     { return "XRP" }
func (x *XRP) Decimals() int    { return 6 }
func (x *XRP) Network() Network { return x.net }

// DeriveAddress derives the classic XRP address (r…) at m/44'/144'/0'/0/index
// (BIP-44, coin type 144 — the path Ledger and most XRP wallets use).
func (x *XRP) DeriveAddress(seed []byte, index uint32) (string, error) {
	key, err := deriveSecp256k1(seed, []uint32{hard + 44, hard + 144, hard + 0, 0, index})
	if err != nil {
		return "", err
	}
	pub, err := key.ECPubKey()
	if err != nil {
		return "", err
	}
	sha := sha256.Sum256(pub.SerializeCompressed())
	rip := ripemd160.New()
	rip.Write(sha[:])
	accountID := rip.Sum(nil) // 20 bytes

	// base58check with version 0x00 (typeAccountID) uses the same double-SHA256
	// checksum as Bitcoin; only the alphabet differs.
	encoded := base58.CheckEncode(accountID, 0x00)
	return remapAlphabet(encoded, btcAlphabet, rippleAlphabet), nil
}

func remapAlphabet(s, from, to string) string {
	out := []byte(s)
	for i, c := range out {
		idx := bytes.IndexByte([]byte(from), c)
		if idx < 0 {
			continue
		}
		out[i] = to[idx]
	}
	return string(out)
}

// Balance returns the native XRP balance (in drops, 6 decimals). An unfunded
// account (actNotFound) reports zero.
func (x *XRP) Balance(ctx context.Context, address string) (money.Amount, error) {
	reqBody, _ := json.Marshal(map[string]any{
		"method": "account_info",
		"params": []any{map[string]any{"account": address, "ledger_index": "validated"}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, x.endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return money.Amount{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return money.Amount{}, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))

	var out struct {
		Result struct {
			AccountData struct {
				Balance string `json:"Balance"`
			} `json:"account_data"`
			Error  string `json:"error"`
			Status string `json:"status"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return money.Amount{}, fmt.Errorf("xrp: decode: %w", err)
	}
	if out.Result.Error == "actNotFound" {
		return money.Zero(x.Decimals()), nil // unfunded account
	}
	if out.Result.Error != "" {
		return money.Amount{}, fmt.Errorf("xrp: %s", out.Result.Error)
	}
	drops, ok := new(big.Int).SetString(out.Result.AccountData.Balance, 10)
	if !ok {
		return money.Amount{}, fmt.Errorf("xrp: bad balance %q", out.Result.AccountData.Balance)
	}
	return money.New(drops, x.Decimals()), nil
}
