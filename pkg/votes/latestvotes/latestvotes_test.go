package latestvotes

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/iotaledger/hive.go/constraints"
	"github.com/iotaledger/hive.go/ds/thresholdmap"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestLatestVotes(t *testing.T) {
	voter := iotago.AccountID{}

	{
		latestVotes := NewLatestVotes[int, MockedVotePower](voter)
		latestVotes.Store(1, MockedVotePower{VotePower: 8})
		validateLatestVotes(t, latestVotes, map[int]MockedVotePower{
			1: {VotePower: 8},
		})
		latestVotes.Store(2, MockedVotePower{VotePower: 10})
		validateLatestVotes(t, latestVotes, map[int]MockedVotePower{
			2: {VotePower: 10},
		})
		latestVotes.Store(3, MockedVotePower{VotePower: 7})
		validateLatestVotes(t, latestVotes, map[int]MockedVotePower{
			2: {VotePower: 10},
			3: {VotePower: 7},
		})
		latestVotes.Store(4, MockedVotePower{VotePower: 9})
		validateLatestVotes(t, latestVotes, map[int]MockedVotePower{
			2: {VotePower: 10},
			4: {VotePower: 9},
		})
		latestVotes.Store(4, MockedVotePower{VotePower: 11})
		validateLatestVotes(t, latestVotes, map[int]MockedVotePower{
			4: {VotePower: 11},
		})
		latestVotes.Store(1, MockedVotePower{VotePower: 15})
		validateLatestVotes(t, latestVotes, map[int]MockedVotePower{
			1: {VotePower: 15},
			4: {VotePower: 11},
		})
	}

	{
		latestVotes := NewLatestVotes[int, MockedVotePower](voter)
		latestVotes.Store(3, MockedVotePower{VotePower: 7})
		latestVotes.Store(2, MockedVotePower{VotePower: 10})
		latestVotes.Store(4, MockedVotePower{VotePower: 9})
		latestVotes.Store(1, MockedVotePower{VotePower: 8})
		latestVotes.Store(1, MockedVotePower{VotePower: 15})
		latestVotes.Store(4, MockedVotePower{VotePower: 11})
		validateLatestVotes(t, latestVotes, map[int]MockedVotePower{
			1: {VotePower: 15},
			4: {VotePower: 11},
		})
	}
}

func validateLatestVotes[VotePowerType constraints.Comparable[VotePowerType]](t *testing.T, votes *LatestVotes[int, VotePowerType], expectedVotes map[int]VotePowerType) {
	votes.ForEach(func(node *thresholdmap.Element[int, VotePowerType]) bool {
		index := node.Key()
		votePower := node.Value()

		expectedVotePower, exists := expectedVotes[index]
		assert.Truef(t, exists, "%s does not exist in latestVotes", index)
		delete(expectedVotes, index)

		assert.Equal(t, expectedVotePower, votePower)

		return true
	})
	assert.Empty(t, expectedVotes)
}

type MockedVotePower struct {
	VotePower int
}

func (p MockedVotePower) Compare(other MockedVotePower) int {
	if p.VotePower-other.VotePower < 0 {
		return -1
	} else if p.VotePower-other.VotePower > 0 {
		return 1
	} else {
		return 0
	}
}
