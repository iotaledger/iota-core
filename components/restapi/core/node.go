package core

import "github.com/iotaledger/iota.go/v4/api"

func protocolParameters() []*api.InfoResProtocolParameters {
	protoParams := make([]*api.InfoResProtocolParameters, 0)
	provider := deps.Protocol.MainEngineInstance().Storage.Settings().APIProvider()
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

func info() *api.InfoResponse {
	clSnapshot := deps.Protocol.MainEngineInstance().Clock.Snapshot()
	syncStatus := deps.Protocol.MainEngineInstance().SyncManager.SyncStatus()
	metrics := deps.MetricsTracker.NodeMetrics()

	return &api.InfoResponse{
		Name:    deps.AppInfo.Name,
		Version: deps.AppInfo.Version,
		Status: &api.InfoResNodeStatus{
			IsHealthy:                   syncStatus.NodeSynced,
			AcceptedTangleTime:          clSnapshot.AcceptedTime,
			RelativeAcceptedTangleTime:  clSnapshot.RelativeAcceptedTime,
			ConfirmedTangleTime:         clSnapshot.ConfirmedTime,
			RelativeConfirmedTangleTime: clSnapshot.RelativeConfirmedTime,
			LatestCommitmentID:          syncStatus.LatestCommitment.ID(),
			LatestFinalizedSlot:         syncStatus.LatestFinalizedSlot,
			LatestAcceptedBlockSlot:     syncStatus.LastAcceptedBlockSlot,
			LatestConfirmedBlockSlot:    syncStatus.LastConfirmedBlockSlot,
			PruningEpoch:                syncStatus.LastPrunedEpoch,
		},
		Metrics: &api.InfoResNodeMetrics{
			BlocksPerSecond:          metrics.BlocksPerSecond,
			ConfirmedBlocksPerSecond: metrics.ConfirmedBlocksPerSecond,
			ConfirmationRate:         metrics.ConfirmedRate,
		},
		ProtocolParameters: protocolParameters(),
		BaseToken: &api.InfoResBaseToken{
			Name:         deps.BaseToken.Name,
			TickerSymbol: deps.BaseToken.TickerSymbol,
			Unit:         deps.BaseToken.Unit,
			Subunit:      deps.BaseToken.Subunit,
			Decimals:     deps.BaseToken.Decimals,
		},
		Features: features,
	}
}
