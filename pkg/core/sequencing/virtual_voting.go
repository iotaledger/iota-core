package sequencing

import (
	"fmt"

	"github.com/iotaledger/hive.go/lo"
)

type VirtualVoting struct {
	Block                     *Block
	Committee                 map[Identifier]bool
	LatestMilestoneVotes      map[Identifier]*Block
	LatestMilestoneCandidates map[Identifier]*Block
	Round                     int
}

func NewVirtualVoting(block *Block) *VirtualVoting {
	return &VirtualVoting{
		Block:                     block,
		Committee:                 make(map[Identifier]bool),
		LatestMilestoneCandidates: make(map[Identifier]*Block),
		LatestMilestoneVotes:      make(map[Identifier]*Block),
	}
}

func (v *VirtualVoting) AddValidator(validatorID Identifier) {
	v.Committee[validatorID] = true
	v.LatestMilestoneVotes[validatorID] = v.Block
	v.LatestMilestoneCandidates[validatorID] = v.Block
}

func (v *VirtualVoting) ProcessVote(validatorID Identifier, parents ...*Block) {
	for _, parent := range parents {
		lo.MergeMaps(v.Committee, parent.VirtualVoting.Committee)

		for currentValidatorID, latestMilestoneCandidate := range parent.VirtualVoting.LatestMilestoneCandidates {
			v.LatestMilestoneCandidates[currentValidatorID] = maxBlock(v.LatestMilestoneCandidates[currentValidatorID], latestMilestoneCandidate)
			v.Round = max(v.Round, latestMilestoneCandidate.VirtualVoting.Round)
		}
	}

	for _, parent := range parents {
		if parent.VirtualVoting.Round == v.Round {
			for seenValidatorID, block := range parent.VirtualVoting.LatestMilestoneVotes {
				v.LatestMilestoneVotes[seenValidatorID] = maxBlock(v.LatestMilestoneVotes[seenValidatorID], block)
			}
		}
	}

	if _, isValidator := v.Committee[validatorID]; isValidator {
		if v.LatestMilestoneCandidates[validatorID].VirtualVoting.Round == v.Round && v.seenSuperMajorityOfVotes() {
			v.startNewRound(validatorID)
		} else if v.LatestMilestoneCandidates[validatorID].VirtualVoting.Round < v.Round {
			v.voteInCurrentRound(validatorID)
		}
	}
}

func (v *VirtualVoting) seenSuperMajorityOfVotes() bool {
	onlineCommitteeSize := 0
	collectedVotesCount := 0

	for validatorID, isOnline := range v.Committee {
		if isOnline {
			onlineCommitteeSize++
		}

		if v.LatestMilestoneCandidates[validatorID].VirtualVoting.Round == v.Round {
			collectedVotesCount++
		}
	}

	return collectedVotesCount >= onlineCommitteeSize*2/3
}

func (v *VirtualVoting) startNewRound(validatorID Identifier) {
	votesInNewRound := make(map[Identifier]*Block)
	votesInNewRound[validatorID] = v.winningCandidate()

	v.Round++
	v.LatestMilestoneVotes = votesInNewRound
	v.LatestMilestoneCandidates[validatorID] = v.Block
}

func (v *VirtualVoting) voteInCurrentRound(validatorID Identifier) {
	v.LatestMilestoneVotes[validatorID] = v.winningCandidate()
	v.LatestMilestoneCandidates[validatorID] = v.Block
}

func (v *VirtualVoting) winningCandidate() (winningBlock *Block) {
	votesPerCandidate, maxVotesPerCandidate := make(map[*Block]int), 0
	for _, block := range v.LatestMilestoneVotes {
		votesPerCandidate[block]++

		maxVotesPerCandidate = max(maxVotesPerCandidate, votesPerCandidate[block])
	}

	for block, votes := range votesPerCandidate {
		if votes == maxVotesPerCandidate && (winningBlock == nil || block.Hash > winningBlock.Hash) {
			winningBlock = block
		}
	}

	if winningBlock != nil && maxVotesPerCandidate >= len(v.Committee)*2/3 {
		fmt.Println(winningBlock.ID, "won with", maxVotesPerCandidate, "votes")
	}

	var winningCandidate *Block
	for _, candidate := range v.LatestMilestoneCandidates {
		if candidate.VirtualVoting.LatestMilestoneVotes[candidate.VirtualVoting.Block.Issuer] == winningBlock {
			winningCandidate = maxBlock(winningCandidate, candidate)
		}
	}

	return winningCandidate
}

type Identifier string
