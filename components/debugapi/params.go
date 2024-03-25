package debugapi

import (
	"github.com/iotaledger/hive.go/app"
)

// ParametersDebugAPI contains the definition of configuration parameters used by the debug API.
type ParametersDebugAPI struct {
	// Enabled whether the DebugAPI component is enabled.
	Enabled bool `default:"false" usage:"whether the DebugAPI component is enabled"`

	Database struct {
		Path        string `default:"testnet/debug" usage:"the path to the database folder"`
		MaxOpenDBs  int    `default:"2" usage:"maximum number of open database instances"`
		Granularity int64  `default:"100" usage:"how many slots should be contained in a single DB instance"`
		Pruning     struct {
			Threshold uint64 `default:"1" usage:"how many epochs should be retained"`
		}
	} `name:"db"`
}

// ParamsDebugAPI is the default configuration parameters for the DebugAPI component.
var ParamsDebugAPI = &ParametersDebugAPI{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"debugAPI": ParamsDebugAPI,
	},
}
