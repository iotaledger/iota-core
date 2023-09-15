//nolint:forcetypeassert,varnamelen,revive,exhaustruct // we don't care about these linters in test cases
package utxoledger_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/byteutils"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger/tpkg"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	iotago_tpkg "github.com/iotaledger/iota.go/v4/tpkg"
)

func AssertOutputUnspentAndSpentTransitions(t *testing.T, output *utxoledger.Output, spent *utxoledger.Spent) {
	outputID := output.OutputID()
	manager := utxoledger.New(mapdb.NewMapDB(), api.SingleVersionProvider(iotago_tpkg.TestAPI))

	require.NoError(t, manager.AddGenesisUnspentOutput(output))

	// Read Output from DB and compare
	readOutput, err := manager.ReadOutputByOutputID(outputID)
	require.NoError(t, err)
	tpkg.EqualOutput(t, output, readOutput)

	// Verify that it is unspent
	unspent, err := manager.IsOutputIDUnspentWithoutLocking(outputID)
	require.NoError(t, err)
	require.True(t, unspent)

	// Verify that all lookup keys exist in the database
	has, err := manager.KVStore().Has(output.UnspentLookupKey())
	require.NoError(t, err)
	require.True(t, has)

	// Spent it with a slot.
	require.NoError(t, manager.ApplyDiff(spent.SlotIndexSpent(), utxoledger.Outputs{}, utxoledger.Spents{spent}))

	// Read Spent from DB and compare
	readSpent, err := manager.ReadSpentForOutputIDWithoutLocking(outputID)
	require.NoError(t, err)
	tpkg.EqualSpent(t, spent, readSpent)

	// Verify that it is spent
	unspent, err = manager.IsOutputIDUnspentWithoutLocking(outputID)
	require.NoError(t, err)
	require.False(t, unspent)

	// Verify that no lookup keys exist in the database
	has, err = manager.KVStore().Has(output.UnspentLookupKey())
	require.NoError(t, err)
	require.False(t, has)

	// Rollback milestone
	require.NoError(t, manager.RollbackDiff(spent.SlotIndexSpent(), utxoledger.Outputs{}, utxoledger.Spents{spent}))

	// Verify that it is unspent
	unspent, err = manager.IsOutputIDUnspentWithoutLocking(outputID)
	require.NoError(t, err)
	require.True(t, unspent)

	// No Spent should be in the DB
	_, err = manager.ReadSpentForOutputIDWithoutLocking(outputID)
	require.ErrorIs(t, err, kvstore.ErrKeyNotFound)

	// Verify that all unspent keys exist in the database
	has, err = manager.KVStore().Has(output.UnspentLookupKey())
	require.NoError(t, err)
	require.True(t, has)
}

func CreateOutputAndAssertSerialization(t *testing.T, blockID iotago.BlockID, indexBooked iotago.SlotIndex, slotCreated iotago.SlotIndex, outputID iotago.OutputID, iotaOutput iotago.Output) *utxoledger.Output {
	iotagoAPI := iotago_tpkg.TestAPI
	output := utxoledger.CreateOutput(api.SingleVersionProvider(iotagoAPI), outputID, blockID, indexBooked, slotCreated, iotaOutput)
	outputBytes, err := iotagoAPI.Encode(output.Output())
	require.NoError(t, err)

	require.Equal(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutput}, outputID[:]), output.KVStorableKey())

	value := output.KVStorableValue()
	require.Equal(t, blockID[:], value[:40])
	require.Equal(t, indexBooked, lo.PanicOnErr(lo.DropCount(iotago.SlotIndexFromBytes(value[40:48]))))
	require.Equal(t, slotCreated, lo.PanicOnErr(lo.DropCount(iotago.SlotIndexFromBytes(value[48:56]))))
	require.Equal(t, outputBytes, value[56:])

	return output
}

