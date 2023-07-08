package conflictdag

import (
	"github.com/iotaledger/hive.go/ds/set"
	"github.com/iotaledger/iota-core/pkg/core/acceptance"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/core/vote"
)

type ConflictDAG[ConflictID, ResourceID IDType, VoteRank VoteRankType[VoteRank]] interface {
	Shutdown()
	Events() *Events[ConflictID, ResourceID]

	CreateConflict(id ConflictID)
	UpdateConflictingResources(id ConflictID, resourceIDs set.Set[ResourceID]) error

	ReadConsistent(callback func(conflictDAG ReadLockedConflictDAG[ConflictID, ResourceID, VoteRank]) error) error
	UpdateConflictParents(conflictID ConflictID, addedParentIDs, removedParentIDs set.Set[ConflictID]) error
	FutureCone(conflictIDs set.Set[ConflictID]) (futureCone set.Set[ConflictID])
	ConflictingConflicts(conflictID ConflictID) (conflictingConflicts set.Set[ConflictID], exists bool)
	CastVotes(vote *vote.Vote[VoteRank], conflictIDs set.Set[ConflictID]) error
	AcceptanceState(conflictIDs set.Set[ConflictID]) acceptance.State
	UnacceptedConflicts(conflictIDs set.Set[ConflictID]) set.Set[ConflictID]
	AllConflictsSupported(seat account.SeatIndex, conflictIDs set.Set[ConflictID]) bool
	EvictConflict(conflictID ConflictID)

	ConflictSets(conflictID ConflictID) (conflictSetIDs set.Set[ResourceID], exists bool)
	ConflictParents(conflictID ConflictID) (conflictIDs set.Set[ConflictID], exists bool)
	ConflictSetMembers(conflictSetID ResourceID) (conflictIDs set.Set[ConflictID], exists bool)
	ConflictWeight(conflictID ConflictID) int64
	ConflictChildren(conflictID ConflictID) (conflictIDs set.Set[ConflictID], exists bool)
	ConflictVoters(conflictID ConflictID) (voters set.Set[account.SeatIndex])
	LikedInstead(conflictIDs set.Set[ConflictID]) set.Set[ConflictID]
}

type ReadLockedConflictDAG[ConflictID, ResourceID IDType, VoteRank VoteRankType[VoteRank]] interface {
	LikedInstead(conflictIDs set.Set[ConflictID]) set.Set[ConflictID]
	FutureCone(conflictIDs set.Set[ConflictID]) (futureCone set.Set[ConflictID])
	ConflictingConflicts(conflictID ConflictID) (conflictingConflicts set.Set[ConflictID], exists bool)
	AcceptanceState(conflictIDs set.Set[ConflictID]) acceptance.State
	UnacceptedConflicts(conflictIDs set.Set[ConflictID]) set.Set[ConflictID]
}
