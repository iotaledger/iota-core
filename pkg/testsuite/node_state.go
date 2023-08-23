package testsuite

import (
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/core/account"
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
	if state.latestCommitmentCumulativeWeight != nil {
		t.AssertLatestCommitmentCumulativeWeight(*state.latestCommitmentCumulativeWeight, nodes...)
	}
	if state.latestFinalizedSlot != nil {
		t.AssertLatestFinalizedSlot(*state.latestFinalizedSlot, nodes...)
	}
	if state.chainID != nil {
		t.AssertChainID(*state.chainID, nodes...)
	}
	if state.sybilProtectionCommitteeSlotIndex != nil && state.sybilProtectionCommittee != nil {
		t.AssertSybilProtectionCommittee(*state.sybilProtectionCommitteeSlotIndex, *state.sybilProtectionCommittee, nodes...)
	}
	if state.sybilProtectionOnlineCommittee != nil {
		t.AssertSybilProtectionOnlineCommittee(*state.sybilProtectionOnlineCommittee, nodes...)
	}
	if state.storageCommitmentAtIndex != nil {
		t.AssertEqualStoredCommitmentAtIndex(*state.storageCommitmentAtIndex, nodes...)
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
	if state.chainManagerSolid != nil && *state.chainManagerSolid {
		t.AssertChainManagerIsSolid(nodes...)
	}
}

type NodeState struct {
	snapshotImported                 *bool
	protocolParameters               *iotago.ProtocolParameters
	latestCommitment                 *iotago.Commitment
	latestCommitmentSlotIndex        *iotago.SlotIndex
	latestCommitmentCumulativeWeight *uint64
	latestFinalizedSlot              *iotago.SlotIndex
	chainID                          *iotago.CommitmentID

	sybilProtectionCommitteeSlotIndex *iotago.SlotIndex
	sybilProtectionCommittee          *[]iotago.AccountID
	sybilProtectionOnlineCommittee    *[]account.SeatIndex

	storageCommitments       *[]*iotago.Commitment
	storageCommitmentAtIndex *iotago.SlotIndex

	storageRootBlocks *[]*blocks.Block
	activeRootBlocks  *[]*blocks.Block

	evictedSlot *iotago.SlotIndex
	prunedEpoch *iotago.EpochIndex
	hasPruned   bool

	chainManagerSolid *bool
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

func WithEqualStoredCommitmentAtIndex(slotIndex iotago.SlotIndex) options.Option[NodeState] {
	return func(state *NodeState) {
		state.storageCommitmentAtIndex = &slotIndex
	}
}

func WithLatestCommitmentCumulativeWeight(cumulativeWeight uint64) options.Option[NodeState] {
	return func(state *NodeState) {
		state.latestCommitmentCumulativeWeight = &cumulativeWeight
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

func WithSybilProtectionCommittee(index iotago.SlotIndex, committee []iotago.AccountID) options.Option[NodeState] {
	return func(state *NodeState) {
		state.sybilProtectionCommitteeSlotIndex = &index
		state.sybilProtectionCommittee = &committee
	}
}

func WithSybilProtectionOnlineCommittee(committee ...account.SeatIndex) options.Option[NodeState] {
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

func WithChainManagerIsSolid() options.Option[NodeState] {
	return func(state *NodeState) {
		solid := true
		state.chainManagerSolid = &solid
	}
}
