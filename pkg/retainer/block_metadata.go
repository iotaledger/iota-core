package retainer

import "github.com/iotaledger/iota.go/v4/nodeclient/models"

type BlockMetadata struct {
	BlockStatus       models.BlockState
	BlockReason       int
	HasTx             bool
	TransactionStatus models.TransactionState
	TransactionReason int
}
