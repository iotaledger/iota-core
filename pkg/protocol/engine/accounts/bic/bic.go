package bic

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	iotago "github.com/iotaledger/iota.go/v4"
)

type BIC struct {
	*account.Accounts[iotago.AccountID, *iotago.AccountID]
}

func (b BIC) AccountBIC(id iotago.AccountID) (account accounts.Weight, err error) {
	// todo store last updated time
	val, exists := b.Get(id)
	if exists {
		return accounts.Weight{Value: val}, nil
	}
	return accounts.Weight{}, errors.Errorf("account not found: %s", id)
}
