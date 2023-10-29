package testsuite

import (
	"context"
	"fmt"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) assertParentsCommitmentExistFromBlockOptions(blockOpts []options.Option[mock.BlockHeaderParams], node *mock.Node) {
	params := options.Apply(&mock.BlockHeaderParams{}, blockOpts)
	parents := params.References[iotago.StrongParentType]
	parents = append(parents, params.References[iotago.WeakParentType]...)
	parents = append(parents, params.References[iotago.ShallowLikeParentType]...)

	for _, block := range t.Blocks(lo.Map(parents, func(id iotago.BlockID) string { return id.Alias() })...) {
		t.AssertCommitmentSlotIndexExists(block.SlotCommitmentID().Slot(), node)
	}
}

func (t *TestSuite) assertParentsExistFromBlockOptions(blockOpts []options.Option[mock.BlockHeaderParams], node *mock.Node) {
	params := options.Apply(&mock.BlockHeaderParams{}, blockOpts)
	parents := params.References[iotago.StrongParentType]
	parents = append(parents, params.References[iotago.WeakParentType]...)
	parents = append(parents, params.References[iotago.ShallowLikeParentType]...)

	t.AssertBlocksExist(t.Blocks(lo.Map(parents, func(id iotago.BlockID) string { return id.Alias() })...), true, node)
}

func (t *TestSuite) limitParentsCountInBlockOptions(blockOpts []options.Option[mock.BlockHeaderParams], maxCount int) []options.Option[mock.BlockHeaderParams] {
	params := options.Apply(&mock.BlockHeaderParams{}, blockOpts)
	if len(params.References[iotago.StrongParentType]) > maxCount {
		blockOpts = append(blockOpts, mock.WithStrongParents(params.References[iotago.StrongParentType][:maxCount]...))
	}
	if len(params.References[iotago.WeakParentType]) > maxCount {
		blockOpts = append(blockOpts, mock.WithWeakParents(params.References[iotago.WeakParentType][:maxCount]...))
	}
	if len(params.References[iotago.ShallowLikeParentType]) > maxCount {
		blockOpts = append(blockOpts, mock.WithShallowLikeParents(params.References[iotago.ShallowLikeParentType][:maxCount]...))
	}

	return blockOpts
}

func (t *TestSuite) RegisterBlock(blockName string, block *blocks.Block) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.registerBlock(blockName, block)
}

func (t *TestSuite) registerBlock(blockName string, block *blocks.Block) {
	t.blocks.Set(blockName, block)
	block.ID().RegisterAlias(blockName)
}

func (t *TestSuite) IssueValidationBlockAtSlot(blockName string, slot iotago.SlotIndex, slotCommitment *iotago.Commitment, node *mock.Node, parents ...iotago.BlockID) *blocks.Block {
	t.AssertBlocksExist(t.Blocks(lo.Map(parents, func(id iotago.BlockID) string { return id.Alias() })...), true, node)

	t.mutex.Lock()
	defer t.mutex.Unlock()

	timeProvider := t.API.TimeProvider()
	issuingTime := timeProvider.SlotStartTime(slot).Add(time.Duration(t.uniqueBlockTimeCounter.Add(1)))

	require.Truef(t.Testing, issuingTime.Before(time.Now()), "node: %s: issued block (%s, slot: %d) is in the current (%s, slot: %d) or future slot", node.Name, issuingTime, slot, time.Now(), timeProvider.SlotFromTime(time.Now()))
	require.True(t.Testing, node.IsValidator(), "node: %s: is not a validator node", node.Name)

	block := node.Validator.IssueValidationBlock(context.Background(), blockName, node, mock.WithValidationBlockHeaderOptions(mock.WithIssuingTime(issuingTime), mock.WithSlotCommitment(slotCommitment), mock.WithStrongParents(parents...)))

	t.registerBlock(blockName, block)

	return block
}

func (t *TestSuite) IssueExistingBlock(blockName string, wallet *mock.Wallet) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	block, exists := t.blocks.Get(blockName)
	require.True(t.Testing, exists)
	require.NotNil(t.Testing, block)

	require.NoError(t.Testing, wallet.BlockIssuer.IssueBlock(block.ModelBlock(), wallet.Node))
}

func (t *TestSuite) IssueValidationBlockWithOptions(blockName string, node *mock.Node, blockOpts ...options.Option[mock.ValidatorBlockParams]) *blocks.Block {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	block := node.IssueValidationBlock(context.Background(), blockName, blockOpts...)

	t.registerBlock(blockName, block)

	return block
}

