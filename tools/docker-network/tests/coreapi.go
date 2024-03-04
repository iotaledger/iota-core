//go:build dockertests

package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type coreAPIAssets map[iotago.SlotIndex]*coreAPISlotAssets

func (a coreAPIAssets) assetForSlot(slot iotago.SlotIndex, account *AccountData) *coreAPISlotAssets {
	_, ok := a[slot]
	if !ok {
		a[slot] = newAssetsPerSlot()
	}
	a[slot].accountSlot = account.OutputID.Slot()
	a[slot].accountAddress = account.Address

	return a[slot]
}

func (a coreAPIAssets) assertCommitments(t *testing.T) {
	for _, asset := range a {
		asset.assertCommitments(t)
	}
}

func (a coreAPIAssets) forEachBlock(t *testing.T, f func(*testing.T, *iotago.Block)) {
	for _, asset := range a {
		for _, block := range asset.dataBlocks {
			f(t, block)
		}
		for _, block := range asset.valueBlocks {
			f(t, block)
		}
	}
}

func (a coreAPIAssets) forEachTransaction(t *testing.T, f func(*testing.T, *iotago.SignedTransaction)) {
	for _, asset := range a {
		for _, tx := range asset.transactions {
			f(t, tx)
		}
	}
}

func (a coreAPIAssets) forEachOutput(t *testing.T, f func(*testing.T, iotago.OutputID, iotago.Output)) {
	for _, asset := range a {
		for outID, out := range asset.basicOutputs {
			f(t, outID, out)
		}
	}
}

func (a coreAPIAssets) forEachSlot(t *testing.T, f func(*testing.T, iotago.SlotIndex, map[string]iotago.CommitmentID)) {
	for slot, slotAssets := range a {
		f(t, slot, slotAssets.commitmentPerNode)
	}
}

func (a coreAPIAssets) forEachCommitment(t *testing.T, f func(*testing.T, map[string]iotago.CommitmentID)) {
	for _, asset := range a {
		f(t, asset.commitmentPerNode)
	}
}

func (a coreAPIAssets) forEachAccountAddress(t *testing.T, f func(*testing.T, *iotago.AccountAddress, map[string]iotago.CommitmentID)) {
	for _, asset := range a {
		f(t, asset.accountAddress, asset.commitmentPerNode)
	}
}

func (a coreAPIAssets) assertUTXOOutputIDsInSlot(t *testing.T, slot iotago.SlotIndex, createdOutputs iotago.OutputIDs, spentOutputs iotago.OutputIDs) {
	created := make(map[iotago.OutputID]types.Empty)
	spent := make(map[iotago.OutputID]types.Empty)
	for _, outputID := range createdOutputs {

		created[outputID] = types.Void
	}

	for _, outputID := range spentOutputs {
		spent[outputID] = types.Void
	}

	for outID := range a[slot].basicOutputs {
		_, ok := created[outID]
		require.True(t, ok, "Output ID not found in created outputs: %s, for slot %d", outID, slot)
	}

	for outID := range a[slot].faucetOutputs {
		_, ok := spent[outID]
		require.True(t, ok, "Output ID not found in spent outputs: %s, for slot %d", outID, slot)
	}
}

func (a coreAPIAssets) assertUTXOOutputsInSlot(t *testing.T, slot iotago.SlotIndex, created []*api.OutputWithID, spent []*api.OutputWithID) {
	createdMap := make(map[iotago.OutputID]iotago.Output)
	spentMap := make(map[iotago.OutputID]iotago.Output)
	for _, output := range created {
		createdMap[output.OutputID] = output.Output
	}
	for _, output := range spent {
		spentMap[output.OutputID] = output.Output
	}

	for outID, out := range a[slot].basicOutputs {
		_, ok := createdMap[outID]
		require.True(t, ok, "Output ID not found in created outputs: %s, for slot %d", outID, slot)
		require.Equal(t, out, createdMap[outID], "Output not equal for ID: %s, for slot %d", outID, slot)
	}

	for outID, out := range a[slot].faucetOutputs {
		_, ok := spentMap[outID]
		require.True(t, ok, "Output ID not found in spent outputs: %s, for slot %d", outID, slot)
		require.Equal(t, out, spentMap[outID], "Output not equal for ID: %s, for slot %d", outID, slot)
	}
}

