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
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	iotago_tpkg "github.com/iotaledger/iota.go/v4/tpkg"
)

func TestSimpleSlotDiffSerialization(t *testing.T) {
	indexBooked := iotago.SlotIndex(255975)
	slotCreated := utils.RandSlotIndex()

	outputID := utils.RandOutputID()
	blockID := utils.RandBlockID()
	address := utils.RandAddress(iotago.AddressEd25519)
	amount := iotago.BaseToken(832493)
	iotaOutput := &iotago.BasicOutput{
		Amount:       amount,
		NativeTokens: iotago.NativeTokens{},
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{
				Address: address,
			},
		},
		Features: iotago.BasicOutputFeatures{},
	}
	output := utxoledger.CreateOutput(api.SingleVersionProvider(iotago_tpkg.TestAPI), outputID, blockID, indexBooked, slotCreated, iotaOutput)

	transactionIDSpent := utils.RandTransactionID()

	indexSpent := indexBooked + 1

	spent := utxoledger.NewSpent(output, transactionIDSpent, indexSpent)

	diff := &utxoledger.SlotDiff{
		Index:   indexSpent,
		Outputs: utxoledger.Outputs{output},
		Spents:  utxoledger.Spents{spent},
	}

	require.Equal(t, byteutils.ConcatBytes([]byte{utxoledger.StoreKeyPrefixSlotDiffs}, lo.PanicOnErr(indexSpent.Bytes())), diff.KVStorableKey())

	value := diff.KVStorableValue()
	require.Equal(t, len(value), 76)
	require.Equal(t, uint32(1), binary.LittleEndian.Uint32(value[:4]))
	require.Equal(t, outputID[:], value[4:38])
	require.Equal(t, uint32(1), binary.LittleEndian.Uint32(value[38:42]))
	require.Equal(t, outputID[:], value[42:76])
}

func TestSlotDiffSerialization(t *testing.T) {
	manager := utxoledger.New(mapdb.NewMapDB(), api.SingleVersionProvider(iotago_tpkg.TestAPI))

	outputs := utxoledger.Outputs{
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
	}

	index := iotago.SlotIndex(756)

	spents := utxoledger.Spents{
		tpkg.RandLedgerStateSpentWithOutput(outputs[3], index),
		tpkg.RandLedgerStateSpentWithOutput(outputs[2], index),
	}

	require.NoError(t, manager.ApplyDiffWithoutLocking(index, outputs, spents))

	readDiff, err := manager.SlotDiffWithoutLocking(index)
	require.NoError(t, err)

	var sortedOutputs = utxoledger.LexicalOrderedOutputs(outputs)
	sort.Sort(sortedOutputs)

	var sortedSpents = utxoledger.LexicalOrderedSpents(spents)
	sort.Sort(sortedSpents)

	require.Equal(t, index, readDiff.Index)
	tpkg.EqualOutputs(t, utxoledger.Outputs(sortedOutputs), readDiff.Outputs)
	tpkg.EqualSpents(t, utxoledger.Spents(sortedSpents), readDiff.Spents)
}
