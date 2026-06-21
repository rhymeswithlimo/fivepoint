package chains

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// userAgent identifies Fivepoint to public endpoints; some providers behind a
// CDN reject requests with the default Go user agent.
const userAgent = "Fivepoint/0.1 (+https://github.com/rhymeswithlimo/fivepoint)"

// rpcClient is a minimal JSON-RPC 2.0 client used by the account-based chains
// (EVM, Solana, XRP). Balance reads need only a single round trip, so this
// stays deliberately small rather than pulling each chain's full SDK client.
type rpcClient struct {
	url  string
	http *http.Client
}

func newRPC(url string) *rpcClient {
	return &rpcClient{url: url, http: &http.Client{Timeout: 20 * time.Second}}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message) }

// call issues a JSON-RPC request and decodes result into out.
func (c *rpcClient) call(ctx context.Context, method string, params any, out any) error {
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("rpc %s: http %d: %s", method, resp.StatusCode, truncate(data))
	}

	var r rpcResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return fmt.Errorf("rpc %s: decode: %w", method, err)
	}
	if r.Error != nil {
		return r.Error
	}
	if out != nil {
		return json.Unmarshal(r.Result, out)
	}
	return nil
}

func truncate(b []byte) string {
	if len(b) > 200 {
		return string(b[:200]) + "…"
	}
	return string(b)
}

// httpGetJSON is a small helper for REST endpoints (used for BTC, whose balances
// come from an indexer/explorer rather than a node RPC).
func httpGetJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: http %d: %s", url, resp.StatusCode, truncate(data))
	}
	return json.Unmarshal(data, out)
}
