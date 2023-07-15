package retainer

import "github.com/iotaledger/iota.go/v4/nodeclient/models"

type BlockMetadata struct {
	Status models.BlockState `serix:"0"`
}
