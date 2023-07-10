package model

import (
	iotago "github.com/iotaledger/iota.go/v4"
)

// ParentReferences is a map between parent type and block IDs.
type ParentReferences map[iotago.ParentsType]iotago.BlockIDs
