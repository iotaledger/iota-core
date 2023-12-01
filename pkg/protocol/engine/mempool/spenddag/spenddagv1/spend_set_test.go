package spenddagv1

import (
	"github.com/iotaledger/iota-core/pkg/core/vote"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TestSpendSet = *SpendSet[iotago.TransactionID, iotago.OutputID, vote.MockedRank]

var NewTestSpendSet = NewSpendSet[iotago.TransactionID, iotago.OutputID, vote.MockedRank]
