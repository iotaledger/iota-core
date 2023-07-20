package retainer

import "github.com/iotaledger/iota.go/v4/nodeclient/apimodels"

type BlockMetadata struct {
	BlockStatus       apimodels.BlockState
	BlockReason       int
	HasTx             bool
	TransactionStatus apimodels.TransactionState
	TransactionReason int
}
