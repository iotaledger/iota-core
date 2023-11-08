package spenddag

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/iota-core/pkg/core/acceptance"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/core/vote"
)

type SpendDAG[SpendID, ResourceID IDType, VoteRank VoteRankType[VoteRank]] interface {
	Shutdown()
	Events() *Events[SpendID, ResourceID]

	CreateSpend(id SpendID)
	UpdateConflictingResources(id SpendID, resourceIDs ds.Set[ResourceID]) error

	ReadConsistent(callback func(spendDAG ReadLockedSpendDAG[SpendID, ResourceID, VoteRank]) error) error
	UpdateSpendParents(spendID SpendID, addedParentIDs, removedParentIDs ds.Set[SpendID]) error
	FutureCone(spendIDs ds.Set[SpendID]) (futureCone ds.Set[SpendID])
	ConflictingSpends(spendID SpendID) (conflictingSpends ds.Set[SpendID], exists bool)
	CastVotes(vote *vote.Vote[VoteRank], spendIDs ds.Set[SpendID]) error
	AcceptanceState(spendIDs ds.Set[SpendID]) acceptance.State
	SetAccepted(spendID SpendID)
	UnacceptedSpends(spendIDs ds.Set[SpendID]) ds.Set[SpendID]
	AllSpendsSupported(seat account.SeatIndex, spendIDs ds.Set[SpendID]) bool
	EvictSpend(spendID SpendID)

	ConflictSets(spendID SpendID) (conflictSetIDs ds.Set[ResourceID], exists bool)
	SpendParents(spendID SpendID) (spendIDs ds.Set[SpendID], exists bool)
	ConflictSetMembers(conflictSetID ResourceID) (spendIDs ds.Set[SpendID], exists bool)
	SpendWeight(spendID SpendID) int64
	SpendChildren(spendID SpendID) (spendIDs ds.Set[SpendID], exists bool)
	SpendVoters(spendID SpendID) (voters ds.Set[account.SeatIndex])
	LikedInstead(spendIDs ds.Set[SpendID]) ds.Set[SpendID]
}

type ReadLockedSpendDAG[SpendID, ResourceID IDType, VoteRank VoteRankType[VoteRank]] interface {
	LikedInstead(spendIDs ds.Set[SpendID]) ds.Set[SpendID]
	FutureCone(spendIDs ds.Set[SpendID]) (futureCone ds.Set[SpendID])
	ConflictingSpends(spendID SpendID) (conflictingConflicts ds.Set[SpendID], exists bool)
	AcceptanceState(spendIDs ds.Set[SpendID]) acceptance.State
	UnacceptedSpends(spendIDs ds.Set[SpendID]) ds.Set[SpendID]
}
