package bic

import (
	"fmt"
	"sync"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Diff stores a changes in bic balances for a given slot.
type Diff struct {
	index      iotago.SlotIndex           `serix:"0"`
	bicChanges map[iotago.AccountID]int64 `serix:"1,lengthPrefixType=uint32,omitempty"`
}

// newBICDiff creates a new BIC diff from the given allotments and burns.
func newBICDiff(index iotago.SlotIndex, allotments map[iotago.AccountID]uint64, burns map[iotago.AccountID]uint64) *Diff {
	bicChanges := make(map[iotago.AccountID]int64)
	for id, allotment := range allotments {
		bicChanges[id] += int64(allotment)
	}
	for id, burn := range burns {
		bicChanges[id] -= int64(burn)
	}

	return &Diff{
		index:      index,
		bicChanges: bicChanges,
	}
}

// BICManager is a Block Issuer Credits module responsible for tracking block issuance credit balances.
type BICManager struct {
	// slot diffs for the BIC between [LatestCommitedSlot - MCA, LatestCommitedSlot]
	slotDiffs *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *Diff]

	// todo abstact BICTree into separate component to hide the diff moving implementation
	store kvstore.KVStore

	// TODO: store the index
	// the slot index of the bic vector ("LatestCommitedSlot")
	bicIndex iotago.SlotIndex

	// TODO on reading from the snapshot: create the BIC tree from the bic vector and the slot diffs
	// bic represents the Block Issuer Credits vector of all registered accounts for bicTreeindex slot, it is updated on the slot commitment.
	bicTree *ads.Map[iotago.AccountID, accounts.AccountImpl, *iotago.AccountID, *accounts.AccountImpl]

	apiProviderFunc func() iotago.API
	mutex           sync.RWMutex

	module.Module
}

// TODO initialise this componen in the ledger manager, keep it behind the interface

func New(store kvstore.KVStore, apiProviderFunc func() iotago.API) *BICManager {
	return &BICManager{
		slotDiffs: shrinkingmap.New[iotago.SlotIndex, *Diff](),
		bicTree:   ads.NewMap[iotago.AccountID, accounts.AccountImpl](lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{StoreKeyPrefixBICTree}))),
	}
}

func (b *BICManager) API() iotago.API {
	return b.apiProviderFunc()
}

func (b *BICManager) BICTreeRoot() iotago.Identifier {
	return iotago.Identifier(b.bicTree.Root())
}

func (b *BICManager) CommitSlot(slotIndex iotago.SlotIndex, allotments map[iotago.AccountID]uint64, burns map[iotago.AccountID]uint64) (bicRoot iotago.Identifier, err error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	diff := newBICDiff(slotIndex, allotments, burns)

	if bicRoot, err = b.applyDiff(diff); err != nil {
		return iotago.Identifier{}, err
	}

	// at this point the bic vector is at bicIndex == slotIndex
	b.removeOldDiff(slotIndex - iotago.MaxCommitableSlotAge - 1)

	return bicRoot, nil
}

func (b *BICManager) BIC(accountID iotago.AccountID, slotIndex iotago.SlotIndex) (accounts.Account, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if slotIndex < b.bicIndex-iotago.MaxCommitableSlotAge {
		return nil, fmt.Errorf("can't calculate BIC, slot index older than bicIndex (%d<%d)", slotIndex, b.bicIndex)
	}
	if slotIndex > b.bicIndex {
		return nil, fmt.Errorf("can't retrieve BIC, slot %d is not committed yet, lastest committed slot: %d", slotIndex, b.bicIndex)
	}
	// read start value from the bic vector at b.bicIndex
	loadedAccount, exists := b.bicTree.Get(accountID)
	if !exists {
		loadedAccount = accounts.NewAccount(b.API(), accountID, accounts.NewCredits(0, slotIndex))
	}
	// last applied diff should be the diff for slotIndex + 1
	for diffIndex := b.bicIndex; diffIndex > slotIndex; diffIndex-- {
		slotDiff, exists := b.slotDiffs.Get(diffIndex)
		if !exists {
			return nil, fmt.Errorf("can't calculate BIC, slot index doesn't exist (%d)", diffIndex)
		}
		if change, exists := slotDiff.bicChanges[accountID]; exists {
			loadedAccount.Credits().Update(-change)
		}
	}
	return loadedAccount, nil

}

func (b *BICManager) Shutdown() {
	// TODO implement shutdown
}

func (b *BICManager) applyDiff(newDiff *Diff) (iotago.Identifier, error) {
	if b.bicIndex+1 != newDiff.index {
		return iotago.Identifier{}, errors.Errorf("cannot apply the ned diff, there is a gap in committed slots, bic vector index: %d, slot to commit: %d", b.bicIndex, newDiff.index)
	}
	// add the new diff to the map
	b.slotDiffs.Set(newDiff.index, newDiff)

	bicRoot, err := b.commitBICTree(newDiff)
	if err != nil {
		return iotago.Identifier{}, errors.Wrapf(err, "could not apply diff to slot %d", newDiff.index)
	}
	// set the new balances index
	b.bicIndex = newDiff.index

	return bicRoot, nil
}

func (b *BICManager) commitBICTree(diff *Diff) (bicRoot iotago.Identifier, err error) {
	// update the bic tree to latestCommitted slot index
	for accountID, valueChange := range diff.bicChanges {
		loadedAccount, exists := b.bicTree.Get(accountID)
		if !exists {
			loadedAccount = accounts.NewAccount(b.API(), accountID, accounts.NewCredits(0, diff.index), nil)
		}
		loadedAccount.Credits().Update(valueChange, diff.index)
		b.bicTree.Set(accountID, loadedAccount)
	}
	b.bicIndex = diff.index

	return b.BICTreeRoot(), nil
}

func (b *BICManager) removeOldDiff(index iotago.SlotIndex) {
	// delete the old slot diff that was applied
	b.slotDiffs.Delete(index)
}
