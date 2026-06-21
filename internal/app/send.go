package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/rhymeswithlimo/fivepoint/internal/chains"
	"github.com/rhymeswithlimo/fivepoint/internal/money"
	"github.com/rhymeswithlimo/fivepoint/internal/vault"
	"github.com/rhymeswithlimo/fivepoint/internal/wallet"
)

// Sending defaults to test networks. Mainnet sending stays disabled until it has
// passed a security review AND a real testnet send for the chain — see the
// build plan's Phase 3 gate. There is intentionally no binding that flips this
// at runtime yet.
var allowMainnetSend = false

// pending holds prepared-but-unconfirmed transactions between PrepareSend and
// ConfirmSend, so ConfirmSend broadcasts exactly what the user previewed.
type pending struct {
	mu  sync.Mutex
	txs map[string]*chains.PreparedTx
}

func newPending() *pending { return &pending{txs: map[string]*chains.PreparedTx{}} }

func (p *pending) put(id string, tx *chains.PreparedTx) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.txs[id] = tx
}

func (p *pending) take(id string) (*chains.PreparedTx, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	tx, ok := p.txs[id]
	delete(p.txs, id)
	return tx, ok
}

// SendPreview is the confirm-screen summary for a prepared transfer.
type SendPreview struct {
	PrepareID string `json:"prepareId"`
	Chain     string `json:"chain"`
	Network   string `json:"network"`
	From      string `json:"from"`
	To        string `json:"to"`
	Amount    string `json:"amount"`
	Fee       string `json:"fee"`
}

// PrepareSend builds (but does not broadcast) a transfer and returns a preview
// for confirmation. amount is a human decimal string in the asset's units.
func (a *App) PrepareSend(walletID, symbol, to, amount string) (SendPreview, error) {
	if a.wallets == nil {
		return SendPreview{}, vault.ErrLocked
	}
	sender, err := a.senderFor(symbol)
	if err != nil {
		return SendPreview{}, err
	}
	if sender.Network() == chains.Mainnet && !allowMainnetSend {
		return SendPreview{}, errors.New("mainnet sending is disabled pending security review; use testnet")
	}

	amt, err := money.ParseDecimal(amount, sender.Decimals())
	if err != nil {
		return SendPreview{}, fmt.Errorf("invalid amount: %w", err)
	}
	if amt.Sign() <= 0 {
		return SendPreview{}, errors.New("amount must be positive")
	}

	seed, err := a.wallets.Seed(walletID)
	if errors.Is(err, wallet.ErrNoSeed) {
		return SendPreview{}, errors.New("this wallet was imported as a single private key and cannot send yet")
	}
	if err != nil {
		return SendPreview{}, err
	}
	defer zeroBytes(seed)

	ctx, cancel := context.WithTimeout(a.baseCtx(), readTimeout)
	defer cancel()

	ptx, err := sender.Prepare(ctx, seed, to, amt)
	if err != nil {
		return SendPreview{}, err
	}

	id := randomID()
	a.pending.put(id, ptx)
	return SendPreview{
		PrepareID: id,
		Chain:     ptx.Chain,
		Network:   sender.Network().String(),
		From:      ptx.From,
		To:        ptx.To,
		Amount:    ptx.Amount.String(),
		Fee:       ptx.Fee.String(),
	}, nil
}

// ConfirmSend signs and broadcasts a previously prepared transfer, returning the
// transaction id. The prepared transaction is consumed (single use).
func (a *App) ConfirmSend(prepareID string) (string, error) {
	if a.wallets == nil {
		return "", vault.ErrLocked
	}
	ptx, ok := a.pending.take(prepareID)
	if !ok {
		return "", errors.New("no such pending transaction (it may have expired)")
	}
	ctx, cancel := context.WithTimeout(a.baseCtx(), readTimeout)
	defer cancel()
	return ptx.Send(ctx)
}

// CancelSend discards a prepared transaction.
func (a *App) CancelSend(prepareID string) {
	a.pending.take(prepareID)
}

func (a *App) senderFor(symbol string) (chains.Sender, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	for _, ch := range a.senders {
		if s, ok := ch.(chains.Sender); ok && s.Symbol() == symbol {
			return s, nil
		}
	}
	return nil, fmt.Errorf("sending %s is not supported yet", symbol)
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
