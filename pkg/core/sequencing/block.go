package sequencing

import (
	"fmt"
)

type ValidatorId string

type CandidateVotes map[ValidatorId]*Block

func (c CandidateVotes) Merge(otherVotes CandidateVotes) {
	for validatorID, block := range otherVotes {
		if existingBlock, exists := c[validatorID]; !exists || block.Hash > existingBlock.Hash {
			c[validatorID] = block
		}
	}
}

func (c CandidateVotes) WinningCandidate() (winningBlock *Block) {
	votesPerCandidate, maxVotesPerCandidate := make(map[*Block]int), 0
	for _, block := range c {
		votesPerCandidate[block]++

		maxVotesPerCandidate = max(maxVotesPerCandidate, votesPerCandidate[block])
	}

	for block, votes := range votesPerCandidate {
		if votes == maxVotesPerCandidate && (winningBlock == nil || block.Hash > winningBlock.Hash) {
			winningBlock = block
		}
	}

	return winningBlock
}

type ReferencedRounds map[ValidatorId]int

func (r ReferencedRounds) SuperMajorityReached() bool {
	return len(r) > 2*len(r)/3
}

type ValidatorVotes map[ValidatorId]int

type Block struct {
	IsValidator          *ValidatorId
	Parents              []*Block
	ReferencedRounds     ReferencedRounds
	LatestCandidateVotes CandidateVotes
	Hash                 int
	Round                int
}

func (b *Block) OnSolid() {
	referencedRounds, maxRound := b.referencedParentRounds()
	votes := b.collectParentVotes(maxRound)

	if validatorID := b.IsValidator; validatorID != nil {
		if ownRound, ownRoundExists := referencedRounds[*validatorID]; !ownRoundExists || ownRound < maxRound {
			votes.Add(validatorID, votes.WinningCandidate())

			// TODO: CAST VOTE FOR THIS ROUND
		} else if ownRound == maxRound && referencedRounds.SuperMajorityReached() {
			maxRound++

			votes = map[ValidatorId]*Block{*validatorID: votes.WinningCandidate()}
			// This is a milestone candidate -> aggregate the votes
			// TODO: CAST VOTE FOR THIS ROUND

		}

		referencedRounds[*validatorID] = maxRound
	}

	b.Round = maxRound

	fmt.Println("Max round is", referencedRounds, maxRound)
}

func (b *Block) collectParentVotes(round int) (latestCandidateVotes CandidateVotes) {
	latestCandidateVotes = make(CandidateVotes)

	for _, parent := range b.Parents {
		if parent.Round == round {
			latestCandidateVotes.Merge(parent.LatestCandidateVotes)
		}
	}

	return latestCandidateVotes
}

func (b *Block) referencedParentRounds() (referencedParentRounds ReferencedRounds, maxRound int) {
	referencedParentRounds = make(map[ValidatorId]int)

	for _, parent := range b.Parents {
		for validatorID, round := range parent.ReferencedRounds {
			if currentRound, currentRoundExists := referencedParentRounds[validatorID]; !currentRoundExists || round > currentRound {
				referencedParentRounds[validatorID] = round

				maxRound = max(maxRound, round)
			}
		}
	}

	return referencedParentRounds, maxRound
}
