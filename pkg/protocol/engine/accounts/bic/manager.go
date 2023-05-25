package bic

import (
	"crypto/ed25519"
	"fmt"
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/pkg/errors"
	"sync"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	iotago "github.com/iotaledger/iota.go/v4"
)

type BICDiff struct {
	index      iotago.SlotIndex            `serix:"0"`
	allotments map[iotago.AccountID]uint64 `serix:"1,lengthPrefixType=uint32,omitempty"`
	burns      map[iotago.AccountID]uint64 `serix:"2,lengthPrefixType=uint32,omitempty"`
}

// BlockIssuanceCredits is a Block Issuer Credits module responsible for tracking account-based mana balances.
type BlockIssuanceCredits struct {
	mutex sync.RWMutex

	// TODO need to store BIC vector at least, only diffs can be recreated based on the ledgerstate
	store kvstore.KVStore

	// the slot index of the bic vector ("LatestCommitedSlot - MCA")
	bicIndex iotago.SlotIndex
	// bic represents the Block Issuer Credits of all registered accounts, it is updated on the slot commitment.
	bic *shrinkingmap.ShrinkingMap[iotago.AccountID, accounts.Account]

	// the slot index of the latest slot diff in the map
	latestSlotDiffsIndex iotago.SlotIndex
	// slot diffs for the BIC between [LatestCommitedSlot - MCA, LatestCommitedSlot]
	slotDiffs *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *BICDiff]

	// the slot index of the latest bic tree in the map
	bicTreeIndex iotago.SlotIndex
	// TODO on reading from the snapshot: create the BIC tree from the bic vector and the slot diffs
	bicTree *ads.Map[iotago.AccountID, accounts.Credits, *iotago.AccountID, *accounts.Credits]

	apiProviderFunc func() iotago.API

	module.Module
}

func New(store kvstore.KVStore, apiProviderFunc func() iotago.API) *BlockIssuanceCredits {
	return &BlockIssuanceCredits{
		bic:       shrinkingmap.New[iotago.AccountID, accounts.Account](),
		slotDiffs: shrinkingmap.New[iotago.SlotIndex, *BICDiff](),
		bicTree:   ads.NewMap[iotago.AccountID, accounts.Credits](lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{StoreKeyPrefixBICTree}))),
	}
}

func (b *BlockIssuanceCredits) API() iotago.API {
	return b.apiProviderFunc()
}

func (b *BlockIssuanceCredits) BICTreeRoot() iotago.Identifier {
	return iotago.Identifier(b.bicTree.Root())
}

func (b *BlockIssuanceCredits) CommitSlot(slotIndex iotago.SlotIndex, allotments map[iotago.AccountID]uint64, burns map[iotago.AccountID]uint64) (bicRoot iotago.Identifier, err error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// TODO: should we combine allotments and burns in one value?
	diff := &BICDiff{
		index:      slotIndex,
		allotments: allotments,
		burns:      burns,
	}

	if err = b.applyDiff(diff); err != nil {
		return iotago.Identifier{}, err
	}

	if bicRoot, err = b.commitBICTree(diff); err != nil {
		return iotago.Identifier{}, err
	}

	return bicRoot, nil
}

func (b *BlockIssuanceCredits) BIC(id iotago.AccountID, slotIndex iotago.SlotIndex) (accounts.Account, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if slotIndex < b.bicIndex {
		return nil, fmt.Errorf("can't calculate BIC, slot index older than bicIndex (%d<%d)", slotIndex, b.bicIndex)
	}

	if slotIndex > b.latestSlotDiffsIndex {
		return nil, fmt.Errorf("can't calculate BIC, slot index newer than latestSlotDiffsIndex (%d>%d)", slotIndex, b.latestSlotDiffsIndex)
	}

	slotsToApply := slotIndex - b.bicIndex

	// check if there is an account, if not we need to create it
	account, _ := b.bic.GetOrCreate(id, func() accounts.Account {
		return accounts.NewAccount(id, accounts.NewCredits(0, slotIndex), []ed25519.PublicKey{})
	})

	newAccount := account.Clone()

	for slotDiffIndex := b.bicIndex + 1; slotDiffIndex <= b.bicIndex+slotsToApply; slotDiffIndex++ {
		slotDiff, exists := b.slotDiffs.Get(slotDiffIndex)
		if !exists {
			return nil, fmt.Errorf("can't calculate BIC, slot index doesn't exist (%d)", slotDiffIndex)
		}

		var accountSlotDiff int64
		if alloted, exists := slotDiff.allotments[id]; exists {
			accountSlotDiff += int64(alloted)
		}

		if burned, exists := slotDiff.burns[id]; exists {
			accountSlotDiff -= int64(burned)
		}
		updatedTime := slotDiff.index
		newAccount.Credits().Update(accountSlotDiff, updatedTime)
	}

	return newAccount, nil
}

