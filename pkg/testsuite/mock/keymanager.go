package mock

import (
	"crypto/ed25519"
	"fmt"

	"github.com/wollac/iota-crypto-demo/pkg/bip32path"
	"github.com/wollac/iota-crypto-demo/pkg/slip10"
	"github.com/wollac/iota-crypto-demo/pkg/slip10/eddsa"

	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
)

// KeyManager is a hierarchical deterministic key manager.
type KeyManager struct {
	seed  []byte
	index uint64
}

func NewKeyManager(seed []byte, index uint64) *KeyManager {
	return &KeyManager{
		seed:  seed,
		index: index,
	}
}

// KeyPair calculates an ed25519 key pair by using slip10.
func (k *KeyManager) KeyPair() (ed25519.PrivateKey, ed25519.PublicKey) {
	path, err := bip32path.ParsePath(fmt.Sprintf(pathString, k.index))
	if err != nil {
		panic(err)
	}

	curve := eddsa.Ed25519()
	key, err := slip10.DeriveKeyFromPath(k.seed, curve, path)
	if err != nil {
		panic(err)
	}

	pubKey, privKey := key.Key.(eddsa.Seed).Ed25519Key()

	return ed25519.PrivateKey(privKey), ed25519.PublicKey(pubKey)
}

// AddressSigner returns an address signer.
func (k *KeyManager) AddressSigner() iotago.AddressSigner {
	privKey, pubKey := k.KeyPair()
	address := iotago.Ed25519AddressFromPubKey(pubKey)

	return iotago.NewInMemoryAddressSigner(iotago.NewAddressKeysForEd25519Address(address, privKey))
}

// Address calculates an ed25519 address by using slip10.
func (k *KeyManager) Address(addressType ...iotago.AddressType) iotago.DirectUnlockableAddress {
	_, pubKey := k.KeyPair()

	addrType := iotago.AddressEd25519
	if len(addressType) > 0 {
		addrType = addressType[0]
	}

	switch addrType {
	case iotago.AddressEd25519:
		return iotago.Ed25519AddressFromPubKey(pubKey)
	case iotago.AddressImplicitAccountCreation:
		return iotago.ImplicitAccountCreationAddressFromPubKey(pubKey)
	default:
		panic(ierrors.Wrapf(iotago.ErrUnknownAddrType, "type %d", addressType))
	}
}
