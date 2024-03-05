package restapi

import (
	"github.com/iotaledger/iota-core/pkg/protocol"
)

type RequestHandler struct {
	protocol *protocol.Protocol
}

func NewRequestHandler(p *protocol.Protocol) *RequestHandler {
	return &RequestHandler{
		protocol: p,
	}
}
