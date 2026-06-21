package chains

import (
	"context"
	"math/big"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/rhymeswithlimo/fivepoint/internal/money"
)

// Blockstream Esplora REST APIs (mainnet and testnet). Bitcoin balances come
// from an indexer/explorer (UTXO sums), not a node RPC.
const (
	mainnetBTCExplorer = "https://blockstream.info/api"
	testnetBTCExplorer = "https://blockstream.info/testnet/api"
)

// Bitcoin derives native SegWit (BIP-84) addresses and reads balances from a
// Blockstream-compatible Esplora explorer.
type Bitcoin struct {
	net      Network
	explorer string
}

// NewBitcoin returns the Bitcoin chain for the network. An empty explorer uses
// the default for that network.
func NewBitcoin(net Network, explorer string) *Bitcoin {
	if explorer == "" {
		if net == Testnet {
			explorer = testnetBTCExplorer
		} else {
			explorer = mainnetBTCExplorer
		}
	}
	return &Bitcoin{net: net, explorer: explorer}
}

func (b *Bitcoin) Symbol() string   { return "BTC" }
func (b *Bitcoin) Name() string     { return "Bitcoin" }
func (b *Bitcoin) Decimals() int    { return 8 }
func (b *Bitcoin) Network() Network { return b.net }

func (b *Bitcoin) params() *chaincfg.Params {
	if b.net == Testnet {
		return &chaincfg.TestNet3Params
	}
	return &chaincfg.MainNetParams
}

// coinType is 0' on mainnet and 1' on every test network (BIP-44).
func (b *Bitcoin) coinType() uint32 {
	if b.net == Testnet {
		return hard + 1
	}
	return hard + 0
}

// DeriveAddress derives the native SegWit (bech32) address at
// m/84'/{0,1}'/0'/0/index — BIP-84, the modern default for Bitcoin wallets
// (Electrum, Ledger, BlueWallet), so imported seeds match. Mainnet yields bc1…,
// testnet yields tb1….
func (b *Bitcoin) DeriveAddress(seed []byte, index uint32) (string, error) {
	key, err := deriveSecp256k1(seed, []uint32{hard + 84, b.coinType(), hard + 0, 0, index})
	if err != nil {
		return "", err
	}
	pub, err := key.ECPubKey()
	if err != nil {
		return "", err
	}
	hash160 := btcutil.Hash160(pub.SerializeCompressed())
	addr, err := btcutil.NewAddressWitnessPubKeyHash(hash160, b.params())
	if err != nil {
		return "", err
	}
	return addr.EncodeAddress(), nil
}

type esploraStats struct {
	FundedSum int64 `json:"funded_txo_sum"`
	SpentSum  int64 `json:"spent_txo_sum"`
}

type esploraAddress struct {
	ChainStats   esploraStats `json:"chain_stats"`
	MempoolStats esploraStats `json:"mempool_stats"`
}

// Balance returns the spendable BTC balance (confirmed + mempool), in satoshi.
func (b *Bitcoin) Balance(ctx context.Context, address string) (money.Amount, error) {
	var out esploraAddress
	if err := httpGetJSON(ctx, b.explorer+"/address/"+address, &out); err != nil {
		return money.Amount{}, err
	}
	sats := (out.ChainStats.FundedSum - out.ChainStats.SpentSum) +
		(out.MempoolStats.FundedSum - out.MempoolStats.SpentSum)
	return money.New(big.NewInt(sats), b.Decimals()), nil
}
