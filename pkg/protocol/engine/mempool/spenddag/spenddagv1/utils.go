package spenddagv1

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/iota-core/pkg/core/weight"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag"
)

// heaviestConflict returns the largest Conflict from the given Conflicts.
func heaviestConflict[SpendID, ResourceID spenddag.IDType, VoterPower spenddag.VoteRankType[VoterPower]](conflicts ds.Set[*Spend[SpendID, ResourceID, VoterPower]]) *Spend[SpendID, ResourceID, VoterPower] {
	var result *Spend[SpendID, ResourceID, VoterPower]
	conflicts.Range(func(conflict *Spend[SpendID, ResourceID, VoterPower]) {
		if conflict.Compare(result) == weight.Heavier {
			result = conflict
		}
	})

	return result
}
