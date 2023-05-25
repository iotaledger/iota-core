package tpkg

import (
	"crypto/rand"
	"encoding/binary"
	"math"
	"math/big"
	"time"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate"
	iotago "github.com/iotaledger/iota.go/v4"
)

var (
	protocolParams = &iotago.ProtocolParameters{
		Version:     3,
		NetworkName: RandString(255),
		Bech32HRP:   iotago.NetworkPrefix(RandString(3)),
		MinPoWScore: RandUint32(50000),
		RentStructure: iotago.RentStructure{
			VByteCost:    100,
			VBFactorData: 1,
			VBFactorKey:  10,
		},
		TokenSupply:           RandAmount(),
		GenesisUnixTimestamp:  uint32(time.Now().Unix()),
		SlotDurationInSeconds: 10,
	}
	api = iotago.LatestAPI(protocolParams)
)

func ProtocolParams() *iotago.ProtocolParameters {
	return protocolParams
}

func API() iotago.API {
	return api
}

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

func RandAmount() uint64 {
	return RandUint64(math.MaxUint64)
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
	return RandOutputOnAddressWithAmount(outputType, address, RandAmount())
}

func RandOutputOnAddressWithAmount(outputType iotago.OutputType, address iotago.Address, amount uint64) iotago.Output {
	var iotaOutput iotago.Output

	switch outputType {
	case iotago.OutputBasic:
		//nolint:forcetypeassert // we already checked the type
		iotaOutput = &iotago.BasicOutput{
			Amount: amount,
			Conditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{
					Address: address,
				},
			},
		}
	case iotago.OutputAccount:
		//nolint:forcetypeassert // we already checked the type
		iotaOutput = &iotago.AccountOutput{
			Amount:    amount,
			AccountID: RandAccountID(),
			Conditions: iotago.AccountOutputUnlockConditions{
				&iotago.StateControllerAddressUnlockCondition{
					Address: address,
				},
				&iotago.GovernorAddressUnlockCondition{
					Address: address,
				},
			},
		}
	case iotago.OutputFoundry:
		if address.Type() != iotago.AddressAccount {
			panic("not an alias address")
		}
		supply := new(big.Int).SetUint64(RandAmount())

		//nolint:forcetypeassert // we already checked the type
		iotaOutput = &iotago.FoundryOutput{
			Amount:       amount,
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
		}
	case iotago.OutputNFT:
		//nolint:forcetypeassert // we already checked the type
		iotaOutput = &iotago.NFTOutput{
			Amount: amount,
			NFTID:  RandNFTID(),
			Conditions: iotago.NFTOutputUnlockConditions{
				&iotago.AddressUnlockCondition{
					Address: address,
				},
			},
		}
	default:
		panic("unhandled output type")
	}

	return iotaOutput
}

func RandLedgerStateOutput() *ledgerstate.Output {
	return RandLedgerStateOutputWithType(RandOutputType())
}

func RandLedgerStateOutputWithType(outputType iotago.OutputType) *ledgerstate.Output {
	return ledgerstate.CreateOutput(api, RandOutputID(), RandBlockID(), RandSlotIndex(), RandSlotIndex(), RandOutput(outputType))
}

func RandLedgerStateOutputOnAddress(outputType iotago.OutputType, address iotago.Address) *ledgerstate.Output {
	return ledgerstate.CreateOutput(api, RandOutputID(), RandBlockID(), RandSlotIndex(), RandSlotIndex(), RandOutputOnAddress(outputType, address))
}

func RandLedgerStateOutputOnAddressWithAmount(outputType iotago.OutputType, address iotago.Address, amount uint64) *ledgerstate.Output {
	return ledgerstate.CreateOutput(api, RandOutputID(), RandBlockID(), RandSlotIndex(), RandSlotIndex(), RandOutputOnAddressWithAmount(outputType, address, amount))
}

func RandLedgerStateSpent(indexSpent iotago.SlotIndex, timestampSpent time.Time) *ledgerstate.Spent {
	return ledgerstate.NewSpent(RandLedgerStateOutput(), RandTransactionID(), indexSpent)
}

func RandLedgerStateSpentWithOutput(output *ledgerstate.Output, indexSpent iotago.SlotIndex, timestampSpent time.Time) *ledgerstate.Spent {
	return ledgerstate.NewSpent(output, RandTransactionID(), indexSpent)
}
