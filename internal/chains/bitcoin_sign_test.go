package chains

import (
	"errors"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
)

// btcTestKeys derives a sender (key + P2WPKH script) and a recipient script from
// the standard test seed, for the BTC signing tests.
func btcTestScripts(t *testing.T) (senderScript, recipientScript []byte) {
	t.Helper()
	b := NewBitcoin(Mainnet, "")
	mk := func(index uint32) (*btcutil.AddressWitnessPubKeyHash, error) {
		hk, err := deriveSecp256k1(testSeed(), []uint32{hard + 84, b.coinType(), hard + 0, 0, index})
		if err != nil {
			return nil, err
		}
		pub, err := hk.ECPubKey()
		if err != nil {
			return nil, err
		}
		return btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(pub.SerializeCompressed()), b.params())
	}
	sa, err := mk(0)
	if err != nil {
		t.Fatal(err)
	}
	ra, err := mk(1)
	if err != nil {
		t.Fatal(err)
	}
	ss, _ := txscript.PayToAddrScript(sa)
	rs, _ := txscript.PayToAddrScript(ra)
	return ss, rs
}

func sumOutputs(values ...int64) int64 {
	var s int64
	for _, v := range values {
		s += v
	}
	return s
}

// The invariant that prevents the classic homemade-wallet disaster: every
// satoshi of input is accounted for as either an output or the fee — change is
// never silently burned. Also confirms change returns to the sender.
func TestBitcoinChangeInvariant(t *testing.T) {
	senderScript, recipientScript := btcTestScripts(t)
	utxos := []btcUTXO{
		{TxID: strings.Repeat("a", 64), Vout: 0, Value: 100_000},
		{TxID: strings.Repeat("b", 64), Vout: 1, Value: 50_000},
	}
	const amount = 120_000
	const feeRate = 10

	tx, selected, fee, err := buildBTCTx(utxos, senderScript, recipientScript, amount, feeRate)
	if err != nil {
		t.Fatalf("buildBTCTx: %v", err)
	}

	var in int64
	for _, u := range selected {
		in += u.Value
	}
	var out int64
	for _, o := range tx.TxOut {
		out += o.Value
	}
	if in != out+fee {
		t.Fatalf("value not conserved: inputs=%d outputs=%d fee=%d", in, out, fee)
	}

	// Recipient must receive exactly `amount`, and change must go back to sender.
	if tx.TxOut[0].Value != amount || string(tx.TxOut[0].PkScript) != string(recipientScript) {
		t.Fatalf("recipient output wrong: %d", tx.TxOut[0].Value)
	}
	if len(tx.TxOut) != 2 || string(tx.TxOut[1].PkScript) != string(senderScript) {
		t.Fatalf("expected change output back to sender")
	}
}

// Change below the dust threshold is folded into the fee (no dust output),
// preserving the value invariant.
func TestBitcoinDustChangeFoldedToFee(t *testing.T) {
	senderScript, recipientScript := btcTestScripts(t)
	// One UTXO barely above amount: leftover < dust.
	utxos := []btcUTXO{{TxID: strings.Repeat("c", 64), Vout: 0, Value: 100_000}}
	const amount = 99_800 // leftover 200 < dust(294)

	tx, selected, fee, err := buildBTCTx(utxos, senderScript, recipientScript, amount, 1)
	if err != nil {
		t.Fatalf("buildBTCTx: %v", err)
	}
	if len(tx.TxOut) != 1 {
		t.Fatalf("expected no change output, got %d outputs", len(tx.TxOut))
	}
	if selected[0].Value != sumOutputs(tx.TxOut[0].Value)+fee {
		t.Fatalf("value not conserved with folded change")
	}
}

func TestBitcoinInsufficientFunds(t *testing.T) {
	senderScript, recipientScript := btcTestScripts(t)
	utxos := []btcUTXO{{TxID: strings.Repeat("d", 64), Vout: 0, Value: 1000}}
	if _, _, _, err := buildBTCTx(utxos, senderScript, recipientScript, 5000, 10); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("want ErrInsufficientFunds, got %v", err)
	}
}

// Sign the built tx and validate every input through btcd's script engine — the
// same verification a full node performs. If the engine accepts the witness, the
// P2WPKH (BIP-143) signature is correct.
func TestBitcoinSignValidatesInEngine(t *testing.T) {
	seed := testSeed()
	b := NewBitcoin(Mainnet, "")
	hk, _ := deriveSecp256k1(seed, []uint32{hard + 84, b.coinType(), hard + 0, 0, 0})
	priv, _ := hk.ECPrivKey()
	pub, _ := hk.ECPubKey()
	senderAddr, _ := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(pub.SerializeCompressed()), b.params())
	senderScript, _ := txscript.PayToAddrScript(senderAddr)
	_, recipientScript := btcTestScripts(t)

	utxos := []btcUTXO{
		{TxID: strings.Repeat("a", 64), Vout: 0, Value: 100_000},
		{TxID: strings.Repeat("b", 64), Vout: 1, Value: 80_000},
	}
	tx, selected, _, err := buildBTCTx(utxos, senderScript, recipientScript, 120_000, 5)
	if err != nil {
		t.Fatalf("buildBTCTx: %v", err)
	}
	if err := signBTCTx(tx, selected, senderScript, priv); err != nil {
		t.Fatalf("signBTCTx: %v", err)
	}

	fetcher, _ := prevOutFetcher(selected, senderScript)
	sigHashes := txscript.NewTxSigHashes(tx, fetcher)
	for i, u := range selected {
		vm, err := txscript.NewEngine(senderScript, tx, i, txscript.StandardVerifyFlags, nil, sigHashes, u.Value, fetcher)
		if err != nil {
			t.Fatalf("NewEngine input %d: %v", i, err)
		}
		if err := vm.Execute(); err != nil {
			t.Fatalf("script engine rejected input %d: %v", i, err)
		}
	}
}
