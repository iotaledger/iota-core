package protocol

import (
	"fmt"

	"github.com/iotaledger/hive.go/crypto/identity"
	"github.com/iotaledger/iota-core/pkg/model"
)

type Dispatcher struct {
	protocol *Protocol
}

func (p *Dispatcher) IssueBlock(block *model.Block) error {
	fmt.Println("IssueBlock", block)
	p.protocol.MainEngine().ProcessBlockFromPeer(block, identity.ID{})

	return nil
}
