package debugapi

import (
	"github.com/iotaledger/hive.go/app"
)

// ParametersDebugAPI contains the definition of configuration parameters used by the debug API.
type ParametersDebugAPI struct {
	// Enabled whether the DebugAPI component is enabled.
	Enabled bool `default:"true" usage:"whether the DebugAPI component is enabled"`

	Path             string `default:"testnet/debug" usage:"the path to the database folder"`
	MaxOpenDBs       int    `default:"2" usage:"maximum number of open database instances"`
	PruningThreshold uint64 `default:"8640" usage:"how many confirmed slots should be retained"`
	DBGranularity    int64  `default:"100" usage:"how many slots should be contained in a single DB instance"`
}

// ParamsDebugAPI is the default configuration parameters for the DebugAPI component.
var ParamsDebugAPI = &ParametersDebugAPI{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"debugAPI": ParamsDebugAPI,
	},
}
