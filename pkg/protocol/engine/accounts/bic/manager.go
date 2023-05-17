package bic

import (
	"fmt"
	"sync"

	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	iotago "github.com/iotaledger/iota.go/v4"
)

// BlockIssuanceCredits is a Block Issuer Credits module responsible for tracking account-based mana balances..
type BlockIssuanceCredits struct {
	workers *workerpool.Group
	// balances represents the Block Issuer Credits of all registered accounts, isupdated on the slot commitment.
	balances *account.Accounts[iotago.AccountID, *iotago.AccountID]

	mutex sync.RWMutex

	module.Module
}

func (b *BlockIssuanceCredits) CommitSlot(slotIndex iotago.SlotIndex, allotments map[iotago.AccountID]uint64) (bicRoot iotago.Identifier, err error) {
	// TODO do we need to store the index, if yes should it be in the engine store or should we create new kv store as in the ledger?
	bicIndex, err := b.ReadBICIndex()
	if err != nil {
		return iotago.Identifier{}, err
	}
	if slotIndex != bicIndex+1 {
		panic(fmt.Errorf("there is a gap in the bicstate %d vs %d", bicIndex, slotIndex))
	}

	b.ApplyDiff(allotments)

	return iotago.Identifier{}, nil
}

func (b *BlockIssuanceCredits) BIC() *account.Accounts[iotago.AccountID, *iotago.AccountID] {
	return b.balances
}

func (b *BlockIssuanceCredits) AccountBIC(id iotago.AccountID) (account *accounts.Account, err error) {
	return nil, nil
}

func (b *BlockIssuanceCredits) Shutdown() {
}

func (b *BlockIssuanceCredits) ReadBICIndex() (index iotago.SlotIndex, err error) {
	return 0, nil
}

func (b *BlockIssuanceCredits) ApplyDiff(allotments map[iotago.AccountID]uint64) {

	for accountID, allotmentValue := range allotments {
		current, _ := b.balances.Get(accountID)
		// allotment is always positive, but balance don't need to be
		b.balances.Set(accountID, current+int64(allotmentValue))
	}

}
