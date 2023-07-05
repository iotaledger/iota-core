package vote

import (
	"github.com/iotaledger/hive.go/constraints"
	"github.com/iotaledger/iota-core/pkg/core/account"
)

// Vote represents a vote that is cast by a voter.
type Vote[Rank constraints.Comparable[Rank]] struct {
	// Voter is the identity of the voter.
	Voter account.SeatIndex

	// Rank is the rank of the voter.
	Rank Rank

	// liked is true if the vote is "positive" (voting "for something").
	liked bool
}

// NewVote creates a new vote.
func NewVote[Rank constraints.Comparable[Rank]](voter account.SeatIndex, rank Rank) *Vote[Rank] {
	return &Vote[Rank]{
		Voter: voter,
		Rank:  rank,
		liked: true,
	}
}

// IsLiked returns true if the vote is "positive" (voting "for something").
func (v *Vote[Rank]) IsLiked() bool {
	return v.liked
}

// WithLiked returns a copy of the vote with the given liked value.
func (v *Vote[Rank]) WithLiked(liked bool) *Vote[Rank] {
	updatedVote := new(Vote[Rank])
	updatedVote.Voter = v.Voter
	updatedVote.Rank = v.Rank
	updatedVote.liked = liked

	return updatedVote
}
