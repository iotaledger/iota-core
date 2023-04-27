package blockissuer

import (
	"time"

	"github.com/iotaledger/hive.go/app"
)

// ParametersBlockIssuer contains the definition of the configuration parameters used by the BlockIssuer component.
type ParametersBlockIssuer struct {
	// Enabled whether the BlockIssuer component is enabled.
	Enabled bool `default:"true" usage:"whether the BlockIssuer component is enabled"`

	// TipSelectionTimeout the timeout for tip selection.
	TipSelectionTimeout time.Duration `default:"10s" usage:"the timeout for tip selection"`

	// TipSelectionRetryInterval the interval for retrying tip selection.
	TipSelectionRetryInterval time.Duration `default:"200ms" usage:"the interval for retrying tip selection"`

	// IssuerAccount the account ID of the account that will issue the blocks.
	IssuerAccount string `default:"" usage:"the account ID of the account that will issue the blocks"`

	// PrivateKey the private key of the account that will issue the blocks.
	PrivateKey string `default:"" usage:"the private key of the account that will issue the blocks"`
}

// ParamsBlockIssuer is the default configuration parameters for the BlockIssuer component.
var ParamsBlockIssuer = &ParametersBlockIssuer{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"blockIssuer": ParamsBlockIssuer,
	},
	Masked: []string{"blockIssuer.privateKey"},
}
