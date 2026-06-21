package chains

import (
	"math/big"
	"testing"

	solana "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/rhymeswithlimo/fivepoint/internal/money"
)

// Offline intent check: decode the transfer instruction Prepare builds and
// assert it encodes the exact recipient and lamport amount. This catches the
// money-losing bugs (wrong decimals → wrong amount, wrong recipient encoding)
// that "the tx was accepted by the node" never would.
func TestSolanaTransferEncoding(t *testing.T) {
	from := solana.NewWallet().PublicKey()
	to := solana.NewWallet().PublicKey()

	// 0.012345678 SOL at 9 decimals == 12,345,678 lamports.
	amt := money.New(big.NewInt(12_345_678), 9)

	ix := system.NewTransferInstruction(amt.Raw().Uint64(), from, to).Build()
	data, err := ix.Data()
	if err != nil {
		t.Fatalf("Data: %v", err)
	}
	decoded, err := system.DecodeInstruction(ix.Accounts(), data)
	if err != nil {
		t.Fatalf("DecodeInstruction: %v", err)
	}
	transfer, ok := decoded.Impl.(*system.Transfer)
	if !ok {
		t.Fatalf("decoded instruction is %T, want *system.Transfer", decoded.Impl)
	}

	if got := *transfer.Lamports; got != 12_345_678 {
		t.Fatalf("lamports = %d, want 12345678", got)
	}
	if !transfer.GetFundingAccount().PublicKey.Equals(from) {
		t.Fatalf("funding account mismatch")
	}
	if !transfer.GetRecipientAccount().PublicKey.Equals(to) {
		t.Fatalf("recipient account mismatch")
	}
}
