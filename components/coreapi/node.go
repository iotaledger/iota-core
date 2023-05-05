package coreapi

//nolint:unparam // we have no error case right now
func info() (*infoResponse, error) {
	cl := deps.Protocol.MainEngineInstance().Clock
	syncStatus := deps.Protocol.SyncManager.SyncStatus()
	metrics := deps.Protocol.MetricsTracker.NodeMetrics()

	return &infoResponse{
		Name:     deps.AppInfo.Name,
		Version:  deps.AppInfo.Version,
		IssuerID: deps.BlockIssuer.Account.ID().ToHex(),
		Status: nodeStatus{
			IsHealthy:            syncStatus.NodeSynced,
			ATT:                  cl.Accepted().Time(),
			RATT:                 cl.Accepted().RelativeTime(),
			CTT:                  cl.Confirmed().Time(),
			RCTT:                 cl.Confirmed().RelativeTime(),
			LatestCommittedSlot:  syncStatus.LatestCommittedSlot,
			FinalizedSlot:        syncStatus.FinalizedSlot,
			LastAcceptedBlockID:  syncStatus.LastAcceptedBlockID.ToHex(),
			LastConfirmedBlockID: syncStatus.LastConfirmedBlockID.ToHex(),
			// TODO: fill in pruningSlot
		},
		Metrics: nodeMetrics{
			BlocksPerSecond:          metrics.BlocksPerSecond,
			ConfirmedBlocksPerSecond: metrics.ConfirmedBlocksPerSecond,
			ConfirmedRate:            metrics.ConfirmedRate,
		},
		SupportedProtocolVersions: deps.Protocol.SupportedVersions(),
		ProtocolParameters:        deps.Protocol.MainEngineInstance().Storage.Settings().ProtocolParameters(),
		Features:                  features,
	}, nil
}