func (t *TestSuite) IssueBasicBlockWithOptions(blockName string, wallet *mock.Wallet, blockOpts ...options.Option[mock.BasicBlockParams]) *blocks.Block {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	block := wallet.IssueBasicBlock(context.Background(), blockName, blockOpts...)

	t.registerBlock(blockName, block)

	return block
}

func (t *TestSuite) IssueBasicBlockAtSlotWithOptions(blockName string, slot iotago.SlotIndex, wallet *mock.Wallet, payload iotago.Payload, blockOpts ...options.Option[mock.BlockHeaderParams]) *blocks.Block {
	t.assertParentsExistFromBlockOptions(blockOpts, wallet.Node)

	t.mutex.Lock()
	defer t.mutex.Unlock()

	timeProvider := t.API.TimeProvider()
	issuingTime := timeProvider.SlotStartTime(slot).Add(time.Duration(t.uniqueBlockTimeCounter.Add(1)))

	require.Truef(t.Testing, issuingTime.Before(time.Now()), "wallet: %s: issued block (%s, slot: %d) is in the current (%s, slot: %d) or future slot", wallet.Name, issuingTime, slot, time.Now(), timeProvider.SlotFromTime(time.Now()))

	block := wallet.IssueBasicBlock(context.Background(), blockName, mock.WithBasicBlockHeader(append(blockOpts, mock.WithIssuingTime(issuingTime))...), mock.WithPayload(payload))

	t.registerBlock(blockName, block)

	return block
}

func (t *TestSuite) IssuePayloadWithOptions(blockName string, wallet *mock.Wallet, payload iotago.Payload, blockHeaderOpts ...options.Option[mock.BlockHeaderParams]) *blocks.Block {
	t.assertParentsExistFromBlockOptions(blockHeaderOpts, wallet.Node)

	t.mutex.Lock()
	defer t.mutex.Unlock()

	block := wallet.IssueBasicBlock(context.Background(), blockName, mock.WithPayload(payload), mock.WithBasicBlockHeader(blockHeaderOpts...))

	t.registerBlock(blockName, block)

	return block
}

func (t *TestSuite) IssueValidationBlock(blockName string, node *mock.Node, blockHeaderOpts ...options.Option[mock.BlockHeaderParams]) *blocks.Block {
	t.assertParentsExistFromBlockOptions(blockHeaderOpts, node)

	require.Truef(t.Testing, node.IsValidator(), "node: %s: is not a validator node", node.Name)

	t.mutex.Lock()
	defer t.mutex.Unlock()

	block := node.Validator.IssueValidationBlock(context.Background(), blockName, node, mock.WithValidationBlockHeaderOptions(blockHeaderOpts...))

	t.registerBlock(blockName, block)

	return block
}
func (t *TestSuite) IssueCandidacyAnnouncementInSlot(alias string, slot iotago.SlotIndex, parentsPrefixAlias string, wallet *mock.Wallet, issuingOptions ...options.Option[mock.BlockHeaderParams]) *blocks.Block {
	timeProvider := t.API.TimeProvider()
	issuingTime := timeProvider.SlotStartTime(slot).Add(time.Duration(t.uniqueBlockTimeCounter.Add(1)))
	require.Truef(t.Testing, issuingTime.Before(time.Now()), "wallet: %s: issued block (%s, slot: %d) is in the current (%s, slot: %d) or future slot", wallet.Name, issuingTime, slot, time.Now(), timeProvider.SlotFromTime(time.Now()))

	return t.IssuePayloadWithOptions(
		alias,
		wallet,
		&iotago.CandidacyAnnouncement{},
		append(issuingOptions,
			mock.WithStrongParents(t.BlockIDsWithPrefix(parentsPrefixAlias)...),
			mock.WithIssuingTime(issuingTime),
		)...,
	)
}