func CreateSpentAndAssertSerialization(t *testing.T, output *utxoledger.Output) *utxoledger.Spent {
	transactionID := utils.RandTransactionID()

	indexSpent := iotago.SlotIndex(6788362)

	spent := utxoledger.NewSpent(output, transactionID, indexSpent)

	require.Equal(t, output, spent.Output())

	outputID := output.OutputID()
	require.Equal(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputSpent}, outputID[:]), spent.KVStorableKey())

	value := spent.KVStorableValue()
	require.Equal(t, transactionID[:], value[:32])
	require.Equal(t, indexSpent, lo.PanicOnErr(lo.DropCount(iotago.SlotIndexFromBytes(value[32:40]))))

	return spent
}

func TestExtendedOutputOnEd25519WithoutSpendConstraintsSerialization(t *testing.T) {
	outputID := utils.RandOutputID()
	blockID := utils.RandBlockID()
	address := utils.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	senderAddress := utils.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	tag := utils.RandBytes(23)
	amount := iotago_tpkg.RandBaseToken(math.MaxUint64)
	index := utils.RandSlotIndex()
	slotCreated := utils.RandSlotIndex()

	iotaOutput := &iotago.BasicOutput{
		Amount:       amount,
		NativeTokens: iotago.NativeTokens{},
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{
				Address: address,
			},
		},
		Features: iotago.BasicOutputFeatures{
			&iotago.SenderFeature{
				Address: senderAddress,
			},
			&iotago.TagFeature{
				Tag: tag,
			},
		},
	}

	output := CreateOutputAndAssertSerialization(t, blockID, index, slotCreated, outputID, iotaOutput)
	spent := CreateSpentAndAssertSerialization(t, output)

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}

func TestExtendedOutputOnEd25519WithSpendConstraintsSerialization(t *testing.T) {
	outputID := utils.RandOutputID()
	blockID := utils.RandBlockID()
	address := utils.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	senderAddress := utils.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	amount := iotago_tpkg.RandBaseToken(math.MaxUint64)
	index := utils.RandSlotIndex()
	slotCreated := utils.RandSlotIndex()
	timeLockUnlockSlot := utils.RandSlotIndex()

	iotaOutput := &iotago.BasicOutput{
		Amount:       amount,
		NativeTokens: iotago.NativeTokens{},
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{
				Address: address,
			},
			&iotago.TimelockUnlockCondition{
				SlotIndex: timeLockUnlockSlot,
			},
		},
		Features: iotago.BasicOutputFeatures{
			&iotago.SenderFeature{
				Address: senderAddress,
			},
		},
	}

	output := CreateOutputAndAssertSerialization(t, blockID, index, slotCreated, outputID, iotaOutput)
	spent := CreateSpentAndAssertSerialization(t, output)

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}

func TestNFTOutputSerialization(t *testing.T) {
	outputID := utils.RandOutputID()
	blockID := utils.RandBlockID()
	address := utils.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	nftID := utils.RandNFTID()
	amount := iotago_tpkg.RandBaseToken(math.MaxUint64)
	index := utils.RandSlotIndex()
	slotCreated := utils.RandSlotIndex()

	iotaOutput := &iotago.NFTOutput{
		Amount:       amount,
		NativeTokens: iotago.NativeTokens{},
		NFTID:        nftID,
		Conditions: iotago.NFTOutputUnlockConditions{
			&iotago.AddressUnlockCondition{
				Address: address,
			},
		},
		Features: iotago.NFTOutputFeatures{},
		ImmutableFeatures: iotago.NFTOutputImmFeatures{
			&iotago.MetadataFeature{
				Data: utils.RandBytes(12),
			},
		},
	}

	output := CreateOutputAndAssertSerialization(t, blockID, index, slotCreated, outputID, iotaOutput)
	spent := CreateSpentAndAssertSerialization(t, output)

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}

