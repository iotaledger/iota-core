package chainmanager

import (
	"github.com/iotaledger/hive.go/crypto/identity"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Fork struct {
	Source       identity.ID
	Commitment   *iotago.Commitment
	ForkingPoint *iotago.Commitment
}
