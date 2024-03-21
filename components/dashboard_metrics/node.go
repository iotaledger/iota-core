package dashboardmetrics

import (
	"runtime"
	"time"
)

var (
	nodeStartupTimestamp = time.Now()
)

func nodeInfoExtended() *NodeInfoExtended {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	status := &NodeInfoExtended{
		Version:       deps.AppInfo.Version,
		LatestVersion: deps.AppInfo.LatestGitHubVersion,
		Uptime:        time.Since(nodeStartupTimestamp).Milliseconds(),
		NodeID:        deps.Host.ID().String(),
		NodeAlias:     deps.AppInfo.Name,
		MemoryUsage:   int64(m.HeapAlloc + m.StackSys + m.MSpanSys + m.MCacheSys + m.BuckHashSys + m.GCSys + m.OtherSys),
	}

	return status
}

func databaseSizesMetrics() *DatabaseSizesMetric {
	return &DatabaseSizesMetric{
		Permanent:  deps.Protocol.Engines.Main.Get().Storage.PermanentDatabaseSize(),
		Prunable:   deps.Protocol.Engines.Main.Get().Storage.PrunableDatabaseSize(),
		TxRetainer: deps.Protocol.Engines.Main.Get().Storage.TransactionRetainerDatabaseSize(),
		Total:      deps.Protocol.Engines.Main.Get().Storage.Size(),
		Time:       time.Now().Unix(),
	}
}