type coreAPISlotAssets struct {
	//slot           iotago.SlotIndex
	//epoch          iotago.EpochIndex
	accountAddress *iotago.AccountAddress
	accountSlot    iotago.SlotIndex
	dataBlocks     []*iotago.Block
	valueBlocks    []*iotago.Block
	transactions   []*iotago.SignedTransaction
	basicOutputs   map[iotago.OutputID]iotago.Output
	faucetOutputs  map[iotago.OutputID]iotago.Output

	commitmentPerNode map[string]iotago.CommitmentID
}

func (a *coreAPISlotAssets) assertCommitments(t *testing.T) {
	prevCommitment := a.commitmentPerNode["V1"]
	for _, commitmentID := range a.commitmentPerNode {
		if prevCommitment == iotago.EmptyCommitmentID {
			require.Fail(t, "commitment is empty")
		}

		require.Equal(t, commitmentID, prevCommitment)
	}
}

func newAssetsPerSlot() *coreAPISlotAssets {
	return &coreAPISlotAssets{
		commitmentPerNode: make(map[string]iotago.CommitmentID),
		dataBlocks:        make([]*iotago.Block, 0),
		valueBlocks:       make([]*iotago.Block, 0),
		transactions:      make([]*iotago.SignedTransaction, 0),
		basicOutputs:      make(map[iotago.OutputID]iotago.Output),
		faucetOutputs:     make(map[iotago.OutputID]iotago.Output),
	}
}

func (d *DockerTestFramework) prepareAssets(totalAssetsNum int) (coreAPIAssets, iotago.SlotIndex) {
	assets := make(coreAPIAssets)
	ctx := context.Background()
	account := d.CreateAccount()
	accountSlot := account.OutputID.Slot()
	latestSlot := iotago.SlotIndex(0)
	assets.assetForSlot(accountSlot, account)

	for i := 0; i < totalAssetsNum; i++ {
		fmt.Println("Creating asset: ", i)
		block := d.CreateTaggedDataBlock(account.ID, []byte("tag"))
		blockSlot := lo.PanicOnErr(block.ID()).Slot()
		latestSlot = lo.Max[iotago.SlotIndex](latestSlot, blockSlot)
		assets.assetForSlot(blockSlot, account).dataBlocks = append(assets[blockSlot].dataBlocks, block)
		d.SubmitBlock(ctx, block)

		block, signedTx, faucetOutput, basicOut := d.CreateValueBlock(account.ID)
		valueBlockSlot := block.MustID().Slot()
		latestSlot = lo.Max[iotago.SlotIndex](latestSlot, valueBlockSlot)
		d.SubmitBlock(ctx, block)

		assets.assetForSlot(valueBlockSlot, account).valueBlocks = append(assets[valueBlockSlot].valueBlocks, block)
		txSlot := lo.PanicOnErr(signedTx.ID()).Slot()
		latestSlot = lo.Max[iotago.SlotIndex](latestSlot, txSlot)

		assets.assetForSlot(txSlot, account).transactions = append(assets[txSlot].transactions, signedTx)
		basicOutpID := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0)
		assets[txSlot].basicOutputs[basicOutpID] = basicOut
		assets[txSlot].faucetOutputs[faucetOutput.ID] = faucetOutput.Output
		time.Sleep(1 * time.Second)
	}

	return assets, latestSlot
}

func (d *DockerTestFramework) requestFromClients(testFunc func(*testing.T, string)) {
	for alias := range d.nodes {
		testFunc(d.Testing, alias)
	}
}
