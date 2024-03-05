package core

import "github.com/iotaledger/iota.go/v4/api"

func info() *api.InfoResponse {
	return &api.InfoResponse{
		Name:               deps.AppInfo.Name,
		Version:            deps.AppInfo.Version,
		Status:             deps.RequestHandler.GetNodeStatus(),
		ProtocolParameters: deps.RequestHandler.GetProtocolParameters(),
		BaseToken: &api.InfoResBaseToken{
			Name:         deps.BaseToken.Name,
			TickerSymbol: deps.BaseToken.TickerSymbol,
			Unit:         deps.BaseToken.Unit,
			Subunit:      deps.BaseToken.Subunit,
			Decimals:     deps.BaseToken.Decimals,
		},
	}
}

func metrics() *api.NetworkMetricsResponse {
	metrics := deps.MetricsTracker.NodeMetrics()
	return &api.NetworkMetricsResponse{
		BlocksPerSecond:          metrics.BlocksPerSecond,
		ConfirmedBlocksPerSecond: metrics.ConfirmedBlocksPerSecond,
		ConfirmationRate:         metrics.ConfirmedRate,
	}
}
