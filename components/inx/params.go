package inx

import (
	"github.com/iotaledger/hive.go/app"
)

// ParametersINX contains the definition of the parameters used by INX.
type ParametersINX struct {
	// Enabled defines whether the INX plugin is enabled.
	Enabled bool `default:"false" usage:"whether the INX plugin is enabled"`
	// the bind address on which the INX can be accessed from
	BindAddress string `default:"localhost:9029" usage:"the bind address on which the INX can be accessed from"`
}

var ParamsINX = &ParametersINX{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"inx": ParamsINX,
	},
	Masked: nil,
}
