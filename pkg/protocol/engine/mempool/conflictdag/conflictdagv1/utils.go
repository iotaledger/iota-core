package conflictdagv1

import (
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/iota-core/pkg/core/weight"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
)

// heaviestConflict returns the largest Conflict from the given Conflicts.
func heaviestConflict[ConflictID, ResourceID conflictdag.IDType, VoterPower conflictdag.VotePowerType[VoterPower]](conflicts *advancedset.AdvancedSet[*Conflict[ConflictID, ResourceID, VoterPower]]) *Conflict[ConflictID, ResourceID, VoterPower] {
	var result *Conflict[ConflictID, ResourceID, VoterPower]
	conflicts.Range(func(conflict *Conflict[ConflictID, ResourceID, VoterPower]) {
		if conflict.Compare(result) == weight.Heavier {
			result = conflict
		}
	})

	return result
}
