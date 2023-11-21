package spenddagv1

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/iota-core/pkg/core/weight"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag"
)

// heaviestSpender returns the heaviest Spender from the given set of Spenders.
func heaviestSpender[SpenderID, ResourceID spenddag.IDType, VoterPower spenddag.VoteRankType[VoterPower]](spenders ds.Set[*Spender[SpenderID, ResourceID, VoterPower]]) *Spender[SpenderID, ResourceID, VoterPower] {
	var result *Spender[SpenderID, ResourceID, VoterPower]
	spenders.Range(func(spender *Spender[SpenderID, ResourceID, VoterPower]) {
		if spender.Compare(result) == weight.Heavier {
			result = spender
		}
	})

	return result
}
