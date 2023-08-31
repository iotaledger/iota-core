package validator

import (
	"time"

	"github.com/iotaledger/hive.go/app"
)

// ParametersValidator contains the definition of the configuration parameters used by the Validator component.
type ParametersValidator struct {
	// Enabled defines whether the Validator component is enabled.
	Enabled bool `default:"false" usage:"whether the Validator component is enabled"`
	// CommitteeBroadcastInterval the interval at which the node will broadcast its committee validator block.
	CommitteeBroadcastInterval time.Duration `default:"500ms" usage:"the interval at which the node will broadcast its committee validator block"`
	// CandidateBroadcastInterval the interval at which the node will broadcast its candidate validator block.
	CandidateBroadcastInterval time.Duration `default:"30m" usage:"the interval at which the node will broadcast its candidate validator block"`
	// ParentsCount is the number of parents that node will choose for its validator blocks.
	ParentsCount int `default:"8" usage:"the number of parents that node will choose for its validator blocks"`
	// IgnoreBootstrapped sets whether the Validator component should start issuing validator blocks before the main engine is bootstrapped.
	IgnoreBootstrapped bool `default:"false" usage:"whether the Validator component should start issuing validator blocks before the main engine is bootstrapped"`
	// Account the accountID of the account that will issue the blocks.
	Account string `default:"" usage:"the accountID of the validator account that will issue the blocks"`
	// PrivateKey the private key of the account that will issue the blocks.
	PrivateKey string `default:"" usage:"the private key of the validator account that will issue the blocks"`
}

// ParamsValidator contains the values of the configuration parameters used by the Activity component.
var ParamsValidator = &ParametersValidator{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"validator": ParamsValidator,
	},
}