func (t *TestSuite) IssueBlockRowInSlot(prefix string, slot iotago.SlotIndex, row int, parentsPrefix string, nodes []*mock.Node, issuingOptions map[string][]options.Option[mock.BlockHeaderParams]) []*blocks.Block {
	blocksIssued := make([]*blocks.Block, 0, len(nodes))

	strongParents := t.BlockIDsWithPrefix(parentsPrefix)
	issuingOptionsCopy := lo.MergeMaps(make(map[string][]options.Option[mock.BlockHeaderParams]), issuingOptions)

	for _, node := range nodes {
		blockName := fmt.Sprintf("%s%d.%d-%s", prefix, slot, row, node.Name)
		issuingOptionsCopy[node.Name] = append(issuingOptionsCopy[node.Name], mock.WithStrongParents(strongParents...))

		timeProvider := t.API.TimeProvider()
		issuingTime := timeProvider.SlotStartTime(slot).Add(time.Duration(t.uniqueBlockTimeCounter.Add(1)))
		require.Truef(t.Testing, issuingTime.Before(time.Now()), "node: %s: issued block (%s, slot: %d) is in the current (%s, slot: %d) or future slot", node.Name, issuingTime, slot, time.Now(), timeProvider.SlotFromTime(time.Now()))

		var b *blocks.Block
		// Only issue validator blocks if account has staking feature and is part of committee.
		if node.Validator != nil && lo.Return1(node.Protocol.MainEngineInstance().SybilProtection.SeatManager().CommitteeInSlot(slot)).HasAccount(node.Validator.AccountID) {
			blockHeaderOptions := append(issuingOptionsCopy[node.Name], mock.WithIssuingTime(issuingTime))
			t.assertParentsCommitmentExistFromBlockOptions(blockHeaderOptions, node)
			t.assertParentsExistFromBlockOptions(blockHeaderOptions, node)

			b = t.IssueValidationBlockWithOptions(blockName, node, mock.WithValidationBlockHeaderOptions(blockHeaderOptions...), mock.WithHighestSupportedVersion(node.HighestSupportedVersion()), mock.WithProtocolParametersHash(node.ProtocolParametersHash()))
		} else {
			txCount := t.automaticTransactionIssuingCounters.Compute(node.Partition, func(currentValue int, exists bool) int {
				return currentValue + 1
			})
			inputName := fmt.Sprintf("automaticSpent-%d:0", txCount-1)
			txName := fmt.Sprintf("automaticSpent-%d", txCount)
			if txCount == 1 {
				inputName = "Genesis:0"
			}
			tx := t.DefaultWallet().CreateBasicOutputsEquallyFromInputs(txName, 1, inputName)

			issuingOptionsCopy[node.Name] = t.limitParentsCountInBlockOptions(issuingOptionsCopy[node.Name], iotago.BlockMaxParents)

			blockHeaderOptions := append(issuingOptionsCopy[node.Name], mock.WithIssuingTime(issuingTime))
			t.assertParentsCommitmentExistFromBlockOptions(blockHeaderOptions, node)
			t.assertParentsExistFromBlockOptions(blockHeaderOptions, node)

			t.DefaultWallet().SetDefaultNode(node)
			b = t.IssueBasicBlockWithOptions(blockName, t.DefaultWallet(), mock.WithPayload(tx), mock.WithBasicBlockHeader(blockHeaderOptions...))
		}
		blocksIssued = append(blocksIssued, b)
	}

	return blocksIssued
}

func (t *TestSuite) IssueBlockRowsInSlot(prefix string, slot iotago.SlotIndex, rows int, initialParentsPrefix string, nodes []*mock.Node, issuingOptions map[string][]options.Option[mock.BlockHeaderParams]) (allBlocksIssued []*blocks.Block, lastBlockRow []*blocks.Block) {
	var blocksIssued, lastBlockRowIssued []*blocks.Block
	parentsPrefix := initialParentsPrefix

	for row := 0; row < rows; row++ {
		if row > 0 {
			parentsPrefix = fmt.Sprintf("%s%d.%d", prefix, slot, row-1)
		}

		lastBlockRowIssued = t.IssueBlockRowInSlot(prefix, slot, row, parentsPrefix, nodes, issuingOptions)
		blocksIssued = append(blocksIssued, lastBlockRowIssued...)
	}

	return blocksIssued, lastBlockRowIssued
}

func (t *TestSuite) IssueBlocksAtSlots(prefix string, slots []iotago.SlotIndex, rowsPerSlot int, initialParentsPrefix string, nodes []*mock.Node, waitForSlotsCommitted bool, issuingOptions map[string][]options.Option[mock.BlockHeaderParams]) (allBlocksIssued []*blocks.Block, lastBlockRow []*blocks.Block) {
	var blocksIssued, lastBlockRowIssued []*blocks.Block
	parentsPrefix := initialParentsPrefix

	for i, slot := range slots {
		if i > 0 {
			parentsPrefix = fmt.Sprintf("%s%d.%d", prefix, slots[i-1], rowsPerSlot-1)
		}

		blocksInSlot, lastRowInSlot := t.IssueBlockRowsInSlot(prefix, slot, rowsPerSlot, parentsPrefix, nodes, issuingOptions)
		blocksIssued = append(blocksIssued, blocksInSlot...)
		lastBlockRowIssued = lastRowInSlot

		if waitForSlotsCommitted {
			if slot > t.API.ProtocolParameters().MinCommittableAge() {
				t.AssertCommitmentSlotIndexExists(slot-(t.API.ProtocolParameters().MinCommittableAge()), nodes...)
			} else {
				t.AssertBlocksExist(blocksInSlot, true, nodes...)
			}
		}
	}

	return blocksIssued, lastBlockRowIssued
}

