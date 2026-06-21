package chains

import (
	"math/big"
	"testing"

	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

// Offline signing proof for EVM: sign a transfer with the wallet's derived key,
// then recover the sender from the signature and assert it equals the derived
// address — proving the signature is valid AND over the right key — and assert
// the signed tx carries the intended recipient, value, and chainID (EIP-155
// replay protection). No network required; deterministic and authoritative.
func TestEVMSignRecover(t *testing.T) {
	seed := testSeed()
	e := NewEthereum(Testnet, "") // Sepolia chainID

	fromAddr, err := e.DeriveAddress(seed, 0)
	if err != nil {
		t.Fatalf("DeriveAddress: %v", err)
	}

	hk, err := deriveSecp256k1(seed, []uint32{hard + 44, hard + 60, hard + 0, 0, 0})
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	priv, _ := hk.ECPrivKey()
	ecdsaPriv, err := ethcrypto.ToECDSA(priv.Serialize())
	if err != nil {
		t.Fatalf("ToECDSA: %v", err)
	}

	to := ethcommon.HexToAddress("0x000000000000000000000000000000000000dEaD")
	value := big.NewInt(1_000_000_000_000_000) // 0.001 ETH in wei
	tx := ethtypes.NewTx(&ethtypes.LegacyTx{
		Nonce: 5, GasPrice: big.NewInt(20_000_000_000), Gas: gasLimitTransfer, To: &to, Value: value,
	})

	signer := ethtypes.LatestSignerForChainID(big.NewInt(e.chainID))
	signed, err := ethtypes.SignTx(tx, signer, ecdsaPriv)
	if err != nil {
		t.Fatalf("SignTx: %v", err)
	}

	recovered, err := ethtypes.Sender(signer, signed)
	if err != nil {
		t.Fatalf("Sender: %v", err)
	}
	if recovered.Hex() != fromAddr {
		t.Fatalf("recovered sender %s != derived address %s", recovered.Hex(), fromAddr)
	}
	if signed.To().Hex() != to.Hex() {
		t.Fatalf("to = %s, want %s", signed.To().Hex(), to.Hex())
	}
	if signed.Value().Cmp(value) != 0 {
		t.Fatalf("value = %s, want %s", signed.Value(), value)
	}
	if signed.ChainId().Int64() != sepoliaChainID {
		t.Fatalf("chainID = %d, want %d", signed.ChainId().Int64(), sepoliaChainID)
	}
}
