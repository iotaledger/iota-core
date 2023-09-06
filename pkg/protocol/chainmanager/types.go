package chainmanager

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/stringify"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type ChainID = iotago.CommitmentID

type Fork struct {
	Source               peer.ID
	MainChain            *Chain
	ForkedChain          *Chain
	ForkingPoint         *model.Commitment
	ForkLatestCommitment *model.Commitment
}

func (f *Fork) String() string {
	return stringify.Struct("Fork",
		stringify.NewStructField("Source", f.Source),
		stringify.NewStructField("MainChain", f.MainChain.String()),
		stringify.NewStructField("ForkedChain", f.ForkedChain.String()),
		stringify.NewStructField("ForkingPoint", f.ForkingPoint),
		stringify.NewStructField("ForkLatestCommitment", f.ForkLatestCommitment),
	)
}
