package chains

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/rhymeswithlimo/fivepoint/internal/money"
)

// dustP2WPKH is the minimum economical output value for a native SegWit output.
const dustP2WPKH = 294

// ErrInsufficientFunds indicates the wallet's UTXOs cannot cover amount + fee.
var ErrInsufficientFunds = errors.New("bitcoin: insufficient funds")

// btcUTXO is one unspent output, as returned by Esplora /address/{a}/utxo.
type btcUTXO struct {
	TxID  string `json:"txid"`
	Vout  uint32 `json:"vout"`
	Value int64  `json:"value"` // satoshi
}

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

// vsize estimates the virtual size (vbytes) of a P2WPKH transaction.
func btcVsize(numIn, numOut int) int64 {
	return int64(11 + 68*numIn + 31*numOut)
}

// buildBTCTx performs coin selection and constructs the unsigned transaction.
// It guarantees value conservation: sum(selected inputs) == sum(outputs) + fee.
// Change returns to senderScript; change below the dust threshold is dropped
// into the fee (a standard, unavoidable rule — never silently lost to a bug).
// Pure (no network/signing) so the change invariant is unit-testable.
func buildBTCTx(utxos []btcUTXO, senderScript, recipientScript []byte, amount, feeRate int64) (*wire.MsgTx, []btcUTXO, int64, error) {
	if amount <= 0 {
		return nil, nil, 0, errors.New("bitcoin: amount must be positive")
	}
	// Largest-first selection keeps the input count (and fee) low.
	sorted := append([]btcUTXO(nil), utxos...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Value > sorted[j].Value })

	var selected []btcUTXO
	var total, fee int64
	enough := false
	for _, u := range sorted {
		selected = append(selected, u)
		total += u.Value
		fee = btcVsize(len(selected), 2) * feeRate // assume a change output
		if total >= amount+fee {
			enough = true
			break
		}
	}
	if !enough {
		return nil, nil, 0, ErrInsufficientFunds
	}

	tx := wire.NewMsgTx(2)
	tx.AddTxOut(wire.NewTxOut(amount, recipientScript))

	change := total - amount - fee
	if change >= dustP2WPKH {
		tx.AddTxOut(wire.NewTxOut(change, senderScript))
	} else {
		// Change too small to be its own output: it becomes part of the fee.
		fee = total - amount
	}

	for _, u := range selected {
		h, err := chainhash.NewHashFromStr(u.TxID)
		if err != nil {
			return nil, nil, 0, err
		}
		tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(h, u.Vout), nil, nil))
	}
	return tx, selected, fee, nil
}

// prevOutFetcher builds the BIP-143 previous-output set (all inputs spend the
// sender's own P2WPKH script).
func prevOutFetcher(selected []btcUTXO, senderScript []byte) (*txscript.MultiPrevOutFetcher, error) {
	f := txscript.NewMultiPrevOutFetcher(nil)
	for _, u := range selected {
		h, err := chainhash.NewHashFromStr(u.TxID)
		if err != nil {
			return nil, err
		}
		f.AddPrevOut(*wire.NewOutPoint(h, u.Vout), wire.NewTxOut(u.Value, senderScript))
	}
	return f, nil
}

// signBTCTx signs every input as P2WPKH (BIP-143 segwit sighash) in place.
func signBTCTx(tx *wire.MsgTx, selected []btcUTXO, senderScript []byte, key *btcec.PrivateKey) error {
	fetcher, err := prevOutFetcher(selected, senderScript)
	if err != nil {
		return err
	}
	sigHashes := txscript.NewTxSigHashes(tx, fetcher)
	for i, u := range selected {
		witness, err := txscript.WitnessSignature(tx, sigHashes, i, u.Value, senderScript, txscript.SigHashAll, key, true)
		if err != nil {
			return err
		}
		tx.TxIn[i].Witness = witness
	}
	return nil
}

// Prepare builds a BTC transfer: fetch UTXOs, select coins, construct outputs
// (recipient + change to self), and capture a signing+broadcast closure. The
// transaction is fully built here; Send signs and broadcasts that exact tx.
func (b *Bitcoin) Prepare(ctx context.Context, seed []byte, to string, amount money.Amount) (*PreparedTx, error) {
	hk, err := deriveSecp256k1(seed, []uint32{hard + 84, b.coinType(), hard + 0, 0, 0})
	if err != nil {
		return nil, err
	}
	priv, err := hk.ECPrivKey()
	if err != nil {
		return nil, err
	}
	pub, err := hk.ECPubKey()
	if err != nil {
		return nil, err
	}
	senderAddr, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(pub.SerializeCompressed()), b.params())
	if err != nil {
		return nil, err
	}
	senderScript, err := txscript.PayToAddrScript(senderAddr)
	if err != nil {
		return nil, err
	}

	recipient, err := btcutil.DecodeAddress(to, b.params())
	if err != nil {
		return nil, fmt.Errorf("bitcoin: invalid recipient address: %w", err)
	}
	recipientScript, err := txscript.PayToAddrScript(recipient)
	if err != nil {
		return nil, err
	}

	var utxos []btcUTXO
	if err := httpGetJSON(ctx, b.explorer+"/address/"+senderAddr.EncodeAddress()+"/utxo", &utxos); err != nil {
		return nil, err
	}
	feeRate := b.feeRate(ctx)

	tx, selected, fee, err := buildBTCTx(utxos, senderScript, recipientScript, amount.Raw().Int64(), feeRate)
	if err != nil {
		return nil, err
	}

	send := func(ctx context.Context) (string, error) {
		if err := signBTCTx(tx, selected, senderScript, priv); err != nil {
			return "", err
		}
		var buf bytes.Buffer
		if err := tx.Serialize(&buf); err != nil {
			return "", err
		}
		return b.broadcast(ctx, buf.Bytes())
	}

	return &PreparedTx{
		Chain:  b.Symbol(),
		From:   senderAddr.EncodeAddress(),
		To:     to,
		Amount: amount,
		Fee:    money.New(big.NewInt(fee), b.Decimals()),
		send:   send,
	}, nil
}

// feeRate returns a sat/vByte fee rate from the explorer, defaulting to a small
// conservative rate (testnet fees are tiny) if estimates are unavailable.
func (b *Bitcoin) feeRate(ctx context.Context) int64 {
	var est map[string]float64
	if err := httpGetJSON(ctx, b.explorer+"/fee-estimates", &est); err == nil {
		if v, ok := est["6"]; ok && v >= 1 {
			return int64(v)
		}
	}
	return 2
}

// broadcast posts a raw transaction (hex) to the Esplora /tx endpoint and
// returns the txid.
func (b *Bitcoin) broadcast(ctx context.Context, raw []byte) (string, error) {
	return httpPostText(ctx, b.explorer+"/tx", fmt.Sprintf("%x", raw))
}
