package chains

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/rhymeswithlimo/fivepoint/internal/money"
)

// gasLimitTransfer is the gas a plain ETH transfer uses (no contract).
const gasLimitTransfer = 21000

// Public, keyless Ethereum endpoints (mainnet and the Sepolia test network).
const (
	mainnetEthRPC = "https://ethereum-rpc.publicnode.com"
	sepoliaEthRPC = "https://ethereum-sepolia-rpc.publicnode.com"
	mainnetChainID = 1
	sepoliaChainID = 11155111
)

// EVM is an account-based EVM chain (Ethereum and EVM-compatible networks).
// EVM addresses are identical across all EVM chains, so one implementation
// serves Ethereum, Polygon, Arbitrum, etc., parameterised by RPC endpoint.
type EVM struct {
	name    string
	symbol  string
	net     Network
	chainID int64
	rpc     *rpcClient
}

// NewEthereum returns the Ethereum chain for the network (mainnet or Sepolia).
// An empty rpcURL uses the default public endpoint for that network.
func NewEthereum(net Network, rpcURL string) *EVM {
	chainID := int64(mainnetChainID)
	if net == Testnet {
		chainID = sepoliaChainID
		if rpcURL == "" {
			rpcURL = sepoliaEthRPC
		}
	} else if rpcURL == "" {
		rpcURL = mainnetEthRPC
	}
	return &EVM{name: "Ethereum", symbol: "ETH", net: net, chainID: chainID, rpc: newRPC(rpcURL)}
}

func (e *EVM) Symbol() string   { return e.symbol }
func (e *EVM) Name() string     { return e.name }
func (e *EVM) Decimals() int    { return 18 }
func (e *EVM) Network() Network { return e.net }

// DeriveAddress derives the EIP-55 checksummed address at m/44'/60'/0'/0/index
// (BIP-44, coin type 60) — the path MetaMask, Ledger Live, and most EVM wallets
// use, so an imported seed yields the same address the user already knows.
func (e *EVM) DeriveAddress(seed []byte, index uint32) (string, error) {
	key, err := deriveSecp256k1(seed, []uint32{hard + 44, hard + 60, hard + 0, 0, index})
	if err != nil {
		return "", err
	}
	priv, err := key.ECPrivKey()
	if err != nil {
		return "", err
	}
	ecdsaPriv, err := ethcrypto.ToECDSA(priv.Serialize())
	if err != nil {
		return "", err
	}
	return ethcrypto.PubkeyToAddress(ecdsaPriv.PublicKey).Hex(), nil
}

// Balance returns the native ETH balance (in wei, 18 decimals).
func (e *EVM) Balance(ctx context.Context, address string) (money.Amount, error) {
	var hexBal string
	if err := e.rpc.call(ctx, "eth_getBalance", []any{address, "latest"}, &hexBal); err != nil {
		return money.Amount{}, err
	}
	wei, ok := new(big.Int).SetString(strings.TrimPrefix(hexBal, "0x"), 16)
	if !ok {
		return money.Amount{}, fmt.Errorf("evm: bad balance hex %q", hexBal)
	}
	return money.New(wei, e.Decimals()), nil
}

// Prepare builds an ETH transfer. The tx is built and signed with EIP-155 replay
// protection (chainID); Send broadcasts that exact signed transaction.
func (e *EVM) Prepare(ctx context.Context, seed []byte, to string, amount money.Amount) (*PreparedTx, error) {
	if !ethcommon.IsHexAddress(to) {
		return nil, fmt.Errorf("evm: invalid recipient address %q", to)
	}
	hk, err := deriveSecp256k1(seed, []uint32{hard + 44, hard + 60, hard + 0, 0, 0})
	if err != nil {
		return nil, err
	}
	priv, err := hk.ECPrivKey()
	if err != nil {
		return nil, err
	}
	ecdsaPriv, err := ethcrypto.ToECDSA(priv.Serialize())
	if err != nil {
		return nil, err
	}
	from := ethcrypto.PubkeyToAddress(ecdsaPriv.PublicKey)
	toAddr := ethcommon.HexToAddress(to)

	nonce, err := e.nonce(ctx, from.Hex())
	if err != nil {
		return nil, err
	}
	gasPrice, err := e.gasPrice(ctx)
	if err != nil {
		return nil, err
	}

	tx := ethtypes.NewTx(&ethtypes.LegacyTx{
		Nonce:    nonce,
		GasPrice: gasPrice,
		Gas:      gasLimitTransfer,
		To:       &toAddr,
		Value:    amount.Raw(),
	})
	signer := ethtypes.LatestSignerForChainID(big.NewInt(e.chainID))
	signed, err := ethtypes.SignTx(tx, signer, ecdsaPriv)
	if err != nil {
		return nil, err
	}

	fee := new(big.Int).Mul(gasPrice, big.NewInt(gasLimitTransfer))
	send := func(ctx context.Context) (string, error) {
		raw, err := signed.MarshalBinary()
		if err != nil {
			return "", err
		}
		var txHash string
		if err := e.rpc.call(ctx, "eth_sendRawTransaction", []any{"0x" + fmt.Sprintf("%x", raw)}, &txHash); err != nil {
			return "", err
		}
		return txHash, nil
	}

	return &PreparedTx{
		Chain:  e.symbol,
		From:   from.Hex(),
		To:     to,
		Amount: amount,
		Fee:    money.New(fee, e.Decimals()),
		send:   send,
	}, nil
}

func (e *EVM) nonce(ctx context.Context, addr string) (uint64, error) {
	var hexNonce string
	if err := e.rpc.call(ctx, "eth_getTransactionCount", []any{addr, "pending"}, &hexNonce); err != nil {
		return 0, err
	}
	n, ok := new(big.Int).SetString(strings.TrimPrefix(hexNonce, "0x"), 16)
	if !ok {
		return 0, fmt.Errorf("evm: bad nonce %q", hexNonce)
	}
	return n.Uint64(), nil
}

func (e *EVM) gasPrice(ctx context.Context) (*big.Int, error) {
	var hexPrice string
	if err := e.rpc.call(ctx, "eth_gasPrice", []any{}, &hexPrice); err != nil {
		return nil, err
	}
	p, ok := new(big.Int).SetString(strings.TrimPrefix(hexPrice, "0x"), 16)
	if !ok {
		return nil, fmt.Errorf("evm: bad gas price %q", hexPrice)
	}
	return p, nil
}
