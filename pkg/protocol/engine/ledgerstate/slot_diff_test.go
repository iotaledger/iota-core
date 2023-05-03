//nolint:forcetypeassert,varnamelen,revive,exhaustruct // we don't care about these linters in test cases
package ledgerstate_test

import (
	"encoding/binary"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/serializer/v2/byteutils"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate/tpkg"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestSimpleMilestoneDiffSerialization(t *testing.T) {
	indexBooked := iotago.SlotIndex(255975)
	timestampCreated := tpkg.RandTimestamp()

	api := tpkg.API()
	outputID := tpkg.RandOutputID()
	blockID := tpkg.RandBlockID()
	address := tpkg.RandAddress(iotago.AddressEd25519)
	amount := uint64(832493)
	iotaOutput := &iotago.BasicOutput{
		Amount: amount,
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{
				Address: address,
			},
		},
	}
	output := ledgerstate.CreateOutput(api, outputID, blockID, indexBooked, timestampCreated, iotaOutput)

	transactionIDSpent := tpkg.RandTransactionID()

	indexSpent := indexBooked + 1

	spent := ledgerstate.NewSpent(output, transactionIDSpent, timestampCreated.Add(3*time.Second), indexSpent)

	diff := &ledgerstate.SlotDiff{
		Index:   indexSpent,
		Outputs: ledgerstate.Outputs{output},
		Spents:  ledgerstate.Spents{spent},
	}

	require.Equal(t, byteutils.ConcatBytes([]byte{ledgerstate.StoreKeyPrefixSlotDiffs}, indexSpent.Bytes()), diff.KVStorableKey())

	value := diff.KVStorableValue()
	require.Equal(t, len(value), 76)
	require.Equal(t, uint32(1), binary.LittleEndian.Uint32(value[:4]))
	require.Equal(t, outputID[:], value[4:38])
	require.Equal(t, uint32(1), binary.LittleEndian.Uint32(value[38:42]))
	require.Equal(t, outputID[:], value[42:76])
}

func TestMilestoneDiffSerialization(t *testing.T) {
	manager := ledgerstate.New(mapdb.NewMapDB(), tpkg.API)

	outputs := ledgerstate.Outputs{
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
	}

	index := iotago.SlotIndex(756)

	spents := ledgerstate.Spents{
		tpkg.RandLedgerStateSpentWithOutput(outputs[3], index, tpkg.RandTimestamp()),
		tpkg.RandLedgerStateSpentWithOutput(outputs[2], index, tpkg.RandTimestamp()),
	}

	require.NoError(t, manager.ApplyConfirmationWithoutLocking(index, outputs, spents))

	readDiff, err := manager.SlotDiffWithoutLocking(index)
	require.NoError(t, err)

	var sortedOutputs = ledgerstate.LexicalOrderedOutputs(outputs)
	sort.Sort(sortedOutputs)

	var sortedSpents = ledgerstate.LexicalOrderedSpents(spents)
	sort.Sort(sortedSpents)

	require.Equal(t, index, readDiff.Index)
	tpkg.EqualOutputs(t, ledgerstate.Outputs(sortedOutputs), readDiff.Outputs)
	tpkg.EqualSpents(t, ledgerstate.Spents(sortedSpents), readDiff.Spents)
}
