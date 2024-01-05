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

func (t *TestSuite) IssueValidationBlockWithHeaderOptions(blockName string, node *mock.Node, blockHeaderOpts ...options.Option[mock.BlockHeaderParams]) *blocks.Block {
	t.Wait(t.Nodes()...)
	t.assertParentsExistFromBlockOptions(blockHeaderOpts, node)
	t.assertParentsCommitmentExistFromBlockOptions(blockHeaderOpts, node)

	t.mutex.Lock()
	defer t.mutex.Unlock()

	timeProvider := t.API.TimeProvider()
	issuingTime := timeProvider.SlotStartTime(t.currentSlot).Add(time.Duration(t.uniqueBlockTimeCounter.Add(1)))
	// Prepend the issuing time so it can be overridden via the passed options.
	blockHeaderOptions := append([]options.Option[mock.BlockHeaderParams]{mock.WithIssuingTime(issuingTime)}, blockHeaderOpts...)

	block := node.IssueValidationBlock(context.Background(), blockName, mock.WithValidationBlockHeaderOptions(blockHeaderOptions...))

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

func (t *TestSuite) IssueValidationBlockWithOptions(blockName string, node *mock.Node, blockOpts ...options.Option[mock.ValidationBlockParams]) *blocks.Block {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	block := node.IssueValidationBlock(context.Background(), blockName, blockOpts...)

	t.registerBlock(blockName, block)

	return block
}

func (t *TestSuite) IssueBasicBlockWithOptions(blockName string, wallet *mock.Wallet, payload iotago.Payload, blockOpts ...options.Option[mock.BlockHeaderParams]) *blocks.Block {
	t.Wait(t.Nodes()...)
	t.assertParentsExistFromBlockOptions(blockOpts, wallet.Node)
	t.assertParentsCommitmentExistFromBlockOptions(blockOpts, wallet.Node)

	t.mutex.Lock()
	defer t.mutex.Unlock()

	timeProvider := t.API.TimeProvider()
	issuingTime := timeProvider.SlotStartTime(t.currentSlot).Add(time.Duration(t.uniqueBlockTimeCounter.Add(1)))
	blockHeaderOptions := append(blockOpts, mock.WithIssuingTime(issuingTime))

	block := wallet.IssueBasicBlock(context.Background(), blockName, mock.WithBasicBlockHeader(blockHeaderOptions...), mock.WithPayload(payload))

	t.registerBlock(blockName, block)

	return block
}

func (t *TestSuite) IssueCandidacyAnnouncementInSlot(alias string, slot iotago.SlotIndex, parentsPrefixAlias string, wallet *mock.Wallet, issuingOptions ...options.Option[mock.BlockHeaderParams]) *blocks.Block {
	t.SetCurrentSlot(slot)

	return t.IssueBasicBlockWithOptions(
		alias,
		wallet,
		&iotago.CandidacyAnnouncement{},
		append(issuingOptions,
			mock.WithStrongParents(t.BlockIDsWithPrefix(parentsPrefixAlias)...),
		)...,
	)
}

func (t *TestSuite) issueBlockRow(prefix string, row int, parentsPrefix string, nodes []*mock.Node, issuingOptions map[string][]options.Option[mock.BlockHeaderParams]) []*blocks.Block {
	blocksIssued := make([]*blocks.Block, 0, len(nodes))

	strongParents := t.BlockIDsWithPrefix(parentsPrefix)
	issuingOptionsCopy := lo.MergeMaps(make(map[string][]options.Option[mock.BlockHeaderParams]), issuingOptions)

	for _, node := range nodes {
		blockName := fmt.Sprintf("%s%d.%d-%s", prefix, t.currentSlot, row, node.Name)
		issuingOptionsCopy[node.Name] = append(issuingOptionsCopy[node.Name], mock.WithStrongParents(strongParents...))

		timeProvider := t.API.TimeProvider()
		issuingTime := timeProvider.SlotStartTime(t.currentSlot).Add(time.Duration(t.uniqueBlockTimeCounter.Add(1)))

		var b *blocks.Block
		// Only issue validation blocks if account has staking feature and is part of committee.
		if node.Validator != nil && lo.Return1(node.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(t.currentSlot)).HasAccount(node.Validator.AccountID) {
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
			tx := t.DefaultWallet().CreateBasicOutputsEquallyFromInput(txName, 1, inputName)

			issuingOptionsCopy[node.Name] = t.limitParentsCountInBlockOptions(issuingOptionsCopy[node.Name], iotago.BasicBlockMaxParents)
			t.assertParentsCommitmentExistFromBlockOptions(issuingOptionsCopy[node.Name], node)
			t.assertParentsExistFromBlockOptions(issuingOptionsCopy[node.Name], node)

			t.DefaultWallet().SetDefaultNode(node)
			b = t.IssueBasicBlockWithOptions(blockName, t.DefaultWallet(), tx, issuingOptionsCopy[node.Name]...)
		}
		blocksIssued = append(blocksIssued, b)
	}

	return blocksIssued
}

func (t *TestSuite) issueBlockRows(prefix string, rows int, initialParentsPrefix string, nodes []*mock.Node, issuingOptions map[string][]options.Option[mock.BlockHeaderParams]) (allBlocksIssued []*blocks.Block, lastBlockRow []*blocks.Block) {
	var blocksIssued, lastBlockRowIssued []*blocks.Block
	parentsPrefix := initialParentsPrefix

	for row := 0; row < rows; row++ {
		if row > 0 {
			parentsPrefix = fmt.Sprintf("%s%d.%d", prefix, t.currentSlot, row-1)
		}

		lastBlockRowIssued = t.issueBlockRow(prefix, row, parentsPrefix, nodes, issuingOptions)
		blocksIssued = append(blocksIssued, lastBlockRowIssued...)
	}

	return blocksIssued, lastBlockRowIssued
}

func (t *TestSuite) IssueBlocksAtSlots(prefix string, slots []iotago.SlotIndex, rowsPerSlot int, initialParentsPrefix string, nodes []*mock.Node, waitForSlotsCommitted bool, useCommitmentAtMinCommittableAge bool) (allBlocksIssued []*blocks.Block, lastBlockRow []*blocks.Block) {
	var blocksIssued, lastBlockRowIssued []*blocks.Block
	parentsPrefix := initialParentsPrefix

	issuingOptions := make(map[string][]options.Option[mock.BlockHeaderParams])

	for i, slot := range slots {
		// advance time of the test suite
		t.SetCurrentSlot(slot)
		if i > 0 {
			parentsPrefix = fmt.Sprintf("%s%d.%d", prefix, slots[i-1], rowsPerSlot-1)
		}

		blocksInSlot, lastRowInSlot := t.issueBlockRows(prefix, rowsPerSlot, parentsPrefix, nodes, issuingOptions)
		blocksIssued = append(blocksIssued, blocksInSlot...)
		lastBlockRowIssued = lastRowInSlot

		if waitForSlotsCommitted {
			if slot > t.API.ProtocolParameters().MinCommittableAge() {
				commitmentSlot := slot - t.API.ProtocolParameters().MinCommittableAge()
				t.AssertCommitmentSlotIndexExists(commitmentSlot, nodes...)

				if useCommitmentAtMinCommittableAge {
					// Make sure that all nodes create blocks throughout the slot that commit to the same commitment at slot-minCommittableAge-1.
					for _, node := range nodes {
						commitment, err := node.Protocol.Engines.Main.Get().Storage.Commitments().Load(commitmentSlot)
						require.NoError(t.Testing, err)

						issuingOptions[node.Name] = []options.Option[mock.BlockHeaderParams]{
							mock.WithSlotCommitment(commitment.Commitment()),
						}
					}
				}
			} else {
				t.AssertBlocksExist(blocksInSlot, true, nodes...)
			}
		}
	}

	return blocksIssued, lastBlockRowIssued
}

func (t *TestSuite) IssueBlocksAtEpoch(prefix string, epoch iotago.EpochIndex, rowsPerSlot int, initialParentsPrefix string, nodes []*mock.Node, waitForSlotsCommitted bool, useCommitmentAtMinCommittableAge bool) (allBlocksIssued []*blocks.Block, lastBlockRow []*blocks.Block) {
	return t.IssueBlocksAtSlots(prefix, t.SlotsForEpoch(epoch), rowsPerSlot, initialParentsPrefix, nodes, waitForSlotsCommitted, useCommitmentAtMinCommittableAge)
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

func (t *TestSuite) CommitUntilSlot(slot iotago.SlotIndex, parents ...iotago.BlockID) []iotago.BlockID {

	// we need to get accepted tangle time up to slot + minCA + 1
	// first issue a chain of blocks with step size minCA up until slot + minCA + 1
	// then issue one more block to accept the last in the chain which will trigger commitment of the second last in the chain
	activeValidators := t.Validators()

	latestCommittedSlot := activeValidators[0].Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Slot()
	if latestCommittedSlot >= slot {
		return parents
	}

	t.SetCurrentSlot(lo.Min(slot+t.API.ProtocolParameters().MinCommittableAge(), latestCommittedSlot+t.API.ProtocolParameters().MinCommittableAge()))

	tips := parents
	chainIndex := 0

	for {
		// preacceptance of nextBlockSlot
		for _, node := range activeValidators {
			require.True(t.Testing, node.IsValidator(), "node: %s: is not a validator node", node.Name)

			committeeAtBlockSlot, exists := node.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(t.currentSlot)

			require.True(t.Testing, exists, "node: %s: does not have committee selected for slot %d", node.Name, t.currentSlot)

			if committeeAtBlockSlot.HasAccount(node.Validator.AccountID) {
				blockName := fmt.Sprintf("chain-%s-%d-%s", parents[0].Alias(), chainIndex, node.Name)
				latestCommitment := node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Commitment()
				tips = []iotago.BlockID{t.IssueValidationBlockWithHeaderOptions(blockName, node, mock.WithSlotCommitment(latestCommitment), mock.WithStrongParents(tips...)).ID()}
			}
		}
		// acceptance of nextBlockSlot
		for _, node := range activeValidators {
			committeeAtBlockSlot, exists := node.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(t.currentSlot)
			require.True(t.Testing, exists, "node: %s: does not have committee selected for slot %d", node.Name, t.currentSlot)
			if committeeAtBlockSlot.HasAccount(node.Validator.AccountID) {
				blockName := fmt.Sprintf("chain-%s-%d-%s", parents[0].Alias(), chainIndex+1, node.Name)
				latestCommitment := node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Commitment()
				tips = []iotago.BlockID{t.IssueValidationBlockWithHeaderOptions(blockName, node, mock.WithSlotCommitment(latestCommitment), mock.WithStrongParents(tips...)).ID()}
			}
		}

		for _, node := range activeValidators {
			t.AssertLatestCommitmentSlotIndex(t.currentSlot-t.API.ProtocolParameters().MinCommittableAge(), node)
		}

		if t.currentSlot == slot+t.API.ProtocolParameters().MinCommittableAge() {
			break
		}

		t.SetCurrentSlot(lo.Min(slot+t.API.ProtocolParameters().MinCommittableAge(), t.currentSlot+t.API.ProtocolParameters().MinCommittableAge()))
		chainIndex += 2
	}

	for _, node := range activeValidators {
		t.AssertLatestCommitmentSlotIndex(slot, node)
	}

	return tips
}
