package spenddagv1

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/iota-core/pkg/core/weight"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag"
)

// heaviestSpend returns the largest Spend from the given Spends.
func heaviestSpend[SpendID, ResourceID spenddag.IDType, VoterPower spenddag.VoteRankType[VoterPower]](spends ds.Set[*Spend[SpendID, ResourceID, VoterPower]]) *Spend[SpendID, ResourceID, VoterPower] {
	var result *Spend[SpendID, ResourceID, VoterPower]
	spends.Range(func(spend *Spend[SpendID, ResourceID, VoterPower]) {
		if spend.Compare(result) == weight.Heavier {
			result = spend
		}
	})

	return result
}
