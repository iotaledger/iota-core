package spenddag

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/iota-core/pkg/core/acceptance"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/core/vote"
)

type SpendDAG[SpenderID, ResourceID IDType, VoteRank VoteRankType[VoteRank]] interface {
	Shutdown()
	Events() *Events[SpenderID, ResourceID]

	CreateSpender(id SpenderID)
	UpdateSpentResources(id SpenderID, resourceIDs ds.Set[ResourceID]) error

	ReadConsistent(callback func(spendDAG ReadLockedSpendDAG[SpenderID, ResourceID, VoteRank]) error) error
	UpdateSpenderParents(spenderID SpenderID, addedParentIDs, removedParentIDs ds.Set[SpenderID]) error
	FutureCone(spenderIDs ds.Set[SpenderID]) (futureCone ds.Set[SpenderID])
	ConflictingSpenders(spenderID SpenderID) (conflictingSpends ds.Set[SpenderID], exists bool)
	CastVotes(vote *vote.Vote[VoteRank], spenderIDs ds.Set[SpenderID]) error
	AcceptanceState(spenderIDs ds.Set[SpenderID]) acceptance.State
	SetAccepted(spenderID SpenderID)
	UnacceptedSpenders(spenderIDs ds.Set[SpenderID]) ds.Set[SpenderID]
	AllSpendsSupported(seat account.SeatIndex, spenderIDs ds.Set[SpenderID]) bool
	EvictSpender(spenderID SpenderID)

	SpendSets(spenderID SpenderID) (spendSetIDs ds.Set[ResourceID], exists bool)
	SpenderParents(spenderID SpenderID) (spenderIDs ds.Set[SpenderID], exists bool)
	SpendSetMembers(spendSetID ResourceID) (spenderIDs ds.Set[SpenderID], exists bool)
	SpenderWeight(spenderID SpenderID) int64
	SpenderChildren(spenderID SpenderID) (spenderIDs ds.Set[SpenderID], exists bool)
	SpenderVoters(spenderID SpenderID) (voters ds.Set[account.SeatIndex])
	LikedInstead(spenderIDs ds.Set[SpenderID]) ds.Set[SpenderID]
}

type ReadLockedSpendDAG[SpenderID, ResourceID IDType, VoteRank VoteRankType[VoteRank]] interface {
	LikedInstead(spenderIDs ds.Set[SpenderID]) ds.Set[SpenderID]
	FutureCone(spenderIDs ds.Set[SpenderID]) (futureCone ds.Set[SpenderID])
	ConflictingSpenders(spenderID SpenderID) (conflictingSpends ds.Set[SpenderID], exists bool)
	AcceptanceState(spenderIDs ds.Set[SpenderID]) acceptance.State
	UnacceptedSpenders(spenderIDs ds.Set[SpenderID]) ds.Set[SpenderID]
}
