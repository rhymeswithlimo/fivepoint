package chains

import (
	"context"
	"crypto/rand"
	"math/big"
	"testing"
	"time"

	solana "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	bip39 "github.com/tyler-smith/go-bip39"
	"github.com/rhymeswithlimo/fivepoint/internal/money"
)

// End-to-end signing proof on Solana devnet: fund a freshly derived key via
// airdrop, send to a second derived address, and confirm the recipient's balance
// actually increased by the sent amount. This verifies INTENT on a live
// network — that funds move to the right place in the right amount — not just
// that a tx serialized. Skipped in -short; tolerant of devnet airdrop limits.
func TestSolanaSendDevnetLive(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Fresh wallet (real entropy) so we don't depend on shared/rate-limited funds.
	entropy := make([]byte, 16)
	if _, err := rand.Read(entropy); err != nil {
		t.Fatal(err)
	}
	mnemonic, _ := bip39.NewMnemonic(entropy)
	seed := bip39.NewSeed(mnemonic, "")

	s := NewSolana(Testnet, "") // devnet
	sender, _ := s.DeriveAddress(seed, 0)
	recipient, _ := s.DeriveAddress(seed, 1)
	t.Logf("sender=%s recipient=%s", sender, recipient)

	senderPub := solana.MustPublicKeyFromBase58(sender)
	recipientPub := solana.MustPublicKeyFromBase58(recipient)

	// Airdrop 1 SOL to the sender (devnet faucet is flaky → retry a few times).
	const airdrop = 1_000_000_000
	airdropped := false
	for i := 0; i < 6 && !airdropped; i++ {
		if _, err := s.sol.RequestAirdrop(ctx, senderPub, airdrop, rpc.CommitmentFinalized); err != nil {
			t.Logf("airdrop attempt %d failed: %v", i+1, err)
			time.Sleep(3 * time.Second)
			continue
		}
		airdropped = true
	}
	if !airdropped {
		t.Skip("devnet faucet unavailable after retries")
	}
	if !waitForBalance(ctx, t, s.sol, senderPub, airdrop) {
		t.Skip("devnet airdrop did not confirm in time (rate limited?)")
	}

	// Record recipient balance, then send 0.01 SOL.
	before := balanceOf(ctx, t, s.sol, recipientPub)
	const sendLamports = 10_000_000 // 0.01 SOL
	amt := money.New(big.NewInt(sendLamports), 9)

	ptx, err := s.Prepare(ctx, seed, recipient, amt)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	// Preview must reflect exactly what we asked for.
	if ptx.To != recipient || ptx.From != sender || ptx.Amount.Raw().Cmp(amt.Raw()) != 0 {
		t.Fatalf("preview mismatch: from=%s to=%s amount=%s", ptx.From, ptx.To, ptx.Amount.String())
	}

	txid, err := ptx.Send(ctx)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	t.Logf("sent txid=%s", txid)

	// Confirm the recipient actually received exactly the sent amount.
	want := before + sendLamports
	if !waitForBalance(ctx, t, s.sol, recipientPub, want) {
		got := balanceOf(ctx, t, s.sol, recipientPub)
		t.Fatalf("recipient balance = %d, want %d (delta %d)", got, want, got-before)
	}
	t.Logf("confirmed: recipient credited %d lamports", sendLamports)
}

func balanceOf(ctx context.Context, t *testing.T, c *rpc.Client, pub solana.PublicKey) uint64 {
	t.Helper()
	res, err := c.GetBalance(ctx, pub, rpc.CommitmentConfirmed)
	if err != nil {
		return 0
	}
	return res.Value
}

func waitForBalance(ctx context.Context, t *testing.T, c *rpc.Client, pub solana.PublicKey, atLeast uint64) bool {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if balanceOf(ctx, t, c, pub) >= atLeast {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(2 * time.Second):
		}
	}
	return false
}
