//nolint:forcetypeassert,varnamelen,revive,exhaustruct // we don't care about these linters in test cases
package utxoledger_test

import (
	"encoding/binary"
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
	require.NoError(t, manager.ApplyDiff(spent.SlotSpent(), utxoledger.Outputs{}, utxoledger.Spents{spent}))

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
	require.NoError(t, manager.RollbackDiff(spent.SlotSpent(), utxoledger.Outputs{}, utxoledger.Spents{spent}))

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

func CreateOutputAndAssertSerialization(t *testing.T, blockID iotago.BlockID, indexBooked iotago.SlotIndex, iotaOutput iotago.Output, outputProof *iotago.OutputIDProof) *utxoledger.Output {
	outputID, err := outputProof.OutputID(iotaOutput)
	require.NoError(t, err)

	iotagoAPI := iotago_tpkg.TestAPI
	output := utxoledger.CreateOutput(api.SingleVersionProvider(iotagoAPI), outputID, blockID, indexBooked, iotaOutput, outputProof)
	outputBytes, err := iotagoAPI.Encode(output.Output())
	require.NoError(t, err)
	proofBytes, err := outputProof.Bytes()
	require.NoError(t, err)

	require.Equal(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutput}, outputID[:]), output.KVStorableKey())

	value := output.KVStorableValue()
	readOffset := 0
	require.Equal(t, blockID[:], value[readOffset:readOffset+iotago.BlockIDLength])
	readOffset += iotago.BlockIDLength
	require.Equal(t, indexBooked, lo.PanicOnErr(lo.DropCount(iotago.SlotIndexFromBytes(value[readOffset:readOffset+iotago.SlotIndexLength]))))
	readOffset += iotago.SlotIndexLength
	require.Equal(t, uint32(len(outputBytes)), binary.LittleEndian.Uint32(value[readOffset:readOffset+4]), "output bytes length")
	readOffset += 4
	require.Equal(t, outputBytes, value[readOffset:readOffset+len(outputBytes)])
	readOffset += len(outputBytes)
	require.Equal(t, uint32(len(proofBytes)), binary.LittleEndian.Uint32(value[readOffset:readOffset+4]), "proof bytes length")
	readOffset += 4
	require.Equal(t, proofBytes, value[readOffset:readOffset+len(proofBytes)])

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
	require.Equal(t, transactionID[:], value[:iotago.TransactionIDLength])
	require.Equal(t, indexSpent, lo.PanicOnErr(lo.DropCount(iotago.SlotIndexFromBytes(value[iotago.TransactionIDLength:iotago.TransactionIDLength+iotago.SlotIndexLength]))))

	return spent
}

func TestBasicOutputOnEd25519WithoutSpendConstraintsSerialization(t *testing.T) {
	txCommitment := iotago_tpkg.Rand32ByteArray()
	txCreationSlot := utils.RandSlotIndex()
	blockID := utils.RandBlockID()
	address := utils.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	senderAddress := utils.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	tag := utils.RandBytes(23)
	amount := iotago_tpkg.RandBaseToken(iotago.MaxBaseToken)
	index := utils.RandSlotIndex()

	iotaOutput := &iotago.BasicOutput{
		Amount: amount,
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

	outputProof, err := iotago.NewOutputIDProof(iotago_tpkg.TestAPI, txCommitment, txCreationSlot, iotago.TxEssenceOutputs{iotaOutput}, 0)
	require.NoError(t, err)

	output := CreateOutputAndAssertSerialization(t, blockID, index, iotaOutput, outputProof)
	spent := CreateSpentAndAssertSerialization(t, output)
	outputID := output.OutputID()

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}

func TestBasicOutputOnEd25519WithSpendConstraintsSerialization(t *testing.T) {
	txCommitment := iotago_tpkg.Rand32ByteArray()
	txCreationSlot := utils.RandSlotIndex()
	blockID := utils.RandBlockID()
	address := utils.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	senderAddress := utils.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	amount := iotago_tpkg.RandBaseToken(iotago.MaxBaseToken)
	index := utils.RandSlotIndex()
	timeLockUnlockSlot := utils.RandSlotIndex()

	iotaOutput := &iotago.BasicOutput{
		Amount: amount,
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{
				Address: address,
			},
			&iotago.TimelockUnlockCondition{
				Slot: timeLockUnlockSlot,
			},
		},
		Features: iotago.BasicOutputFeatures{
			&iotago.SenderFeature{
				Address: senderAddress,
			},
		},
	}

	outputProof, err := iotago.NewOutputIDProof(iotago_tpkg.TestAPI, txCommitment, txCreationSlot, iotago.TxEssenceOutputs{iotaOutput}, 0)
	require.NoError(t, err)

	output := CreateOutputAndAssertSerialization(t, blockID, index, iotaOutput, outputProof)
	spent := CreateSpentAndAssertSerialization(t, output)

	outputID := output.OutputID()

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}

