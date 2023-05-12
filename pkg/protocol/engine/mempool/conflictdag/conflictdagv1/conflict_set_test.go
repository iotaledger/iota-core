package conflictdagv1

import (
	"github.com/iotaledger/iota-core/pkg/core/vote"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TestConflictSet = *ConflictSet[iotago.TransactionID, iotago.OutputID, vote.MockedPower]

var NewTestConflictSet = NewConflictSet[iotago.TransactionID, iotago.OutputID, vote.MockedPower]
