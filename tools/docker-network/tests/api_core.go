//go:build dockertests

package tests

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type coreAPIAssets map[iotago.SlotIndex]*coreAPISlotAssets

func (a coreAPIAssets) setupAssetsForSlot(slot iotago.SlotIndex) {
	_, ok := a[slot]
	if !ok {
		a[slot] = newAssetsPerSlot()
	}
}

func (a coreAPIAssets) assertCommitments(t *testing.T) {
	for _, asset := range a {
		asset.assertCommitments(t)
	}
}

func (a coreAPIAssets) assertBICs(t *testing.T) {
	for _, asset := range a {
		asset.assertBICs(t)
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

func (a coreAPIAssets) forEachTransaction(t *testing.T, f func(*testing.T, *iotago.SignedTransaction, iotago.BlockID)) {
	for _, asset := range a {
		for i, tx := range asset.transactions {
			blockID := asset.valueBlocks[i].MustID()
			f(t, tx, blockID)
		}
	}
}

func (a coreAPIAssets) forEachReattachment(t *testing.T, f func(*testing.T, iotago.BlockID)) {
	for _, asset := range a {
		for _, reattachment := range asset.reattachments {
			f(t, reattachment)
		}
	}
}

func (a coreAPIAssets) forEachOutput(t *testing.T, f func(*testing.T, iotago.OutputID, iotago.Output)) {
	for _, asset := range a {
		for outID, out := range asset.basicOutputs {
			f(t, outID, out)
		}
		for outID, out := range asset.faucetOutputs {
			f(t, outID, out)
		}
		for outID, out := range asset.delegationOutputs {
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

func (a coreAPIAssets) forEachAccountAddress(t *testing.T, f func(t *testing.T, accountAddress *iotago.AccountAddress, commitmentPerNode map[string]iotago.CommitmentID, bicPerNode map[string]iotago.BlockIssuanceCredits)) {
	for _, asset := range a {
		if asset.accountAddress == nil {
			// no account created in this slot
			continue
		}
		f(t, asset.accountAddress, asset.commitmentPerNode, asset.bicPerNode)
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
	accountAddress    *iotago.AccountAddress
	dataBlocks        []*iotago.Block
	valueBlocks       []*iotago.Block
	transactions      []*iotago.SignedTransaction
	reattachments     []iotago.BlockID
	basicOutputs      map[iotago.OutputID]iotago.Output
	faucetOutputs     map[iotago.OutputID]iotago.Output
	delegationOutputs map[iotago.OutputID]iotago.Output

	commitmentPerNode map[string]iotago.CommitmentID
	bicPerNode        map[string]iotago.BlockIssuanceCredits
}

func (a *coreAPISlotAssets) assertCommitments(t *testing.T) {
	prevCommitment := a.commitmentPerNode["V1"]
	for _, commitmentID := range a.commitmentPerNode {
		if prevCommitment == iotago.EmptyCommitmentID {
			require.Fail(t, "commitment is empty")
		}

		require.Equal(t, commitmentID, prevCommitment)
		prevCommitment = commitmentID
	}
}

func (a *coreAPISlotAssets) assertBICs(t *testing.T) {
	prevBIC := a.bicPerNode["V1"]
	for _, bic := range a.bicPerNode {
		require.Equal(t, bic, prevBIC)
		prevBIC = bic
	}
}

func newAssetsPerSlot() *coreAPISlotAssets {
	return &coreAPISlotAssets{
		dataBlocks:        make([]*iotago.Block, 0),
		valueBlocks:       make([]*iotago.Block, 0),
		transactions:      make([]*iotago.SignedTransaction, 0),
		reattachments:     make([]iotago.BlockID, 0),
		basicOutputs:      make(map[iotago.OutputID]iotago.Output),
		faucetOutputs:     make(map[iotago.OutputID]iotago.Output),
		delegationOutputs: make(map[iotago.OutputID]iotago.Output),
		commitmentPerNode: make(map[string]iotago.CommitmentID),
		bicPerNode:        make(map[string]iotago.BlockIssuanceCredits),
	}
}

func (d *DockerTestFramework) prepareAssets(totalAssetsNum int) (coreAPIAssets, iotago.SlotIndex) {
	assets := make(coreAPIAssets)
	ctx := context.Background()

	latestSlot := iotago.SlotIndex(0)

	for i := 0; i < totalAssetsNum; i++ {
		// account
		account := d.CreateAccount()
		assets.setupAssetsForSlot(account.OutputID.Slot())
		assets[account.OutputID.Slot()].accountAddress = account.Address

		// data block
		block := d.CreateTaggedDataBlock(account.ID, []byte("tag"))
		blockSlot := lo.PanicOnErr(block.ID()).Slot()
		assets.setupAssetsForSlot(blockSlot)
		assets[blockSlot].dataBlocks = append(assets[blockSlot].dataBlocks, block)
		d.SubmitBlock(ctx, block)

		// transaction
		valueBlock, signedTx, faucetOutput := d.CreateBasicOutputBlock(account.ID)
		valueBlockSlot := valueBlock.MustID().Slot()
		assets.setupAssetsForSlot(valueBlockSlot)
		// transaction and outputs are stored with the earliest included block
		assets[valueBlockSlot].valueBlocks = append(assets[valueBlockSlot].valueBlocks, valueBlock)
		assets[valueBlockSlot].transactions = append(assets[valueBlockSlot].transactions, signedTx)
		basicOutputID := iotago.OutputIDFromTransactionIDAndIndex(signedTx.Transaction.MustID(), 0)
		assets[valueBlockSlot].basicOutputs[basicOutputID] = signedTx.Transaction.Outputs[0]
		assets[valueBlockSlot].faucetOutputs[faucetOutput.ID] = faucetOutput.Output
		d.SubmitBlock(ctx, valueBlock)
		d.AwaitTransactionPayloadAccepted(ctx, signedTx.Transaction.MustID())

		// issue reattachment after the fisrt one is already included
		issuerResp, congestionResp := d.PrepareBlockIssuance(ctx, d.wallet.DefaultClient(), account.Address)
		secondAttachment := d.SubmitPayload(ctx, signedTx, account.Address.AccountID(), congestionResp, issuerResp)
		assets[valueBlockSlot].reattachments = append(assets[valueBlockSlot].reattachments, secondAttachment)

		// delegation
		delegationOutputID, delegationOutput := d.DelegateToValidator(account.ID, d.Node("V1").AccountAddress(d.Testing))
		assets.setupAssetsForSlot(delegationOutputID.CreationSlot())
		assets[delegationOutputID.CreationSlot()].delegationOutputs[delegationOutputID] = delegationOutput

		latestSlot = lo.Max[iotago.SlotIndex](latestSlot, blockSlot, valueBlockSlot, delegationOutputID.CreationSlot(), secondAttachment.Slot())

		fmt.Printf("Assets for slot %d\n: dataBlock: %s block: %s\ntx: %s\nbasic output: %s, faucet output: %s\n delegation output: %s\n",
			valueBlockSlot, block.MustID().String(), valueBlock.MustID().String(), signedTx.MustID().String(),
			basicOutputID.String(), faucetOutput.ID.String(), delegationOutputID.String())
	}

	return assets, latestSlot
}

func (d *DockerTestFramework) requestFromClients(testFunc func(*testing.T, string)) {
	for alias := range d.nodes {
		testFunc(d.Testing, alias)
	}
}
