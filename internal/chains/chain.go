// Package chains implements per-blockchain address derivation and read-only
// balance lookups behind a single Chain interface, so the rest of Fivepoint is
// agnostic to whether an asset is UTXO-based (BTC), account-based (EVM), or uses
// a different signature scheme (Solana ed25519, XRP). Signing/broadcasting is
// added per-chain in a later, gated phase.
package chains

import (
	"context"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/rhymeswithlimo/fivepoint/internal/money"
)

// hard is the offset that marks a BIP-32 derivation index as hardened.
const hard = hdkeychain.HardenedKeyStart

// Network selects mainnet or a chain's test network (testnet/devnet/altnet).
// Signing defaults to Testnet; mainnet sending is gated separately.
type Network int

const (
	Mainnet Network = iota
	Testnet
)

func (n Network) String() string {
	if n == Testnet {
		return "testnet"
	}
	return "mainnet"
}

// Chain is a read-only view of a blockchain: derive an address from a seed and
// look up its native-asset balance.
type Chain interface {
	// Symbol is the native asset ticker (e.g. "ETH", "BTC", "SOL", "XRP").
	Symbol() string
	// Name is the human-readable chain name.
	Name() string
	// Decimals is the native asset's precision (matches money base units).
	Decimals() int
	// DeriveAddress derives the receive address at the given account index from a
	// BIP-39 seed, using the chain's standard derivation path.
	DeriveAddress(seed []byte, index uint32) (string, error)
	// Balance returns the native-asset balance for an address.
	Balance(ctx context.Context, address string) (money.Amount, error)
}

// Config holds optional user-supplied endpoints. Empty fields fall back to the
// public defaults; users can override them in settings for reliability.
type Config struct {
	EthRPC      string
	BTCExplorer string
	SolanaRPC   string
	XRPEndpoint string
}

// Default returns the v1 chain set (BTC, ETH, SOL, XRP) for the given network,
// configured from cfg.
func Default(net Network, cfg Config) []Chain {
	return []Chain{
		NewBitcoin(net, cfg.BTCExplorer),
		NewEthereum(net, cfg.EthRPC),
		NewSolana(net, cfg.SolanaRPC),
		NewXRP(net, cfg.XRPEndpoint),
	}
}

// PreparedTx is a built, previewable transaction awaiting user confirmation.
// Its preview fields are shown on the confirm screen; Send broadcasts the exact
// transaction that was prepared (it never rebuilds), so the user signs what they
// saw.
type PreparedTx struct {
	Chain  string       // asset symbol
	From   string       // sender address
	To     string       // recipient address
	Amount money.Amount // amount to send (native units)
	Fee    money.Amount // network fee (native units)

	send func(context.Context) (string, error) // signs+broadcasts, returns txid
}

// Send signs and broadcasts the prepared transaction, returning the txid.
func (p *PreparedTx) Send(ctx context.Context) (string, error) { return p.send(ctx) }

// Sender is a Chain that can also build and broadcast value transfers.
type Sender interface {
	Chain
	// Network reports whether this instance targets mainnet or a test network.
	Network() Network
	// Prepare builds (but does not broadcast) a transfer of amount to `to`,
	// returning a previewable PreparedTx.
	Prepare(ctx context.Context, seed []byte, to string, amount money.Amount) (*PreparedTx, error)
}

// deriveSecp256k1 walks a BIP-32 path over the secp256k1 curve (used by BTC,
// EVM, and XRP). The path is a sequence of child indices, hardened ones already
// offset by `hard`.
func deriveSecp256k1(seed []byte, path []uint32) (*hdkeychain.ExtendedKey, error) {
	key, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	if err != nil {
		return nil, err
	}
	for _, index := range path {
		key, err = key.Derive(index)
		if err != nil {
			return nil, err
		}
	}
	return key, nil
}
