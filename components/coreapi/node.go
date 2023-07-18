package coreapi

//nolint:unparam // we have no error case right now
func info() (*infoResponse, error) {
	cl := deps.Protocol.MainEngineInstance().Clock
	syncStatus := deps.Protocol.SyncManager.SyncStatus()
	metrics := deps.MetricsTracker.NodeMetrics()
	protoParams := deps.Protocol.CurrentAPI().ProtocolParameters()

	protoParamsBytes, err := deps.Protocol.CurrentAPI().JSONEncode(protoParams)
	if err != nil {
		return nil, err
	}

	return &infoResponse{
		Name:     deps.AppInfo.Name,
		Version:  deps.AppInfo.Version,
		IssuerID: deps.BlockIssuer.Account.ID().ToHex(),
		Status: nodeStatus{
			IsHealthy:              syncStatus.NodeSynced,
			ATT:                    cl.Accepted().Time(),
			RATT:                   cl.Accepted().RelativeTime(),
			CTT:                    cl.Confirmed().Time(),
			RCTT:                   cl.Confirmed().RelativeTime(),
			LatestCommittedSlot:    syncStatus.LatestCommitment.Index(),
			FinalizedSlot:          syncStatus.LatestFinalizedSlot,
			LastAcceptedBlockSlot:  syncStatus.LastAcceptedBlockSlot,
			LastConfirmedBlockSlot: syncStatus.LastConfirmedBlockSlot,
			// TODO: fill in pruningSlot
		},
		Metrics: nodeMetrics{
			BlocksPerSecond:          metrics.BlocksPerSecond,
			ConfirmedBlocksPerSecond: metrics.ConfirmedBlocksPerSecond,
			ConfirmedRate:            metrics.ConfirmedRate,
		},
		SupportedProtocolVersions: deps.Protocol.SupportedVersions(),
		ProtocolParameters:        protoParamsBytes,
		Features:                  features,
	}, nil
}
