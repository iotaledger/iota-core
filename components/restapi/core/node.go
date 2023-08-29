package core

import (
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

func protocolParameters() []*apimodels.InfoResProtocolParameters {
	protoParams := make([]*apimodels.InfoResProtocolParameters, 0)
	provider := deps.Protocol.MainEngineInstance().Storage.Settings().APIProvider()
	for _, version := range provider.ProtocolEpochVersions() {
		protocolParams := provider.ProtocolParameters(version.Version)
		if protocolParams == nil {
			continue
		}

		protoParams = append(protoParams, &apimodels.InfoResProtocolParameters{
			StartEpoch: version.StartEpoch,
			Parameters: protocolParams,
		})
	}

	return protoParams
}

func info() *apimodels.InfoResponse {
	clSnapshot := deps.Protocol.MainEngineInstance().Clock.Snapshot()
	syncStatus := deps.Protocol.SyncManager.SyncStatus()
	metrics := deps.MetricsTracker.NodeMetrics()

	return &apimodels.InfoResponse{
		Name:    deps.AppInfo.Name,
		Version: deps.AppInfo.Version,
		Status: &apimodels.InfoResNodeStatus{
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
		Metrics: &apimodels.InfoResNodeMetrics{
			BlocksPerSecond:          metrics.BlocksPerSecond,
			ConfirmedBlocksPerSecond: metrics.ConfirmedBlocksPerSecond,
			ConfirmationRate:         metrics.ConfirmedRate,
		},
		ProtocolParameters: protocolParameters(),
		BaseToken: &apimodels.InfoResBaseToken{
			Name:            deps.BaseToken.Name,
			TickerSymbol:    deps.BaseToken.TickerSymbol,
			Unit:            deps.BaseToken.Unit,
			Subunit:         deps.BaseToken.Subunit,
			Decimals:        deps.BaseToken.Decimals,
			UseMetricPrefix: deps.BaseToken.UseMetricPrefix,
		},
		Features: features,
	}
}