func (b *BlockIssuanceCredits) Shutdown() {
}

func (b *BlockIssuanceCredits) LatestCommittedIndex() (iotago.SlotIndex, error) {
	return b.latestSlotDiffsIndex, nil
}

func (b *BlockIssuanceCredits) applyDiff(newDiff *BICDiff) error {
	// TODO (daria): do we need to store the index, if yes should it be in the engine store or should we create new kv store as in the ledger?

	// check if the expected next slot diff is applied
	if newDiff.index != b.latestSlotDiffsIndex+1 {
		return errors.Errorf("could not apply diff, expected slot index %d, got %d", b.latestSlotDiffsIndex+1, newDiff.index)
	}

	// add the new diff to the map
	b.slotDiffs.Set(newDiff.index, newDiff)

	// set the new latest slot diff index
	b.latestSlotDiffsIndex = newDiff.index

	// we only need to apply changes to the balances if the vector is MCA slots in the past
	if newDiff.index <= iotago.MaxCommitableSlotAge {
		return nil
	}

	newBalancesIndex := newDiff.index - iotago.MaxCommitableSlotAge

	// get the old diff
	oldDiff, exists := b.slotDiffs.Get(newBalancesIndex)
	if !exists {
		// TODO: nicer error message
		return fmt.Errorf("slot index does not exist: %d", newDiff.index-iotago.MaxCommitableSlotAge)
	}

	for accountID, allotmentValue := range oldDiff.allotments {
		// check if there is an account, if not we need to create it
		account, _ := b.bic.GetOrCreate(accountID, func() accounts.Account {
			return accounts.NewAccount(accountID, accounts.NewCredits(0, newBalancesIndex), []ed25519.PublicKey{})
		})

		account.Credits().Value += int64(allotmentValue)
	}

	for accountID, burnValue := range oldDiff.burns {
		// check if there is an account, if not we need to create it
		account, _ := b.bic.GetOrCreate(accountID, func() accounts.Account {
			return accounts.NewAccount(accountID, accounts.NewCredits(0, newBalancesIndex), []ed25519.PublicKey{})
		})

		account.Credits().Value -= int64(burnValue)
	}

	// set the new balances index
	b.bicIndex = newBalancesIndex

	// delete the old slot diff that was applied
	b.slotDiffs.Delete(newBalancesIndex)

	return nil
}

func (b *BlockIssuanceCredits) commitBICTree(diff *BICDiff) (bicRoot iotago.Identifier, err error) {
	// previous bic tree should be at index -1
	if b.bicTreeIndex != diff.index+1 {
		return iotago.Identifier{}, errors.Errorf("the difference between already committed bic: %d and the target commit: %d is different than 1", b.bicTreeIndex, diff.index)
	}

	// TODO change diff to have target balance only
	updates := make(map[iotago.AccountID]int64)
	for accountID, allotmentValue := range diff.allotments {
		updates[accountID] = int64(allotmentValue)
	}
	for accountID, burnValue := range diff.burns {
		updates[accountID] -= int64(burnValue)
	}
	// update the bic tree to latestCommitted slot index
	for accountID, valueChange := range updates {
		account, _ := b.bic.GetOrCreate(accountID, func() accounts.Account {
			return accounts.NewAccount(accountID, accounts.NewCredits(0, diff.index), []ed25519.PublicKey{})
		})
		account.Credits().Value += valueChange
		b.bic.Set(accountID, account)
	}
	b.bicTreeIndex = diff.index

	return b.BICTreeRoot(), nil
}
