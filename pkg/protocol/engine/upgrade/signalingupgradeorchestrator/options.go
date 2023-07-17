package signalingupgradeorchestrator

import (
	"github.com/iotaledger/hive.go/runtime/options"
	iotago "github.com/iotaledger/iota.go/v4"
)

func WithProtocolParameters(protocolParameters ...iotago.ProtocolParameters) options.Option[Orchestrator] {
	return func(o *Orchestrator) {
		o.optsProtocolParameters = protocolParameters
	}
}
