package prometheus

import (
	"github.com/iotaledger/hive.go/app"
)

// ParametersMetrics contains the definition of the parameters used by the metrics component.
type ParametersMetrics struct {
	// Enabled defines whether the Metrics component is enabled.
	Enabled bool `default:"true" usage:"whether the Metrics component is enabled"`
	// BindAddress defines the bind address for the Prometheus exporter server.
	BindAddress string `default:"0.0.0.0:9311" usage:"bind address on which the Prometheus exporter server"`
	// GoMetrics defines whether to include Go metrics.
	GoMetrics bool `default:"false" usage:"include go metrics"`
	// ProcessMetrics defines whether to include process metrics.
	ProcessMetrics bool `default:"false" usage:"include process metrics"`
	// PromhttpMetrics defines whether to include promhttp metrics.
	PromhttpMetrics bool `default:"false" usage:"include promhttp metrics"`
}

// ParamsMetrics contains the configuration used by the metrics collector plugin.
var ParamsMetrics = &ParametersMetrics{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"prometheus": ParamsMetrics,
	},
}
