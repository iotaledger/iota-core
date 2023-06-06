package conflictdag

import (
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/iota-core/pkg/core/acceptance"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	iotago "github.com/iotaledger/iota.go/v4"
)

type ConflictDAG[ConflictID, ResourceID IDType, VotePower VotePowerType[VotePower]] interface {
	Shutdown()
	Events() *Events[ConflictID, ResourceID]

	CreateConflict(id ConflictID)
	UpdateConflictingResources(id ConflictID, resourceIDs *advancedset.AdvancedSet[ResourceID]) error

	ReadConsistent(callback func(conflictDAG ReadLockedConflictDAG[ConflictID, ResourceID, VotePower]) error) error
	UpdateConflictParents(conflictID ConflictID, addedParentIDs, removedParentIDs *advancedset.AdvancedSet[ConflictID]) error
	FutureCone(conflictIDs *advancedset.AdvancedSet[ConflictID]) (futureCone *advancedset.AdvancedSet[ConflictID])
	ConflictingConflicts(conflictID ConflictID) (conflictingConflicts *advancedset.AdvancedSet[ConflictID], exists bool)
	CastVotes(vote *vote.Vote[VotePower], conflictIDs *advancedset.AdvancedSet[ConflictID]) error
	AcceptanceState(conflictIDs *advancedset.AdvancedSet[ConflictID]) acceptance.State
	UnacceptedConflicts(conflictIDs *advancedset.AdvancedSet[ConflictID]) *advancedset.AdvancedSet[ConflictID]
	AllConflictsSupported(issuerID iotago.AccountID, conflictIDs *advancedset.AdvancedSet[ConflictID]) bool
	EvictConflict(conflictID ConflictID)

	ConflictSets(conflictID ConflictID) (conflictSetIDs *advancedset.AdvancedSet[ResourceID], exists bool)
	ConflictParents(conflictID ConflictID) (conflictIDs *advancedset.AdvancedSet[ConflictID], exists bool)
	ConflictSetMembers(conflictSetID ResourceID) (conflictIDs *advancedset.AdvancedSet[ConflictID], exists bool)
	ConflictWeight(conflictID ConflictID) int64
	ConflictChildren(conflictID ConflictID) (conflictIDs *advancedset.AdvancedSet[ConflictID], exists bool)
	ConflictVoters(conflictID ConflictID) (voters map[iotago.AccountID]int64)
	LikedInstead(conflictIDs *advancedset.AdvancedSet[ConflictID]) *advancedset.AdvancedSet[ConflictID]
}

type ReadLockedConflictDAG[ConflictID, ResourceID IDType, VotePower VotePowerType[VotePower]] interface {
	LikedInstead(conflictIDs *advancedset.AdvancedSet[ConflictID]) *advancedset.AdvancedSet[ConflictID]
	FutureCone(conflictIDs *advancedset.AdvancedSet[ConflictID]) (futureCone *advancedset.AdvancedSet[ConflictID])
	ConflictingConflicts(conflictID ConflictID) (conflictingConflicts *advancedset.AdvancedSet[ConflictID], exists bool)
	AcceptanceState(conflictIDs *advancedset.AdvancedSet[ConflictID]) acceptance.State
	UnacceptedConflicts(conflictIDs *advancedset.AdvancedSet[ConflictID]) *advancedset.AdvancedSet[ConflictID]
}
