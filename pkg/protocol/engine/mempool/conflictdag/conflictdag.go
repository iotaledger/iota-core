package conflictdag

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/iota-core/pkg/core/acceptance"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/core/vote"
)

type ConflictDAG[ConflictID, ResourceID IDType, VoteRank VoteRankType[VoteRank]] interface {
	Shutdown()
	Events() *Events[ConflictID, ResourceID]

	CreateConflict(id ConflictID)
	UpdateConflictingResources(id ConflictID, resourceIDs ds.Set[ResourceID]) error

	ReadConsistent(callback func(conflictDAG ReadLockedConflictDAG[ConflictID, ResourceID, VoteRank]) error) error
	UpdateConflictParents(conflictID ConflictID, addedParentIDs, removedParentIDs ds.Set[ConflictID]) error
	FutureCone(conflictIDs ds.Set[ConflictID]) (futureCone ds.Set[ConflictID])
	ConflictingConflicts(conflictID ConflictID) (conflictingConflicts ds.Set[ConflictID], exists bool)
	CastVotes(vote *vote.Vote[VoteRank], conflictIDs ds.Set[ConflictID]) error
	AcceptanceState(conflictIDs ds.Set[ConflictID]) acceptance.State
	UnacceptedConflicts(conflictIDs ds.Set[ConflictID]) ds.Set[ConflictID]
	AllConflictsSupported(seat account.SeatIndex, conflictIDs ds.Set[ConflictID]) bool
	EvictConflict(conflictID ConflictID)

	ConflictSets(conflictID ConflictID) (conflictSetIDs ds.Set[ResourceID], exists bool)
	ConflictParents(conflictID ConflictID) (conflictIDs ds.Set[ConflictID], exists bool)
	ConflictSetMembers(conflictSetID ResourceID) (conflictIDs ds.Set[ConflictID], exists bool)
	ConflictWeight(conflictID ConflictID) int64
	ConflictChildren(conflictID ConflictID) (conflictIDs ds.Set[ConflictID], exists bool)
	ConflictVoters(conflictID ConflictID) (voters ds.Set[account.SeatIndex])
	LikedInstead(conflictIDs ds.Set[ConflictID]) ds.Set[ConflictID]
}

type ReadLockedConflictDAG[ConflictID, ResourceID IDType, VoteRank VoteRankType[VoteRank]] interface {
	LikedInstead(conflictIDs ds.Set[ConflictID]) ds.Set[ConflictID]
	FutureCone(conflictIDs ds.Set[ConflictID]) (futureCone ds.Set[ConflictID])
	ConflictingConflicts(conflictID ConflictID) (conflictingConflicts ds.Set[ConflictID], exists bool)
	AcceptanceState(conflictIDs ds.Set[ConflictID]) acceptance.State
	UnacceptedConflicts(conflictIDs ds.Set[ConflictID]) ds.Set[ConflictID]
}
