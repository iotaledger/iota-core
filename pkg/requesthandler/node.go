package requesthandler

import (
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func (r *RequestHandler) APIProvider() iotago.APIProvider {
	return r.protocol.Engines.Main.Get().Storage.Settings().APIProvider()
}

func (r *RequestHandler) IsNodeSynced() bool {
	return r.protocol.Engines.Main.Get().SyncManager.IsNodeSynced()
}

func (r *RequestHandler) IsNetworkHealthy() bool {
	return r.protocol.Engines.Main.Get().SyncManager.IsNodeSynced() && !r.protocol.Engines.Main.Get().SyncManager.IsFinalizationDelayed()
}

func (r *RequestHandler) GetProtocolParameters() []*api.InfoResProtocolParameters {
	protoParams := make([]*api.InfoResProtocolParameters, 0)
	provider := r.protocol.Engines.Main.Get().Storage.Settings().APIProvider()
	for _, version := range provider.ProtocolEpochVersions() {
		protocolParams := provider.ProtocolParameters(version.Version)
		if protocolParams == nil {
			continue
		}

		protoParams = append(protoParams, &api.InfoResProtocolParameters{
			StartEpoch: version.StartEpoch,
			Parameters: protocolParams,
		})
	}

	return protoParams
}

func (r *RequestHandler) GetNodeStatus() *api.InfoResNodeStatus {
	clSnapshot := r.protocol.Engines.Main.Get().Clock.Snapshot()
	syncStatus := r.protocol.Engines.Main.Get().SyncManager.SyncStatus()

	return &api.InfoResNodeStatus{
		IsHealthy:                   syncStatus.NodeSynced,
		IsNetworkHealthy:            syncStatus.NodeSynced && !syncStatus.FinalizationDelayed,
		AcceptedTangleTime:          clSnapshot.AcceptedTime,
		RelativeAcceptedTangleTime:  clSnapshot.RelativeAcceptedTime,
		ConfirmedTangleTime:         clSnapshot.ConfirmedTime,
		RelativeConfirmedTangleTime: clSnapshot.RelativeConfirmedTime,
		LatestCommitmentID:          syncStatus.LatestCommitment.ID(),
		LatestFinalizedSlot:         syncStatus.LatestFinalizedSlot,
		LatestAcceptedBlockSlot:     syncStatus.LastAcceptedBlockSlot,
		LatestConfirmedBlockSlot:    syncStatus.LastConfirmedBlockSlot,
		PruningEpoch:                syncStatus.LastPrunedEpoch,
	}
}