func (t *TestSuite) IssueBlocksAtEpoch(prefix string, epoch iotago.EpochIndex, rowsPerSlot int, initialParentsPrefix string, nodes []*mock.Node, waitForSlotsCommitted bool, issuingOptions map[string][]options.Option[mock.BlockHeaderParams]) (allBlocksIssued []*blocks.Block, lastBlockRow []*blocks.Block) {
	return t.IssueBlocksAtSlots(prefix, t.SlotsForEpoch(epoch), rowsPerSlot, initialParentsPrefix, nodes, waitForSlotsCommitted, issuingOptions)
}

func (t *TestSuite) SlotsForEpoch(epoch iotago.EpochIndex) []iotago.SlotIndex {
	slotsPerEpoch := t.API.TimeProvider().EpochDurationSlots()

	slots := make([]iotago.SlotIndex, 0, slotsPerEpoch)
	epochStart := t.API.TimeProvider().EpochStart(epoch)
	for i := epochStart; i < epochStart+slotsPerEpoch; i++ {
		if i == 0 {
			continue
		}
		slots = append(slots, i)
	}

	return slots
}

func (t *TestSuite) CommitUntilSlot(slot iotago.SlotIndex, parent *blocks.Block) *blocks.Block {

	// we need to get accepted tangle time up to slot + minCA + 1
	// first issue a chain of blocks with step size minCA up until slot + minCA + 1
	// then issue one more block to accept the last in the chain which will trigger commitment of the second last in the chain
	activeValidators := t.Validators()

	latestCommittedSlot := activeValidators[0].Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Slot()
	if latestCommittedSlot >= slot {
		return parent
	}
	nextBlockSlot := lo.Min(slot+t.API.ProtocolParameters().MinCommittableAge(), latestCommittedSlot+t.API.ProtocolParameters().MinCommittableAge())
	tip := parent
	chainIndex := 0
	for {
		// preacceptance of nextBlockSlot
		for _, node := range activeValidators {
			require.True(t.Testing, node.IsValidator(), "node: %s: is not a validator node", node.Name)
			committeeAtBlockSlot, exists := node.Protocol.MainEngineInstance().SybilProtection.SeatManager().CommitteeInSlot(nextBlockSlot)
			require.True(t.Testing, exists, "node: %s: does not have committee selected for slot %d", node.Name, nextBlockSlot)
			if committeeAtBlockSlot.HasAccount(node.Validator.AccountID) {
				blockName := fmt.Sprintf("chain-%s-%d-%s", parent.ID().Alias(), chainIndex, node.Name)
				tip = t.IssueValidationBlockAtSlot(blockName, nextBlockSlot, node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node, tip.ID())
			}
		}
		// acceptance of nextBlockSlot
		for _, node := range activeValidators {
			committeeAtBlockSlot, exists := node.Protocol.MainEngineInstance().SybilProtection.SeatManager().CommitteeInSlot(nextBlockSlot)
			require.True(t.Testing, exists, "node: %s: does not have committee selected for slot %d", node.Name, nextBlockSlot)
			if committeeAtBlockSlot.HasAccount(node.Validator.AccountID) {
				blockName := fmt.Sprintf("chain-%s-%d-%s", parent.ID().Alias(), chainIndex+1, node.Name)
				tip = t.IssueValidationBlockAtSlot(blockName, nextBlockSlot, node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node, tip.ID())
			}
		}

		for _, node := range activeValidators {
			t.AssertLatestCommitmentSlotIndex(nextBlockSlot-t.API.ProtocolParameters().MinCommittableAge(), node)
		}

		if nextBlockSlot == slot+t.API.ProtocolParameters().MinCommittableAge() {
			break
		}

		nextBlockSlot = lo.Min(slot+t.API.ProtocolParameters().MinCommittableAge(), nextBlockSlot+t.API.ProtocolParameters().MinCommittableAge())
		chainIndex += 2
	}

	for _, node := range activeValidators {
		t.AssertLatestCommitmentSlotIndex(slot, node)
	}

	return tip
}
