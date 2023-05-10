package testsuite

import (
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertNodeState(nodes []*mock.Node, opts ...options.Option[NodeState]) {
	state := options.Apply(&NodeState{}, opts)

	if state.snapshotImported != nil {
		t.AssertSnapshotImported(*state.snapshotImported, nodes...)
	}
	if state.protocolParameters != nil {
		t.AssertProtocolParameters(*state.protocolParameters, nodes...)
	}
	if state.latestCommitment != nil {
		t.AssertLatestCommitment(state.latestCommitment, nodes...)
	}
	if state.latestCommitmentSlotIndex != nil {
		t.AssertLatestCommitmentSlotIndex(*state.latestCommitmentSlotIndex, nodes...)
	}
	if state.latestMutationSlot != nil {
		t.AssertLatestStateMutationSlot(*state.latestMutationSlot, nodes...)
	}
	if state.latestFinalizedSlot != nil {
		t.AssertLatestFinalizedSlot(*state.latestFinalizedSlot, nodes...)
	}
	if state.chainID != nil {
		t.AssertChainID(*state.chainID, nodes...)
	}
	if state.sybilProtectionCommittee != nil {
		t.AssertSybilProtectionCommittee(*state.sybilProtectionCommittee, nodes...)
	}
	if state.sybilProtectionOnlineCommittee != nil {
		t.AssertSybilProtectionOnlineCommittee(*state.sybilProtectionOnlineCommittee, nodes...)
	}
	if state.storageCommitments != nil {
		t.AssertStorageCommitments(*state.storageCommitments, nodes...)
	}
	if state.storageRootBlocks != nil {
		t.AssertStorageRootBlocks(*state.storageRootBlocks, nodes...)
	}
	if state.activeRootBlocks != nil {
		t.AssertActiveRootBlocks(*state.activeRootBlocks, nodes...)
	}
	if state.evictedSlot != nil {
		t.AssertEvictedSlot(*state.evictedSlot, nodes...)
	}
	if state.prunedSlot != nil {
		t.AssertPrunedSlot(*state.prunedSlot, state.hasPruned, nodes...)
	}
}

type NodeState struct {
	snapshotImported          *bool
	protocolParameters        *iotago.ProtocolParameters
	latestCommitment          *iotago.Commitment
	latestCommitmentSlotIndex *iotago.SlotIndex
	latestMutationSlot        *iotago.SlotIndex
	latestFinalizedSlot       *iotago.SlotIndex
	chainID                   *iotago.CommitmentID

	sybilProtectionCommittee       *map[iotago.AccountID]int64
	sybilProtectionOnlineCommittee *map[iotago.AccountID]int64

	storageCommitments *[]*iotago.Commitment

	storageRootBlocks *[]*blocks.Block
	activeRootBlocks  *[]*blocks.Block

	evictedSlot *iotago.SlotIndex
	prunedSlot  *iotago.SlotIndex
	hasPruned   bool
}

func WithSnapshotImported(snapshotImported bool) options.Option[NodeState] {
	return func(state *NodeState) {
		state.snapshotImported = &snapshotImported
	}
}

func WithProtocolParameters(protocolParameters iotago.ProtocolParameters) options.Option[NodeState] {
	return func(state *NodeState) {
		state.protocolParameters = &protocolParameters
	}
}

func WithLatestCommitment(commitment *iotago.Commitment) options.Option[NodeState] {
	return func(state *NodeState) {
		state.latestCommitment = commitment
	}
}

func WithLatestCommitmentSlotIndex(slotIndex iotago.SlotIndex) options.Option[NodeState] {
	return func(state *NodeState) {
		state.latestCommitmentSlotIndex = &slotIndex
	}
}

func WithLatestStateMutationSlot(slotIndex iotago.SlotIndex) options.Option[NodeState] {
	return func(state *NodeState) {
		state.latestMutationSlot = &slotIndex
	}
}

func WithLatestFinalizedSlot(slotIndex iotago.SlotIndex) options.Option[NodeState] {
	return func(state *NodeState) {
		state.latestFinalizedSlot = &slotIndex
	}
}

func WithChainID(chainID iotago.CommitmentID) options.Option[NodeState] {
	return func(state *NodeState) {
		state.chainID = &chainID
	}
}

func WithSybilProtectionCommittee(committee map[iotago.AccountID]int64) options.Option[NodeState] {
	return func(state *NodeState) {
		state.sybilProtectionCommittee = &committee
	}
}

func WithSybilProtectionOnlineCommittee(committee map[iotago.AccountID]int64) options.Option[NodeState] {
	return func(state *NodeState) {
		state.sybilProtectionOnlineCommittee = &committee
	}
}

func WithStorageCommitments(commitments []*iotago.Commitment) options.Option[NodeState] {
	return func(state *NodeState) {
		state.storageCommitments = &commitments
	}
}

func WithStorageRootBlocks(blocks []*blocks.Block) options.Option[NodeState] {
	return func(state *NodeState) {
		state.storageRootBlocks = &blocks
	}
}

func WithActiveRootBlocks(blocks []*blocks.Block) options.Option[NodeState] {
	return func(state *NodeState) {
		state.activeRootBlocks = &blocks
	}
}

func WithEvictedSlot(slotIndex iotago.SlotIndex) options.Option[NodeState] {
	return func(state *NodeState) {
		state.evictedSlot = &slotIndex
	}
}

func WithPrunedSlot(slotIndex iotago.SlotIndex, hasPruned bool) options.Option[NodeState] {
	return func(state *NodeState) {
		state.prunedSlot = &slotIndex
		state.hasPruned = hasPruned
	}
}
