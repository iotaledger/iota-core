package blockissuer

import (
	"time"

	"github.com/iotaledger/hive.go/app"
)

// ParametersBlockIssuer contains the definition of the configuration parameters used by the BlockIssuer component.
type ParametersBlockIssuer struct {
	Enabled bool `default:"true" usage:"whether the Activity component is enabled"`

	TipSelectionTimeout time.Duration `default:"10s" usage:"the timeout for tip selection"`

	TipSelectionRetryInterval time.Duration `default:"200ms" usage:"the interval at which the node will broadcast its activity block"`

	IssuerAccount string `default:"" usage:"the number of parents that node will choose for its activity blocks"`

	PrivateKey string `default:"" usage:"the number of parents that node will choose for its activity blocks"`
}

// ParamsBlockIssuer is the default configuration parameters for the BlockIssuer component.
var ParamsBlockIssuer = &ParametersBlockIssuer{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"blockIssuer": ParamsBlockIssuer,
	},
	Masked: []string{"blockIssuer.privateKey"},
}
