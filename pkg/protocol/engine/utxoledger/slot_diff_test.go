//nolint:forcetypeassert,varnamelen,revive,exhaustruct // we don't care about these linters in test cases
package utxoledger_test

import (
	"encoding/binary"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/byteutils"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger/tpkg"
	iotago "github.com/iotaledger/iota.go/v4"
	iotago_tpkg "github.com/iotaledger/iota.go/v4/tpkg"
)

func TestSimpleSlotDiffSerialization(t *testing.T) {
	indexBooked := iotago.SlotIndex(255975)

	txID := iotago_tpkg.RandTransactionID()
	outputID := iotago_tpkg.RandOutputID()
	blockID := iotago_tpkg.RandBlockID()
	address := iotago_tpkg.RandAddress(iotago.AddressEd25519)
	amount := iotago.BaseToken(832493)
	iotaOutput := &iotago.BasicOutput{
		Amount: amount,
		UnlockConditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{
				Address: address,
			},
		},
		Features: iotago.BasicOutputFeatures{},
	}

	outputProof, err := iotago.NewOutputIDProof(iotago_tpkg.ZeroCostTestAPI, txID.Identifier(), txID.Slot(), iotago.TxEssenceOutputs{iotaOutput}, 0)
	require.NoError(t, err)

	output := utxoledger.CreateOutput(iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI), outputID, blockID, indexBooked, iotaOutput, outputProof)

	transactionIDSpent := iotago_tpkg.RandTransactionID()

	indexSpent := indexBooked + 1

	spent := utxoledger.NewSpent(output, transactionIDSpent, indexSpent)

	diff := &utxoledger.SlotDiff{
		Slot:    indexSpent,
		Outputs: utxoledger.Outputs{output},
		Spents:  utxoledger.Spents{spent},
	}

	require.Equal(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixSlotDiffs}, lo.PanicOnErr(indexSpent.Bytes())), diff.KVStorableKey())

	value := diff.KVStorableValue()
	require.Equal(t, len(value), iotago.OutputIDLength*2+8)
	require.Equal(t, uint32(1), binary.LittleEndian.Uint32(value[:4]))
	require.Equal(t, outputID[:], value[4:4+iotago.OutputIDLength])
	require.Equal(t, uint32(1), binary.LittleEndian.Uint32(value[4+iotago.OutputIDLength:iotago.OutputIDLength+8]))
	require.Equal(t, outputID[:], value[iotago.OutputIDLength+8:iotago.OutputIDLength*2+8])
}

func TestSlotDiffSerialization(t *testing.T) {
	manager := utxoledger.New(mapdb.NewMapDB(), iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI))

	outputs := utxoledger.Outputs{
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
	}

	slot := iotago.SlotIndex(756)

	spents := utxoledger.Spents{
		tpkg.RandLedgerStateSpentWithOutput(outputs[3], slot),
		tpkg.RandLedgerStateSpentWithOutput(outputs[2], slot),
	}

	require.NoError(t, lo.Return2(manager.ApplyDiffWithoutLocking(slot, outputs, spents)))

	readDiff, err := manager.SlotDiffWithoutLocking(slot)
	require.NoError(t, err)

	var sortedOutputs = utxoledger.LexicalOrderedOutputs(outputs)
	sort.Sort(sortedOutputs)

	var sortedSpents = utxoledger.LexicalOrderedSpents(spents)
	sort.Sort(sortedSpents)

	require.Equal(t, slot, readDiff.Slot)
	tpkg.EqualOutputs(t, utxoledger.Outputs(sortedOutputs), readDiff.Outputs)
	tpkg.EqualSpents(t, utxoledger.Spents(sortedSpents), readDiff.Spents)
}
