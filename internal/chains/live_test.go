package chains

import (
	"context"
	"testing"
	"time"
)

// Live network smokes; skipped in -short. Prove the read path end-to-end.
func liveCtx(t *testing.T) (context.Context, context.CancelFunc) {
	if testing.Short() {
		t.Skip("network")
	}
	return context.WithTimeout(context.Background(), 30*time.Second)
}

func TestEVMBalanceLive(t *testing.T) {
	ctx, cancel := liveCtx(t); defer cancel()
	bal, err := NewEthereum(Mainnet, "").Balance(ctx, "0x0000000000000000000000000000000000000000")
	if err != nil { t.Fatalf("Balance: %v", err) }
	if bal.Sign() <= 0 { t.Fatalf("want positive, got %s", bal.String()) }
	t.Logf("ETH null-address = %s", bal.String())
}

func TestBitcoinBalanceLive(t *testing.T) {
	ctx, cancel := liveCtx(t); defer cancel()
	// Genesis coinbase address — always funded.
	bal, err := NewBitcoin(Mainnet, "").Balance(ctx, "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa")
	if err != nil { t.Fatalf("Balance: %v", err) }
	if bal.Sign() <= 0 { t.Fatalf("want positive, got %s", bal.String()) }
	t.Logf("BTC genesis = %s", bal.String())
}

func TestSolanaBalanceLive(t *testing.T) {
	ctx, cancel := liveCtx(t); defer cancel()
	// Wrapped-SOL mint account exists and holds rent lamports.
	bal, err := NewSolana(Mainnet, "").Balance(ctx, "So11111111111111111111111111111111111111112")
	if err != nil { t.Fatalf("Balance: %v", err) }
	if bal.Sign() <= 0 { t.Fatalf("want positive, got %s", bal.String()) }
	t.Logf("SOL wrapped-mint = %s", bal.String())
}

func TestXRPBalanceLive(t *testing.T) {
	ctx, cancel := liveCtx(t); defer cancel()
	// Genesis account, always funded.
	bal, err := NewXRP(Mainnet, "").Balance(ctx, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")
	if err != nil { t.Fatalf("Balance: %v", err) }
	if bal.Sign() <= 0 { t.Fatalf("want positive, got %s", bal.String()) }
	t.Logf("XRP genesis = %s", bal.String())
}
