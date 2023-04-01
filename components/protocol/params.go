package protocol

import "github.com/iotaledger/hive.go/app"

// ParametersProtocol
type ParametersProtocol struct {
}

// ParamsProtocol
var ParamsProtocol = &ParametersProtocol{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"protocol": ParamsProtocol,
	},
}