func TestNFTOutputWithSpendConstraintsSerialization(t *testing.T) {
	outputID := utils.RandOutputID()
	blockID := utils.RandBlockID()
	address := utils.RandNFTID()
	issuerAddress := utils.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	nftID := utils.RandNFTID()
	amount := iotago_tpkg.RandBaseToken(math.MaxUint64)
	index := utils.RandSlotIndex()
	slotCreated := utils.RandSlotIndex()
	expirationUnlockSlot := utils.RandSlotIndex()

	iotaOutput := &iotago.NFTOutput{
		Amount:       amount,
		NativeTokens: iotago.NativeTokens{},
		NFTID:        nftID,
		Conditions: iotago.NFTOutputUnlockConditions{
			&iotago.AddressUnlockCondition{
				Address: address.ToAddress(),
			},
			&iotago.ExpirationUnlockCondition{
				SlotIndex:     expirationUnlockSlot,
				ReturnAddress: issuerAddress,
			},
		},
		Features: iotago.NFTOutputFeatures{},
		ImmutableFeatures: iotago.NFTOutputImmFeatures{
			&iotago.MetadataFeature{
				Data: utils.RandBytes(12),
			},
			&iotago.IssuerFeature{
				Address: issuerAddress,
			},
		},
	}

	output := CreateOutputAndAssertSerialization(t, blockID, index, slotCreated, outputID, iotaOutput)
	spent := CreateSpentAndAssertSerialization(t, output)

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}

func TestAccountOutputSerialization(t *testing.T) {
	outputID := utils.RandOutputID()
	blockID := utils.RandBlockID()
	aliasID := utils.RandAccountID()
	stateController := utils.RandAccountID()
	governor := utils.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	issuer := utils.RandNFTID()
	sender := utils.RandAccountID()
	amount := iotago_tpkg.RandBaseToken(math.MaxUint64)
	index := utils.RandSlotIndex()
	slotCreated := utils.RandSlotIndex()

	iotaOutput := &iotago.AccountOutput{
		Amount:       amount,
		NativeTokens: iotago.NativeTokens{},
		AccountID:    aliasID,
		Conditions: iotago.AccountOutputUnlockConditions{
			&iotago.StateControllerAddressUnlockCondition{
				Address: stateController.ToAddress(),
			},
			&iotago.GovernorAddressUnlockCondition{
				Address: governor,
			},
		},
		Features: iotago.AccountOutputFeatures{
			&iotago.SenderFeature{
				Address: sender.ToAddress(),
			},
		},
		ImmutableFeatures: iotago.AccountOutputImmFeatures{
			&iotago.IssuerFeature{
				Address: issuer.ToAddress(),
			},
		},
	}

	output := CreateOutputAndAssertSerialization(t, blockID, index, slotCreated, outputID, iotaOutput)
	spent := CreateSpentAndAssertSerialization(t, output)

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}

func TestFoundryOutputSerialization(t *testing.T) {
	outputID := utils.RandOutputID()
	blockID := utils.RandBlockID()
	aliasID := utils.RandAccountID()
	amount := iotago_tpkg.RandBaseToken(math.MaxUint64)
	index := utils.RandSlotIndex()
	slotCreated := utils.RandSlotIndex()
	supply := new(big.Int).SetUint64(iotago_tpkg.RandUint64(math.MaxUint64))

	iotaOutput := &iotago.FoundryOutput{
		Amount:       amount,
		NativeTokens: iotago.NativeTokens{},
		SerialNumber: utils.RandUint32(math.MaxUint32),
		TokenScheme: &iotago.SimpleTokenScheme{
			MintedTokens:  supply,
			MeltedTokens:  new(big.Int).SetBytes([]byte{0}),
			MaximumSupply: supply,
		},
		Conditions: iotago.FoundryOutputUnlockConditions{
			&iotago.ImmutableAccountUnlockCondition{
				Address: aliasID.ToAddress().(*iotago.AccountAddress),
			},
		},
		Features:          iotago.FoundryOutputFeatures{},
		ImmutableFeatures: iotago.FoundryOutputImmFeatures{},
	}

	output := CreateOutputAndAssertSerialization(t, blockID, index, slotCreated, outputID, iotaOutput)
	spent := CreateSpentAndAssertSerialization(t, output)

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}
