package metricstracker

import (
	"github.com/iotaledger/hive.go/app"
)

// ParametersMetricsTracker contains the definition of the parameters used by Metrics Tracker.
type ParametersMetricsTracker struct {
	// Enabled defines whether the Metrics Tracker plugin is enabled.
	Enabled bool `default:"true" usage:"whether the Metrics Tracker plugin is enabled"`
}

var ParamsMetricsTracker = &ParametersMetricsTracker{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"metricsTracker": ParamsMetricsTracker,
	},
}
