package core

import (
	"encoding/json"

	"github.com/iotaledger/iota.go/v4/nodeclient/models"
)

//nolint:unparam // we have no error case right now
func info() (*models.InfoResponse, error) {
	// TODO: maybe add a clock snapshot here to have consistent info in the response?
	cl := deps.Protocol.MainEngineInstance().Clock
	syncStatus := deps.Protocol.SyncManager.SyncStatus()
	metrics := deps.MetricsTracker.NodeMetrics()
	protoParams := deps.Protocol.LatestAPI().ProtocolParameters()

	protoParamsBytes, err := deps.Protocol.LatestAPI().JSONEncode(protoParams)
	if err != nil {
		return nil, err
	}
	protoParamsBytesRaw := json.RawMessage(protoParamsBytes)

	return &models.InfoResponse{
		Name:    deps.AppInfo.Name,
		Version: deps.AppInfo.Version,
		Status: &models.InfoResNodeStatus{
			IsHealthy:                   syncStatus.NodeSynced,
			AcceptedTangleTime:          uint64(cl.Accepted().Time().Unix()),
			RelativeAcceptedTangleTime:  uint64(cl.Accepted().RelativeTime().Unix()),
			ConfirmedTangleTime:         uint64(cl.Confirmed().Time().Unix()),
			RelativeConfirmedTangleTime: uint64(cl.Confirmed().RelativeTime().Unix()),
			// TODO: fill in pruningSlot
			LatestCommittedSlot:    syncStatus.LatestCommittedSlot,
			LatestFinalizedSlot:    syncStatus.FinalizedSlot,
			PruningSlot:            0,
			LatestAcceptedBlockID:  syncStatus.LastAcceptedBlockID.ToHex(),
			LatestConfirmedBlockID: syncStatus.LastConfirmedBlockID.ToHex(),
		},
		Metrics: &models.InfoResNodeMetrics{
			BlocksPerSecond:          metrics.BlocksPerSecond,
			ConfirmedBlocksPerSecond: metrics.ConfirmedBlocksPerSecond,
			ConfirmationRate:         metrics.ConfirmedRate,
		},
		SupportedProtocolVersions: deps.Protocol.SupportedVersions(),
		ProtocolParameters:        &protoParamsBytesRaw,
		Features:                  features,
	}, nil
}
