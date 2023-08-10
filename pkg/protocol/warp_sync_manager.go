package protocol

import (
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
)

type WarpSyncManager struct {
	protocol       *Protocol
	requestedSlots map[uint32]struct{}
}

func NewWarpSyncManager(protocol *Protocol) *WarpSyncManager {
	w := &WarpSyncManager{
		protocol:       protocol,
		requestedSlots: make(map[uint32]struct{}),
	}

	protocol.ChainManager.Events.CommitmentPublished.Hook(func(chainCommitment *chainmanager.ChainCommitment) {
		chainCommitment.IsSolid().OnTrigger(func() {
			chainID := chainCommitment.Chain().ForkingPoint.ID()
			mainEngineInstance := protocol.MainEngineInstance()
			candidateEngineInstance := protocol.CandidateEngineInstance()

			if mainEngineInstance.ChainID() == chainID {
				// process for main chain
			} else if candidateEngineInstance != nil && candidateEngineInstance.ChainID() == chainID {
				// process for candidate chain
			}
		})

	})

	protocol.MainEngineInstance().Events.Notarization.LatestCommitmentUpdated.Hook(func(commitment *model.Commitment) {
		// TODO: CHECK IF THERE ARE ADDITIONAL SOLID COMMITMENTS ON THE SAME CHAIN THAT SHOULD BE WARPSYNCED
	})

	return w
}
