package core

import (
	"encoding/json"

	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

func protocolParameters() ([]*apimodels.InfoResProtocolParameters, error) {
	protoParams := make([]*apimodels.InfoResProtocolParameters, 0)
	provider := deps.Protocol.MainEngineInstance().Storage.Settings().APIProvider()
	for _, version := range provider.ProtocolEpochVersions() {
		protocolParams := provider.ProtocolParameters(version.Version)
		if protocolParams == nil {
			continue
		}

		protoParamsBytes, err := deps.Protocol.CurrentAPI().JSONEncode(protocolParams)
		if err != nil {
			return nil, err
		}
		protoParamsJSONRaw := json.RawMessage(protoParamsBytes)

		protoParams = append(protoParams, &apimodels.InfoResProtocolParameters{
			StartEpoch: version.StartEpoch,
			Parameters: &protoParamsJSONRaw,
		})
	}

	return protoParams, nil
}

func info() (*apimodels.InfoResponse, error) {
	protocolParams, err := protocolParameters()
	if err != nil {
		return nil, err
	}

	clSnapshot := deps.Protocol.MainEngineInstance().Clock.Snapshot()
	syncStatus := deps.Protocol.SyncManager.SyncStatus()
	metrics := deps.MetricsTracker.NodeMetrics()

	return &apimodels.InfoResponse{
		Name:    deps.AppInfo.Name,
		Version: deps.AppInfo.Version,
		Status: &apimodels.InfoResNodeStatus{
			IsHealthy:                   syncStatus.NodeSynced,
			AcceptedTangleTime:          uint64(clSnapshot.AcceptedTime.UnixNano()),
			RelativeAcceptedTangleTime:  uint64(clSnapshot.RelativeAcceptedTime.UnixNano()),
			ConfirmedTangleTime:         uint64(clSnapshot.ConfirmedTime.UnixNano()),
			RelativeConfirmedTangleTime: uint64(clSnapshot.RelativeConfirmedTime.UnixNano()),
			LatestCommitmentID:          syncStatus.LatestCommitment.ID().ToHex(),
			LatestFinalizedSlot:         syncStatus.LatestFinalizedSlot,
			LatestAcceptedBlockSlot:     syncStatus.LastAcceptedBlockSlot,
			LatestConfirmedBlockSlot:    syncStatus.LastConfirmedBlockSlot,
			PruningSlot:                 syncStatus.LatestPrunedSlot,
		},
		Metrics: &apimodels.InfoResNodeMetrics{
			BlocksPerSecond:          metrics.BlocksPerSecond,
			ConfirmedBlocksPerSecond: metrics.ConfirmedBlocksPerSecond,
			ConfirmationRate:         metrics.ConfirmedRate,
		},
		ProtocolParameters: protocolParams,
		BaseToken: &apimodels.InfoResBaseToken{
			Name:            deps.BaseToken.Name,
			TickerSymbol:    deps.BaseToken.TickerSymbol,
			Unit:            deps.BaseToken.Unit,
			Subunit:         deps.BaseToken.Subunit,
			Decimals:        deps.BaseToken.Decimals,
			UseMetricPrefix: deps.BaseToken.UseMetricPrefix,
		},
		Features: features,
	}, nil
}
