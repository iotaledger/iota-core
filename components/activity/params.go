package activity

import (
	"time"

	"github.com/iotaledger/hive.go/app"
)

// ParametersActivity
type ParametersActivity struct {
	// Enabled defines whether the Activity component is enabled.
	Enabled bool `default:"true" usage:"whether the Activity component is enabled"`
	// BroadcastInterval is the interval at which the node broadcasts its activity block.
	BroadcastInterval time.Duration `default:"2s" usage:"the interval at which the node will broadcast its activity block"`
	// ParentsCount is the number of parents that node will choose for its activity blocks.
	ParentsCount int `default:"8" usage:"the number of parents that node will choose for its activity blocks"`
}

// ParamsActivity
var ParamsActivity = &ParametersActivity{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"activity": ParamsActivity,
	},
}
