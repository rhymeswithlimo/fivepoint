package chains

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/binary"
	"errors"
)

// slip10.go implements SLIP-0010 HD key derivation over the ed25519 curve, used
// by Solana. ed25519 supports only hardened derivation. Verified against the
// official SLIP-0010 ed25519 test vectors (see slip10_test.go).

type slip10Key struct {
	key   []byte // 32-byte private key (IL)
	chain []byte // 32-byte chain code (IR)
}

func slip10Master(seed []byte) slip10Key {
	mac := hmac.New(sha512.New, []byte("ed25519 seed"))
	mac.Write(seed)
	sum := mac.Sum(nil)
	return slip10Key{key: sum[:32], chain: sum[32:]}
}

func (k slip10Key) child(index uint32) (slip10Key, error) {
	if index < hard {
		return slip10Key{}, errors.New("slip10: ed25519 requires hardened derivation")
	}
	var data [37]byte
	data[0] = 0x00
	copy(data[1:33], k.key)
	binary.BigEndian.PutUint32(data[33:], index)

	mac := hmac.New(sha512.New, k.chain)
	mac.Write(data[:])
	sum := mac.Sum(nil)
	return slip10Key{key: sum[:32], chain: sum[32:]}, nil
}

// deriveEd25519 walks a fully-hardened path and returns the ed25519 key pair.
func deriveEd25519(seed []byte, path []uint32) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	k := slip10Master(seed)
	for _, index := range path {
		var err error
		if k, err = k.child(index); err != nil {
			return nil, nil, err
		}
	}
	priv := ed25519.NewKeyFromSeed(k.key)
	return priv, priv.Public().(ed25519.PublicKey), nil
}
