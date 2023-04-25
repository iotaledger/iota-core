package chainmanager

import (
	"github.com/iotaledger/iota-core/pkg/network"
	iotago "github.com/iotaledger/iota.go/v4"
)

type ChainID = iotago.CommitmentID

type Fork struct {
	Source       network.PeerID
	Commitment   *iotago.Commitment
	ForkingPoint *iotago.Commitment
}
