//nolint:revive //temporary mock full of TODOs
package mockedconflictdag

import (
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/iota-core/pkg/core/acceptance"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	iotago "github.com/iotaledger/iota.go/v4"
)

type ConflictDAG[ConflictID conflictdag.IDType, ResourceID conflictdag.IDType, VotePower conflictdag.VotePowerType[VotePower]] struct {
	events *conflictdag.Events[ConflictID, ResourceID]
}

func New[ConflictID conflictdag.IDType, ResourceID conflictdag.IDType, VotePower conflictdag.VotePowerType[VotePower]]() *ConflictDAG[ConflictID, ResourceID, VotePower] {
	return &ConflictDAG[ConflictID, ResourceID, VotePower]{
		events: conflictdag.NewEvents[ConflictID, ResourceID](),
	}
}

func (c ConflictDAG[ConflictID, ResourceID, VotePower]) Events() *conflictdag.Events[ConflictID, ResourceID] {
	return c.events
}

func (c ConflictDAG[ConflictID, ResourceID, VotePower]) CreateConflict(id ConflictID, parentIDs *advancedset.AdvancedSet[ConflictID], resourceIDs *advancedset.AdvancedSet[ResourceID], initialAcceptanceState acceptance.State) error {
	//TODO implement me
	panic("implement me")
}

func (c ConflictDAG[ConflictID, ResourceID, VotePower]) ReadConsistent(callback func(conflictDAG conflictdag.ReadLockedConflictDAG[ConflictID, ResourceID, VotePower]) error) error {
	//TODO implement me
	panic("implement me")
}

func (c ConflictDAG[ConflictID, ResourceID, VotePower]) JoinConflictSets(conflictID ConflictID, resourceIDs *advancedset.AdvancedSet[ResourceID]) error {
	//TODO implement me
	panic("implement me")
}

func (c ConflictDAG[ConflictID, ResourceID, VotePower]) UpdateConflictParents(conflictID ConflictID, addedParentID ConflictID, removedParentIDs *advancedset.AdvancedSet[ConflictID]) error {
	//TODO implement me
	panic("implement me")
}

func (c ConflictDAG[ConflictID, ResourceID, VotePower]) FutureCone(conflictIDs *advancedset.AdvancedSet[ConflictID]) (futureCone *advancedset.AdvancedSet[ConflictID]) {
	//TODO implement me
	panic("implement me")
}

func (c ConflictDAG[ConflictID, ResourceID, VotePower]) ConflictingConflicts(conflictID ConflictID) (conflictingConflicts *advancedset.AdvancedSet[ConflictID], exists bool) {
	//TODO implement me
	panic("implement me")
}

func (c ConflictDAG[ConflictID, ResourceID, VotePower]) CastVotes(vote *vote.Vote[VotePower], conflictIDs *advancedset.AdvancedSet[ConflictID]) error {
	//TODO implement me
	panic("implement me")
}

func (c ConflictDAG[ConflictID, ResourceID, VotePower]) AcceptanceState(conflictIDs *advancedset.AdvancedSet[ConflictID]) acceptance.State {
	if conflictIDs.IsEmpty() {
		return acceptance.Accepted
	}

	return acceptance.Pending
}

func (c ConflictDAG[ConflictID, ResourceID, VotePower]) UnacceptedConflicts(conflictIDs *advancedset.AdvancedSet[ConflictID]) *advancedset.AdvancedSet[ConflictID] {
	//TODO implement me
	panic("implement me")
}

func (c ConflictDAG[ConflictID, ResourceID, VotePower]) AllConflictsSupported(issuerID iotago.AccountID, conflictIDs *advancedset.AdvancedSet[ConflictID]) bool {
	//TODO implement me
	panic("implement me")
}

func (c ConflictDAG[ConflictID, ResourceID, VotePower]) EvictConflict(conflictID ConflictID) error {
	//TODO implement me
	panic("implement me")
}

func (c ConflictDAG[ConflictID, ResourceID, VotePower]) ConflictSets(conflictID ConflictID) (conflictSetIDs *advancedset.AdvancedSet[ResourceID], exists bool) {
	//TODO implement me
	panic("implement me")
}

func (c ConflictDAG[ConflictID, ResourceID, VotePower]) ConflictParents(conflictID ConflictID) (conflictIDs *advancedset.AdvancedSet[ConflictID], exists bool) {
	//TODO implement me
	panic("implement me")
}

func (c ConflictDAG[ConflictID, ResourceID, VotePower]) ConflictSetMembers(conflictSetID ResourceID) (conflictIDs *advancedset.AdvancedSet[ConflictID], exists bool) {
	//TODO implement me
	panic("implement me")
}

func (c ConflictDAG[ConflictID, ResourceID, VotePower]) ConflictWeight(conflictID ConflictID) int64 {
	//TODO implement me
	panic("implement me")
}

func (c ConflictDAG[ConflictID, ResourceID, VotePower]) ConflictChildren(conflictID ConflictID) (conflictIDs *advancedset.AdvancedSet[ConflictID], exists bool) {
	//TODO implement me
	panic("implement me")
}

func (c ConflictDAG[ConflictID, ResourceID, VotePower]) ConflictVoters(conflictID ConflictID) (voters map[iotago.AccountID]int64) {
	//TODO implement me
	panic("implement me")
}

var _ conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, vote.MockedPower] = new(ConflictDAG[iotago.TransactionID, iotago.OutputID, vote.MockedPower])
