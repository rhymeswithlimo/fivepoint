package chains

import (
	"context"
	"math/big"

	"github.com/btcsuite/btcd/btcutil/base58"
	solana "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rhymeswithlimo/fivepoint/internal/money"
)

// Solana mainnet-beta and devnet (test network) endpoints.
const (
	mainnetSolanaRPC = "https://api.mainnet-beta.solana.com"
	devnetSolanaRPC  = "https://api.devnet.solana.com"
	// solBaseFee is the per-signature base fee in lamports for a simple transfer.
	solBaseFee = 5000
)

// Solana derives ed25519 (SLIP-0010) addresses, reads SOL balances, and builds
// SOL transfers.
type Solana struct {
	net Network
	sol *rpc.Client
}

// NewSolana returns the Solana chain for the network. An empty rpcURL uses the
// default endpoint for that network (mainnet-beta or devnet).
func NewSolana(net Network, rpcURL string) *Solana {
	if rpcURL == "" {
		if net == Testnet {
			rpcURL = devnetSolanaRPC
		} else {
			rpcURL = mainnetSolanaRPC
		}
	}
	return &Solana{net: net, sol: rpc.New(rpcURL)}
}

func (s *Solana) Symbol() string   { return "SOL" }
func (s *Solana) Name() string     { return "Solana" }
func (s *Solana) Decimals() int    { return 9 }
func (s *Solana) Network() Network { return s.net }

// DeriveAddress derives the base58 address at m/44'/501'/index'/0' (Phantom's
// account scheme). Solana addresses are identical on mainnet and devnet.
func (s *Solana) DeriveAddress(seed []byte, index uint32) (string, error) {
	_, pub, err := deriveEd25519(seed, []uint32{hard + 44, hard + 501, hard + index, hard + 0})
	if err != nil {
		return "", err
	}
	return base58.Encode(pub), nil
}

// Balance returns the native SOL balance (in lamports, 9 decimals).
func (s *Solana) Balance(ctx context.Context, address string) (money.Amount, error) {
	pub, err := solana.PublicKeyFromBase58(address)
	if err != nil {
		return money.Amount{}, err
	}
	res, err := s.sol.GetBalance(ctx, pub, rpc.CommitmentFinalized)
	if err != nil {
		return money.Amount{}, err
	}
	return money.New(new(big.Int).SetUint64(res.Value), s.Decimals()), nil
}

// Prepare builds a SOL transfer. The transaction (including its recent
// blockhash) is built now; signing and broadcasting happen in Send, which signs
// this exact transaction.
func (s *Solana) Prepare(ctx context.Context, seed []byte, to string, amount money.Amount) (*PreparedTx, error) {
	priv, pub, err := deriveEd25519(seed, []uint32{hard + 44, hard + 501, hard + 0, hard + 0})
	if err != nil {
		return nil, err
	}
	from := solana.PublicKeyFromBytes(pub)
	toPub, err := solana.PublicKeyFromBase58(to)
	if err != nil {
		return nil, err
	}

	recent, err := s.sol.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return nil, err
	}

	ix := system.NewTransferInstruction(amount.Raw().Uint64(), from, toPub).Build()
	tx, err := solana.NewTransaction([]solana.Instruction{ix}, recent.Value.Blockhash, solana.TransactionPayer(from))
	if err != nil {
		return nil, err
	}

	solPriv := solana.PrivateKey(priv)
	send := func(ctx context.Context) (string, error) {
		if _, err := tx.Sign(func(k solana.PublicKey) *solana.PrivateKey {
			if k.Equals(from) {
				return &solPriv
			}
			return nil
		}); err != nil {
			return "", err
		}
		sig, err := s.sol.SendTransaction(ctx, tx)
		if err != nil {
			return "", err
		}
		return sig.String(), nil
	}

	return &PreparedTx{
		Chain:  s.Symbol(),
		From:   from.String(),
		To:     to,
		Amount: amount,
		Fee:    money.New(big.NewInt(solBaseFee), s.Decimals()),
		send:   send,
	}, nil
}