func TestNFTOutputSerialization(t *testing.T) {
	txCommitment := iotago_tpkg.Rand32ByteArray()
	txCreationSlot := utils.RandSlotIndex()
	blockID := utils.RandBlockID()
	address := utils.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	nftID := utils.RandNFTID()
	amount := iotago_tpkg.RandBaseToken(iotago.MaxBaseToken)
	index := utils.RandSlotIndex()

	iotaOutput := &iotago.NFTOutput{
		Amount: amount,
		NFTID:  nftID,
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

	outputProof, err := iotago.NewOutputIDProof(iotago_tpkg.TestAPI, txCommitment, txCreationSlot, iotago.TxEssenceOutputs{iotaOutput}, 0)
	require.NoError(t, err)

	output := CreateOutputAndAssertSerialization(t, blockID, index, iotaOutput, outputProof)
	spent := CreateSpentAndAssertSerialization(t, output)
	outputID := output.OutputID()

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}

func TestNFTOutputWithSpendConstraintsSerialization(t *testing.T) {
	txCommitment := iotago_tpkg.Rand32ByteArray()
	txCreationSlot := utils.RandSlotIndex()
	blockID := utils.RandBlockID()
	address := utils.RandNFTID()
	issuerAddress := utils.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	nftID := utils.RandNFTID()
	amount := iotago_tpkg.RandBaseToken(iotago.MaxBaseToken)
	index := utils.RandSlotIndex()
	expirationUnlockSlot := utils.RandSlotIndex()

	iotaOutput := &iotago.NFTOutput{
		Amount: amount,
		NFTID:  nftID,
		Conditions: iotago.NFTOutputUnlockConditions{
			&iotago.AddressUnlockCondition{
				Address: address.ToAddress(),
			},
			&iotago.ExpirationUnlockCondition{
				Slot:          expirationUnlockSlot,
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

	outputProof, err := iotago.NewOutputIDProof(iotago_tpkg.TestAPI, txCommitment, txCreationSlot, iotago.TxEssenceOutputs{iotaOutput}, 0)
	require.NoError(t, err)

	output := CreateOutputAndAssertSerialization(t, blockID, index, iotaOutput, outputProof)
	spent := CreateSpentAndAssertSerialization(t, output)
	outputID := output.OutputID()

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}

func TestAccountOutputSerialization(t *testing.T) {
	txCommitment := iotago_tpkg.Rand32ByteArray()
	txCreationSlot := utils.RandSlotIndex()
	blockID := utils.RandBlockID()
	aliasID := utils.RandAccountID()
	stateController := utils.RandAccountID()
	governor := utils.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	issuer := utils.RandNFTID()
	sender := utils.RandAccountID()
	amount := iotago_tpkg.RandBaseToken(iotago.MaxBaseToken)
	index := utils.RandSlotIndex()

	iotaOutput := &iotago.AccountOutput{
		Amount:    amount,
		AccountID: aliasID,
		Conditions: iotago.AccountOutputUnlockConditions{
			&iotago.StateControllerAddressUnlockCondition{
				Address: stateController.ToAddress(),
			},
			&iotago.GovernorAddressUnlockCondition{
				Address: governor,
			},
		},
		StateMetadata: make([]byte, 0),
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

	outputProof, err := iotago.NewOutputIDProof(iotago_tpkg.TestAPI, txCommitment, txCreationSlot, iotago.TxEssenceOutputs{iotaOutput}, 0)
	require.NoError(t, err)

	output := CreateOutputAndAssertSerialization(t, blockID, index, iotaOutput, outputProof)
	spent := CreateSpentAndAssertSerialization(t, output)
	outputID := output.OutputID()

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}

func TestFoundryOutputSerialization(t *testing.T) {
	txCommitment := iotago_tpkg.Rand32ByteArray()
	txCreationSlot := utils.RandSlotIndex()
	blockID := utils.RandBlockID()
	aliasID := utils.RandAccountID()
	amount := iotago_tpkg.RandBaseToken(iotago.MaxBaseToken)
	index := utils.RandSlotIndex()
	supply := new(big.Int).SetUint64(iotago_tpkg.RandUint64(math.MaxUint64))

	iotaOutput := &iotago.FoundryOutput{
		Amount:       amount,
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

	outputProof, err := iotago.NewOutputIDProof(iotago_tpkg.TestAPI, txCommitment, txCreationSlot, iotago.TxEssenceOutputs{iotaOutput}, 0)
	require.NoError(t, err)

	output := CreateOutputAndAssertSerialization(t, blockID, index, iotaOutput, outputProof)
	spent := CreateSpentAndAssertSerialization(t, output)
	outputID := output.OutputID()

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}

func TestDelegationOutputSerialization(t *testing.T) {
	txCommitment := iotago_tpkg.Rand32ByteArray()
	txCreationSlot := utils.RandSlotIndex()
	blockID := utils.RandBlockID()
	address := utils.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	amount := iotago_tpkg.RandBaseToken(iotago.MaxBaseToken)
	index := utils.RandSlotIndex()

	iotaOutput := &iotago.DelegationOutput{
		Amount:           amount,
		DelegatedAmount:  amount,
		DelegationID:     iotago_tpkg.RandDelegationID(),
		ValidatorAddress: utils.RandAddress(iotago.AddressAccount).(*iotago.AccountAddress),
		StartEpoch:       iotago_tpkg.RandEpoch(),
		Conditions: iotago.DelegationOutputUnlockConditions{
			&iotago.AddressUnlockCondition{
				Address: address,
			},
		},
	}

	outputProof, err := iotago.NewOutputIDProof(iotago_tpkg.TestAPI, txCommitment, txCreationSlot, iotago.TxEssenceOutputs{iotaOutput}, 0)
	require.NoError(t, err)

	output := CreateOutputAndAssertSerialization(t, blockID, index, iotaOutput, outputProof)
	spent := CreateSpentAndAssertSerialization(t, output)
	outputID := output.OutputID()

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}
