package retainer

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Retainer keeps and resolves all the information needed in the API and INX.
type Retainer struct {
	protocol *protocol.Protocol
}

// the retainer should store the "confirmed" flag of blocks between they got committed and finalized.
// this storage should have buckets and the confirmed info should be stored by simply setting the blockid.

// several intervals to prune => triggered by the pruning manager
//
//	=> the confirmed flag until it got finalized (is this always the same interval?)
//	=> the info about conflicting blocks (maybe 1 - 2 epochs)
//
// maybe also store the orphaned block there as well?
func NewRetainer(protocol *protocol.Protocol) *Retainer {
	return &Retainer{
		protocol: protocol,
	}
}

func (r *Retainer) Block(blockID iotago.BlockID) (*model.Block, error) {
	block, _ := r.protocol.MainEngineInstance().Block(blockID)
	if block == nil {
		return nil, ierrors.Errorf("block not found: %s", blockID.ToHex())
	}

	return block, nil
}

func (r *Retainer) BlockMetadata(blockID iotago.BlockID) (*model.Block, error) {
	return nil, nil
}
