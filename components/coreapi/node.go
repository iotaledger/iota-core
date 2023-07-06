package coreapi

import (
	"encoding/json"

	"github.com/iotaledger/iota.go/v4/nodeclient"
)

//nolint:unparam // we have no error case right now
func info() (*nodeclient.InfoResponse, error) {
	cl := deps.Protocol.MainEngineInstance().Clock
	syncStatus := deps.Protocol.SyncManager.SyncStatus()
	metrics := deps.MetricsTracker.NodeMetrics()
	protoParams := deps.Protocol.LatestAPI().ProtocolParameters()

	protoParamsBytes, err := deps.Protocol.LatestAPI().JSONEncode(protoParams)
	if err != nil {
		return nil, err
	}
	protoParamsJSONRaw := json.RawMessage(protoParamsBytes)

	return &nodeclient.InfoResponse{
		Name:     deps.AppInfo.Name,
		Version:  deps.AppInfo.Version,
		IssuerID: deps.BlockIssuer.Account.ID().ToHex(),
		Status: &nodeclient.InfoResNodeStatus{
			IsHealthy:            syncStatus.NodeSynced,
			ATT:                  uint64(cl.Accepted().Time().UnixNano()),
			RATT:                 uint64(cl.Accepted().RelativeTime().UnixNano()),
			CTT:                  uint64(cl.Confirmed().Time().UnixNano()),
			RCTT:                 uint64(cl.Confirmed().RelativeTime().UnixNano()),
			LatestCommittedSlot:  syncStatus.LatestCommittedSlot,
			FinalizedSlot:        syncStatus.FinalizedSlot,
			LastAcceptedBlockID:  syncStatus.LastAcceptedBlockID.ToHex(),
			LastConfirmedBlockID: syncStatus.LastConfirmedBlockID.ToHex(),
			// TODO: fill in pruningSlot
		},
		Metrics: &nodeclient.InfoResNodeMetrics{
			BlocksPerSecond:          metrics.BlocksPerSecond,
			ConfirmedBlocksPerSecond: metrics.ConfirmedBlocksPerSecond,
			ConfirmedRate:            metrics.ConfirmedRate,
		},
		SupportedProtocolVersions: deps.Protocol.SupportedVersions(),
		ProtocolParameters:        &protoParamsJSONRaw,
		Features:                  features,
	}, nil
}
