package sequencing

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/reactive"
)

// Vote represents a struct that holds the information about a block vote.
type Vote struct {
	// Block is the block that is casting the vote.
	Block *Block

	// Committee is the committee that is voting on the block.
	Committee ds.Set[IssuerID]

	// VotesForPreviousRound is a map of votes that were cast on the previous round.
	VotesForPreviousRound map[IssuerID]*Vote

	// LastSeenVotes is a map of the last seen votes for each validator.
	LastSeenVotes map[IssuerID]*Vote

	// Round is the round of the vote.
	Round int

	// IsMilestoneCandidate is triggered when the vote becomes a milestone candidate.
	IsMilestoneCandidate reactive.Event

	// IsMilestone is triggered when the vote is accepted as a milestone.
	IsMilestone reactive.Event
}

func NewVote() *Vote {
	return &Vote{
		IsMilestoneCandidate: reactive.NewEvent(),
		IsMilestone:          reactive.NewEvent(),
	}
}

// FromBlockContext initializes the vote with information from the Block DAG context.
func (v *Vote) FromBlockContext(block *Block, parents ...*Block) {
	v.Block = block
	v.Committee = ds.NewSet[IssuerID]()
	v.VotesForPreviousRound = make(map[IssuerID]*Vote)
	v.LastSeenVotes = make(map[IssuerID]*Vote)

	// inherit the committee and seen votes from the parents
	for _, parentBlock := range parents {
		parentVote := parentBlock.Vote

		// inherit committee from parents
		v.Committee.AddAll(parentVote.Committee)

		for validatorID, lastSeenVote := range parentVote.LastSeenVotes {
			// if the last seen vote is from a newer round, we need to advance our round and clear the votes
			if lastSeenVote.Round > v.Round {
				v.Round, v.VotesForPreviousRound = lastSeenVote.Round, make(map[IssuerID]*Vote)
			}

			// we need to keep track of the last seen vote for each validator
			v.LastSeenVotes[validatorID] = maxVote(v.LastSeenVotes[validatorID], lastSeenVote)
		}

		// if the parent is in the same round as us, we inherit the votes
		if parentVote.Round == v.Round {
			for validatorID, latestVote := range parentVote.VotesForPreviousRound {
				v.VotesForPreviousRound[validatorID] = maxVote(v.VotesForPreviousRound[validatorID], latestVote)
			}
		}
	}

	// cast our own vote or finalize a round if we are part of the committee
	if v.Committee.Has(block.Issuer) {
		if _, hasVoted := v.VotesForPreviousRound[block.Issuer]; !hasVoted {
			v.castVote()
		} else if hasVoted && len(v.VotesForPreviousRound) > v.Committee.Size()*2/3 {
			v.finalizeRound()
		}
	}
}

// FromGenesisBlock initializes the vote with the given genesis block and committee.
func (v *Vote) FromGenesisBlock(block *Block, committee ds.Set[IssuerID]) {
	v.Block = block
	v.Committee = committee
	v.VotesForPreviousRound = make(map[IssuerID]*Vote)
	v.LastSeenVotes = make(map[IssuerID]*Vote)

	// mark the genesis block as the initial vote of each validator
	committee.Range(func(validatorID IssuerID) {
		v.VotesForPreviousRound[validatorID] = nil
		v.LastSeenVotes[validatorID] = v
	})
}

// Target returns the target of the vote (the previous vote in a chain of votes).
func (v *Vote) Target() *Vote {
	return v.VotesForPreviousRound[v.Block.Issuer]
}

// Weight returns the weight of the block in the current round (it cycles through the weights using round-robin).
func (v *Vote) Weight() int {
	return (int(v.Block.Hash) + v.Round) % v.Committee.Size()
}

// castVote casts a vote for the previous round.
func (v *Vote) castVote() {
	// cast vote for the previous round
	v.VotesForPreviousRound[v.Block.Issuer] = v.leaderOfPreviousRound()
	v.LastSeenVotes[v.Block.Issuer] = v

	// trigger milestone candidate event
	v.IsMilestoneCandidate.Trigger()
}

// finalizeRound finalizes the round and casts a vote in the new round (on the heaviest vote from the previous round).
func (v *Vote) finalizeRound() {
	// leader of the current round becomes the vote for the new round
	leaderOfCurrentRound := v.leaderOfCurrentRound()

	// increment the round and reset votes to our initial vote for the new round
	v.Round++
	v.VotesForPreviousRound = map[IssuerID]*Vote{v.Block.Issuer: leaderOfCurrentRound}
	v.LastSeenVotes[v.Block.Issuer] = v

	// trigger milestone candidate event
	v.IsMilestoneCandidate.Trigger()
}

// leaderOfPreviousRound determines the current leader of the previous round.
func (v *Vote) leaderOfPreviousRound() *Vote {
	var heaviestVote *Vote
	var majorityCount int

	// iterate over collected votes and determine the heaviest one (cast by most validators)
	countedVotes := make(map[*Vote]int)
	for _, vote := range v.VotesForPreviousRound {
		if countedVotes[vote]++; countedVotes[vote] > majorityCount {
			heaviestVote = vote

			majorityCount = countedVotes[vote]
		} else if countedVotes[vote] == majorityCount {
			heaviestVote = maxVote(heaviestVote, vote)
		}
	}

	// trigger acceptance if we have a majority
	if heaviestVote != nil && majorityCount > v.Committee.Size()*2/3 {
		heaviestVote.IsMilestone.Trigger()
	}

	return heaviestVote
}

// leaderOfCurrentRound determines the leader of the current round.
func (v *Vote) leaderOfCurrentRound() *Vote {
	// determine the result of the previous round
	leaderOfPreviousRound := v.leaderOfPreviousRound()

	// vote for the heaviest latest seen vote in the current round that references the leader of the previous round
	var heaviestLatestSeenVote *Vote
	for _, latestSeenVote := range v.LastSeenVotes {
		if latestSeenVote.Target() == leaderOfPreviousRound {
			heaviestLatestSeenVote = maxVote(heaviestLatestSeenVote, latestSeenVote)
		}
	}

	return heaviestLatestSeenVote
}

// maxVote returns the heavier of the two votes (it defines the tie-breakers that are used in the voting process).
func maxVote(a *Vote, b *Vote) (heavierVote *Vote) {
	switch {
	case b == nil:
		return a
	case a == nil:
		return b
	case a.Round > b.Round:
		return a
	case a.Round < b.Round:
		return b
	case a.Weight() > b.Weight():
		return a
	default:
		return b
	}
}
