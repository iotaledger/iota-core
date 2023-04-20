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
	// IgnoreBootstrapped sets whether the Activity component should start issuing activity blocks before the main engine is bootstrapped
	IgnoreBootstrapped bool `default:"false" usage:"whether the Activity component should start issuing activity blocks before the main engine is bootstrapped"`
}

// ParamsActivity
var ParamsActivity = &ParametersActivity{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"activity": ParamsActivity,
	},
}
