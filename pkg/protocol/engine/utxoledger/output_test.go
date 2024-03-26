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
	iotago "github.com/iotaledger/iota.go/v4"
	iotago_tpkg "github.com/iotaledger/iota.go/v4/tpkg"
)

func AssertOutputUnspentAndSpentTransitions(t *testing.T, output *utxoledger.Output, spent *utxoledger.Spent) {
	outputID := output.OutputID()
	manager := utxoledger.New(mapdb.NewMapDB(), iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI))

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
	require.NoError(t, lo.Return2(manager.ApplyDiff(spent.SlotSpent(), utxoledger.Outputs{}, utxoledger.Spents{spent})))

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

	iotagoAPI := iotago_tpkg.ZeroCostTestAPI
	output := utxoledger.CreateOutput(iotago.SingleVersionProvider(iotagoAPI), outputID, blockID, indexBooked, iotaOutput, outputProof)
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
	transactionID := iotago_tpkg.RandTransactionID()

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
	txCreationSlot := iotago_tpkg.RandSlot()
	blockID := iotago_tpkg.RandBlockID()
	address := iotago_tpkg.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	senderAddress := iotago_tpkg.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	tag := iotago_tpkg.RandBytes(23)
	amount := iotago_tpkg.RandBaseToken(iotago.MaxBaseToken)
	index := iotago_tpkg.RandSlot()

	iotaOutput := &iotago.BasicOutput{
		Amount: amount,
		UnlockConditions: iotago.BasicOutputUnlockConditions{
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

	outputProof, err := iotago.NewOutputIDProof(iotago_tpkg.ZeroCostTestAPI, txCommitment, txCreationSlot, iotago.TxEssenceOutputs{iotaOutput}, 0)
	require.NoError(t, err)

	output := CreateOutputAndAssertSerialization(t, blockID, index, iotaOutput, outputProof)
	spent := CreateSpentAndAssertSerialization(t, output)
	outputID := output.OutputID()

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}

func TestBasicOutputOnEd25519WithSpendConstraintsSerialization(t *testing.T) {
	txCommitment := iotago_tpkg.Rand32ByteArray()
	txCreationSlot := iotago_tpkg.RandSlot()
	blockID := iotago_tpkg.RandBlockID()
	address := iotago_tpkg.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	senderAddress := iotago_tpkg.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	amount := iotago_tpkg.RandBaseToken(iotago.MaxBaseToken)
	index := iotago_tpkg.RandSlot()
	timeLockUnlockSlot := iotago_tpkg.RandSlot()

	iotaOutput := &iotago.BasicOutput{
		Amount: amount,
		UnlockConditions: iotago.BasicOutputUnlockConditions{
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

	outputProof, err := iotago.NewOutputIDProof(iotago_tpkg.ZeroCostTestAPI, txCommitment, txCreationSlot, iotago.TxEssenceOutputs{iotaOutput}, 0)
	require.NoError(t, err)

	output := CreateOutputAndAssertSerialization(t, blockID, index, iotaOutput, outputProof)
	spent := CreateSpentAndAssertSerialization(t, output)

	outputID := output.OutputID()

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}

func TestNFTOutputSerialization(t *testing.T) {
	txCommitment := iotago_tpkg.Rand32ByteArray()
	txCreationSlot := iotago_tpkg.RandSlot()
	blockID := iotago_tpkg.RandBlockID()
	address := iotago_tpkg.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	nftID := iotago_tpkg.RandNFTID()
	amount := iotago_tpkg.RandBaseToken(iotago.MaxBaseToken)
	index := iotago_tpkg.RandSlot()

	iotaOutput := &iotago.NFTOutput{
		Amount: amount,
		NFTID:  nftID,
		UnlockConditions: iotago.NFTOutputUnlockConditions{
			&iotago.AddressUnlockCondition{
				Address: address,
			},
		},
		Features: iotago.NFTOutputFeatures{},
		ImmutableFeatures: iotago.NFTOutputImmFeatures{
			&iotago.MetadataFeature{
				Entries: iotago.MetadataFeatureEntries{
					"data": iotago_tpkg.RandBytes(12),
				},
			},
		},
	}

	outputProof, err := iotago.NewOutputIDProof(iotago_tpkg.ZeroCostTestAPI, txCommitment, txCreationSlot, iotago.TxEssenceOutputs{iotaOutput}, 0)
	require.NoError(t, err)

	output := CreateOutputAndAssertSerialization(t, blockID, index, iotaOutput, outputProof)
	spent := CreateSpentAndAssertSerialization(t, output)
	outputID := output.OutputID()

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}

func TestNFTOutputWithSpendConstraintsSerialization(t *testing.T) {
	txCommitment := iotago_tpkg.Rand32ByteArray()
	txCreationSlot := iotago_tpkg.RandSlot()
	blockID := iotago_tpkg.RandBlockID()
	address := iotago_tpkg.RandNFTID()
	issuerAddress := iotago_tpkg.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	nftID := iotago_tpkg.RandNFTID()
	amount := iotago_tpkg.RandBaseToken(iotago.MaxBaseToken)
	index := iotago_tpkg.RandSlot()
	expirationUnlockSlot := iotago_tpkg.RandSlot()

	iotaOutput := &iotago.NFTOutput{
		Amount: amount,
		NFTID:  nftID,
		UnlockConditions: iotago.NFTOutputUnlockConditions{
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
				Entries: iotago.MetadataFeatureEntries{
					"data": iotago_tpkg.RandBytes(12),
				},
			},
			&iotago.IssuerFeature{
				Address: issuerAddress,
			},
		},
	}

	outputProof, err := iotago.NewOutputIDProof(iotago_tpkg.ZeroCostTestAPI, txCommitment, txCreationSlot, iotago.TxEssenceOutputs{iotaOutput}, 0)
	require.NoError(t, err)

	output := CreateOutputAndAssertSerialization(t, blockID, index, iotaOutput, outputProof)
	spent := CreateSpentAndAssertSerialization(t, output)
	outputID := output.OutputID()

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}

func TestAccountOutputSerialization(t *testing.T) {
	txCommitment := iotago_tpkg.Rand32ByteArray()
	txCreationSlot := iotago_tpkg.RandSlot()
	blockID := iotago_tpkg.RandBlockID()
	aliasID := iotago_tpkg.RandAccountID()
	address := iotago_tpkg.RandAccountID().ToAddress()
	issuer := iotago_tpkg.RandNFTID()
	sender := iotago_tpkg.RandAccountID()
	amount := iotago_tpkg.RandBaseToken(iotago.MaxBaseToken)
	index := iotago_tpkg.RandSlot()

	iotaOutput := &iotago.AccountOutput{
		Amount:    amount,
		AccountID: aliasID,
		UnlockConditions: iotago.AccountOutputUnlockConditions{
			&iotago.AddressUnlockCondition{
				Address: address,
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

	outputProof, err := iotago.NewOutputIDProof(iotago_tpkg.ZeroCostTestAPI, txCommitment, txCreationSlot, iotago.TxEssenceOutputs{iotaOutput}, 0)
	require.NoError(t, err)

	output := CreateOutputAndAssertSerialization(t, blockID, index, iotaOutput, outputProof)
	spent := CreateSpentAndAssertSerialization(t, output)
	outputID := output.OutputID()

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}

func TestAnchorOutputSerialization(t *testing.T) {
	txCommitment := iotago_tpkg.Rand32ByteArray()
	txCreationSlot := iotago_tpkg.RandSlot()
	blockID := iotago_tpkg.RandBlockID()
	aliasID := iotago_tpkg.RandAnchorID()
	stateController := iotago_tpkg.RandAnchorID()
	governor := iotago_tpkg.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	issuer := iotago_tpkg.RandNFTID()
	amount := iotago_tpkg.RandBaseToken(iotago.MaxBaseToken)
	index := iotago_tpkg.RandSlot()

	iotaOutput := &iotago.AnchorOutput{
		Amount:   amount,
		AnchorID: aliasID,
		UnlockConditions: iotago.AnchorOutputUnlockConditions{
			&iotago.StateControllerAddressUnlockCondition{
				Address: stateController.ToAddress(),
			},
			&iotago.GovernorAddressUnlockCondition{
				Address: governor,
			},
		},
		Features: iotago.AnchorOutputFeatures{
			&iotago.StateMetadataFeature{
				Entries: iotago.StateMetadataFeatureEntries{
					"test": []byte("value"),
				},
			},
		},
		ImmutableFeatures: iotago.AnchorOutputImmFeatures{
			&iotago.IssuerFeature{
				Address: issuer.ToAddress(),
			},
		},
	}

	outputProof, err := iotago.NewOutputIDProof(iotago_tpkg.ZeroCostTestAPI, txCommitment, txCreationSlot, iotago.TxEssenceOutputs{iotaOutput}, 0)
	require.NoError(t, err)

	output := CreateOutputAndAssertSerialization(t, blockID, index, iotaOutput, outputProof)
	spent := CreateSpentAndAssertSerialization(t, output)
	outputID := output.OutputID()

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}

func TestFoundryOutputSerialization(t *testing.T) {
	txCommitment := iotago_tpkg.Rand32ByteArray()
	txCreationSlot := iotago_tpkg.RandSlot()
	blockID := iotago_tpkg.RandBlockID()
	aliasID := iotago_tpkg.RandAccountID()
	amount := iotago_tpkg.RandBaseToken(iotago.MaxBaseToken)
	index := iotago_tpkg.RandSlot()
	supply := new(big.Int).SetUint64(iotago_tpkg.RandUint64(math.MaxUint64))

	iotaOutput := &iotago.FoundryOutput{
		Amount:       amount,
		SerialNumber: iotago_tpkg.RandUint32(math.MaxUint32),
		TokenScheme: &iotago.SimpleTokenScheme{
			MintedTokens:  supply,
			MeltedTokens:  new(big.Int).SetBytes([]byte{0}),
			MaximumSupply: supply,
		},
		UnlockConditions: iotago.FoundryOutputUnlockConditions{
			&iotago.ImmutableAccountUnlockCondition{
				Address: aliasID.ToAddress().(*iotago.AccountAddress),
			},
		},
		Features:          iotago.FoundryOutputFeatures{},
		ImmutableFeatures: iotago.FoundryOutputImmFeatures{},
	}

	outputProof, err := iotago.NewOutputIDProof(iotago_tpkg.ZeroCostTestAPI, txCommitment, txCreationSlot, iotago.TxEssenceOutputs{iotaOutput}, 0)
	require.NoError(t, err)

	output := CreateOutputAndAssertSerialization(t, blockID, index, iotaOutput, outputProof)
	spent := CreateSpentAndAssertSerialization(t, output)
	outputID := output.OutputID()

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}

func TestDelegationOutputSerialization(t *testing.T) {
	txCommitment := iotago_tpkg.Rand32ByteArray()
	txCreationSlot := iotago_tpkg.RandSlot()
	blockID := iotago_tpkg.RandBlockID()
	address := iotago_tpkg.RandAddress(iotago.AddressEd25519).(*iotago.Ed25519Address)
	amount := iotago_tpkg.RandBaseToken(iotago.MaxBaseToken)
	index := iotago_tpkg.RandSlot()

	iotaOutput := &iotago.DelegationOutput{
		Amount:           amount,
		DelegatedAmount:  amount,
		DelegationID:     iotago_tpkg.RandDelegationID(),
		ValidatorAddress: iotago_tpkg.RandAddress(iotago.AddressAccount).(*iotago.AccountAddress),
		StartEpoch:       iotago_tpkg.RandEpoch(),
		UnlockConditions: iotago.DelegationOutputUnlockConditions{
			&iotago.AddressUnlockCondition{
				Address: address,
			},
		},
	}

	outputProof, err := iotago.NewOutputIDProof(iotago_tpkg.ZeroCostTestAPI, txCommitment, txCreationSlot, iotago.TxEssenceOutputs{iotaOutput}, 0)
	require.NoError(t, err)

	output := CreateOutputAndAssertSerialization(t, blockID, index, iotaOutput, outputProof)
	spent := CreateSpentAndAssertSerialization(t, output)
	outputID := output.OutputID()

	require.ElementsMatch(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixOutputUnspent}, outputID[:]), output.UnspentLookupKey())
	AssertOutputUnspentAndSpentTransitions(t, output, spent)
}
