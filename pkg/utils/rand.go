package utils

import (
	"crypto/rand"
	"encoding/binary"
	"math"
	"math/big"
	"time"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func RandomRead(p []byte) (n int, err error) {
	return rand.Read(p)
}

func RandomIntn(n int) int {
	result, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic(err)
	}

	return int(result.Int64())
}

func RandomInt31n(n int32) int32 {
	result, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic(err)
	}

	return int32(result.Int64())
}

func RandomInt63n(n int64) int64 {
	result, err := rand.Int(rand.Reader, big.NewInt(n))
	if err != nil {
		panic(err)
	}

	return result.Int64()
}

// RandBytes returns length amount random bytes.
func RandBytes(length int) []byte {
	var b []byte
	for i := 0; i < length; i++ {
		b = append(b, byte(RandomIntn(256)))
	}

	return b
}

func RandString(length int) string {
	return string(RandBytes(length))
}

// RandUint16 returns a random uint16.
func RandUint16(max uint16) uint16 {
	return uint16(RandomInt31n(int32(max)))
}

// RandUint32 returns a random uint32.
func RandUint32(max uint32) uint32 {
	return uint32(RandomInt63n(int64(max)))
}

// RandUint64 returns a random uint64.
func RandUint64(max uint64) uint64 {
	return uint64(RandomInt63n(int64(uint32(max))))
}

func RandOutputID(index ...uint16) iotago.OutputID {
	idx := RandUint16(126)
	if len(index) > 0 {
		idx = index[0]
	}

	var outputID iotago.OutputID
	_, err := RandomRead(outputID[:iotago.TransactionIDLength])
	if err != nil {
		panic(err)
	}

	binary.LittleEndian.PutUint16(outputID[iotago.TransactionIDLength:], idx)

	return outputID
}

func RandBlockID() iotago.BlockID {
	blockID := iotago.BlockID{}
	copy(blockID[:], RandBytes(iotago.BlockIDLength))

	return blockID
}

func RandTransactionID() iotago.TransactionID {
	transactionID := iotago.TransactionID{}
	copy(transactionID[:], RandBytes(iotago.TransactionIDLength))

	return transactionID
}

func RandNFTID() iotago.NFTID {
	nft := iotago.NFTID{}
	copy(nft[:], RandBytes(iotago.NFTIDLength))

	return nft
}

func RandAccountID() iotago.AccountID {
	alias := iotago.AccountID{}
	copy(alias[:], RandBytes(iotago.AccountIDLength))

	return alias
}

func RandSlotIndex() iotago.SlotIndex {
	return iotago.SlotIndex(RandUint64(math.MaxUint64))
}

func RandTimestamp() time.Time {
	return time.Unix(int64(RandUint32(math.MaxUint32)), 0)
}

func RandAddress(addressType iotago.AddressType) iotago.Address {
	switch addressType {
	case iotago.AddressEd25519:
		address := &iotago.Ed25519Address{}
		addressBytes := RandBytes(32)
		copy(address[:], addressBytes)

		return address

	case iotago.AddressNFT:
		return RandNFTID().ToAddress()

	case iotago.AddressAccount:
		return RandAccountID().ToAddress()

	default:
		panic("unknown address type")
	}
}

func RandOutputType() iotago.OutputType {
	return iotago.OutputType(byte(RandomIntn(3) + 3))
}

func RandOutput(outputType iotago.OutputType) iotago.Output {
	var addr iotago.Address
	if outputType == iotago.OutputFoundry {
		addr = RandAddress(iotago.AddressAccount)
	} else {
		addr = RandAddress(iotago.AddressEd25519)
	}

	return RandOutputOnAddress(outputType, addr)
}

func RandOutputOnAddress(outputType iotago.OutputType, address iotago.Address) iotago.Output {
	return RandOutputOnAddressWithAmount(outputType, address, tpkg.RandBaseToken(math.MaxUint64))
}

func RandOutputOnAddressWithAmount(outputType iotago.OutputType, address iotago.Address, amount iotago.BaseToken) iotago.Output {
	var iotaOutput iotago.Output

	switch outputType {
	case iotago.OutputBasic:
		//nolint:forcetypeassert // we already checked the type
		iotaOutput = &iotago.BasicOutput{
			Amount:       amount,
			NativeTokens: iotago.NativeTokens{},
			Conditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{
					Address: address,
				},
			},
			Features: iotago.BasicOutputFeatures{},
		}
	case iotago.OutputAccount:
		//nolint:forcetypeassert // we already checked the type
		iotaOutput = &iotago.AccountOutput{
			Amount:       amount,
			NativeTokens: iotago.NativeTokens{},
			AccountID:    RandAccountID(),
			Conditions: iotago.AccountOutputUnlockConditions{
				&iotago.StateControllerAddressUnlockCondition{
					Address: address,
				},
				&iotago.GovernorAddressUnlockCondition{
					Address: address,
				},
			},
			Features:          iotago.AccountOutputFeatures{},
			ImmutableFeatures: iotago.AccountOutputImmFeatures{},
		}
	case iotago.OutputFoundry:
		if address.Type() != iotago.AddressAccount {
			panic("not an alias address")
		}
		supply := new(big.Int).SetUint64(tpkg.RandUint64(math.MaxUint64))

		//nolint:forcetypeassert // we already checked the type
		iotaOutput = &iotago.FoundryOutput{
			Amount:       amount,
			NativeTokens: iotago.NativeTokens{},
			SerialNumber: 0,
			TokenScheme: &iotago.SimpleTokenScheme{
				MintedTokens:  supply,
				MeltedTokens:  new(big.Int).SetBytes([]byte{0}),
				MaximumSupply: supply,
			},
			Conditions: iotago.FoundryOutputUnlockConditions{
				&iotago.ImmutableAccountUnlockCondition{
					Address: address.(*iotago.AccountAddress),
				},
			},
			Features:          iotago.FoundryOutputFeatures{},
			ImmutableFeatures: iotago.FoundryOutputImmFeatures{},
		}
	case iotago.OutputNFT:
		//nolint:forcetypeassert // we already checked the type
		iotaOutput = &iotago.NFTOutput{
			Amount:       amount,
			NativeTokens: iotago.NativeTokens{},
			NFTID:        RandNFTID(),
			Conditions: iotago.NFTOutputUnlockConditions{
				&iotago.AddressUnlockCondition{
					Address: address,
				},
			},
			Features:          iotago.NFTOutputFeatures{},
			ImmutableFeatures: iotago.NFTOutputImmFeatures{},
		}
	default:
		panic("unhandled output type")
	}

	return iotaOutput
}

func RandBlockIssuerKey() iotago.BlockIssuerKey {
	return iotago.BlockIssuerKeyEd25519FromPublicKey(ed25519.PublicKey(RandBytes(32)))
}

func RandBlockIssuerKeys() iotago.BlockIssuerKeys {
	// We always generate at least one key.
	length := RandomIntn(10) + 1
	blockIssuerKeys := make(iotago.BlockIssuerKeys, length)
	for i := 0; i < length; i++ {
		blockIssuerKeys[i] = RandBlockIssuerKey()
	}

	return blockIssuerKeys
}
