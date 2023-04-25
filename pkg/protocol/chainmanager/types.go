package chainmanager

import (
	"github.com/iotaledger/hive.go/stringify"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	iotago "github.com/iotaledger/iota.go/v4"
)

type ChainID = iotago.CommitmentID

type Fork struct {
	Source       network.PeerID
	Commitment   *model.Commitment
	ForkingPoint *model.Commitment
}

func (f *Fork) String() string {
	return stringify.Struct("Fork",
		stringify.NewStructField("Source", f.Source),
		stringify.NewStructField("Commitment", f.Commitment),
		stringify.NewStructField("ForkingPoint", f.ForkingPoint),
	)
}
