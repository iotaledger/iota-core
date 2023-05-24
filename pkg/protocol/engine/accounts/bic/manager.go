package bic

import (
	"crypto/ed25519"
	"fmt"
	"io"
	"sync"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	iotago "github.com/iotaledger/iota.go/v4"
)

type AccountDiff struct {
	index      iotago.SlotIndex
	allotments map[iotago.AccountID]uint64
	burns      map[iotago.AccountID]uint64
}

// BlockIssuanceCredits is a Block Issuer Credits module responsible for tracking account-based mana balances.
type BlockIssuanceCredits struct {
	mutex sync.RWMutex

	// the slot index of the balances vector ("LatestCommitedSlot - MCA")
	balancesIndex iotago.SlotIndex
	// balances represents the Block Issuer Credits of all registered accounts, it is updated on the slot commitment.
	balances *shrinkingmap.ShrinkingMap[iotago.AccountID, accounts.Account]

	// the slot index of the latest slot diff in the map
	latestSlotDiffsIndex iotago.SlotIndex
	// slot diffs for the BIC between [LatestCommitedSlot - MCA, LatestCommitedSlot]
	slotDiffs *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *AccountDiff]

	module.Module
}

func (b *BlockIssuanceCredits) CommitSlot(slotIndex iotago.SlotIndex, allotments map[iotago.AccountID]uint64, burns map[iotago.AccountID]uint64) (bicRoot iotago.Identifier, err error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// TODO: should we combine allotments and burns in one value?
	diff := &AccountDiff{
		index:      slotIndex,
		allotments: allotments,
		burns:      burns,
	}

	if err := b.applyDiff(diff); err != nil {
		return iotago.Identifier{}, err
	}

	return iotago.Identifier{}, nil
}

func (b *BlockIssuanceCredits) BIC(id iotago.AccountID, slotIndex iotago.SlotIndex) (accounts.Account, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if slotIndex < b.balancesIndex {
		return nil, fmt.Errorf("can't calculate BIC, slot index older than balancesIndex (%d<%d)", slotIndex, b.balancesIndex)
	}

	if slotIndex > b.latestSlotDiffsIndex {
		return nil, fmt.Errorf("can't calculate BIC, slot index newer than latestSlotDiffsIndex (%d>%d)", slotIndex, b.latestSlotDiffsIndex)
	}

	slotsToApply := slotIndex - b.balancesIndex

	// check if there is an account, if not we need to create it
	account, _ := b.balances.GetOrCreate(id, func() accounts.Account {
		return accounts.NewAccount(id, accounts.NewWeight(0, slotIndex), []ed25519.PublicKey{})
	})

	newAccount := account.Clone()

	for slotDiffIndex := b.balancesIndex + 1; slotDiffIndex <= b.balancesIndex+slotsToApply; slotDiffIndex++ {
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

		// TODO: add an update method
		newAccount.Credits().Value += accountSlotDiff
	}

	return newAccount, nil
}

func (b *BlockIssuanceCredits) Shutdown() {
}

func (b *BlockIssuanceCredits) LatestCommittedIndex() (iotago.SlotIndex, error) {
	return b.latestSlotDiffsIndex, nil
}

func (b *BlockIssuanceCredits) applyDiff(newDiff *AccountDiff) error {
	// TODO (daria): do we need to store the index, if yes should it be in the engine store or should we create new kv store as in the ledger?

	// check if the expected next slot diff is applied
	if newDiff.index != b.latestSlotDiffsIndex+1 {
		// TODO: nicer error message
		return fmt.Errorf("there is a gap in the bicstate (%d vs %d)", newDiff.index, b.latestSlotDiffsIndex+1)
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
	// TODO: is there an off-by-one error?
	oldDiff, exists := b.slotDiffs.Get(newBalancesIndex)
	if !exists {
		// TODO: nicer error message
		return fmt.Errorf("slot index does not exist: %d", newDiff.index-iotago.MaxCommitableSlotAge)
	}

	for accountID, allotmentValue := range oldDiff.allotments {
		// check if there is an account, if not we need to create it
		account, _ := b.balances.GetOrCreate(accountID, func() accounts.Account {
			return accounts.NewAccount(accountID, accounts.NewWeight(0, newBalancesIndex), []ed25519.PublicKey{})
		})

		account.Credits().Value += int64(allotmentValue)
	}

	for accountID, burnValue := range oldDiff.burns {
		// check if there is an account, if not we need to create it
		account, _ := b.balances.GetOrCreate(accountID, func() accounts.Account {
			return accounts.NewAccount(accountID, accounts.NewWeight(0, newBalancesIndex), []ed25519.PublicKey{})
		})

		account.Credits().Value -= int64(burnValue)
	}

	// set the new balances index
	b.balancesIndex = newBalancesIndex

	// delete the old slot diff that was applied
	b.slotDiffs.Delete(newBalancesIndex)

	return nil
}

func (b *BlockIssuanceCredits) Import(reader io.ReadSeeker) error {
	// TODO:
	// we will have one complete vector for the BIC for one slot in the snapshot file
	// latest commitment - MCA
	// then we also have the BIC diffs until latest committed slot index

	return nil
}

func (b *BlockIssuanceCredits) Export(writer io.WriteSeeker, targetIndex iotago.SlotIndex) error {
	// TODO:
	// do something similar like:
	/*
		m.ReadLockLedger()
		defer m.ReadUnlockLedger()

		ledgerIndex, err := m.ReadLedgerIndexWithoutLocking()
		if err != nil {
			return err
		}
		if err := utils.WriteValueFunc(writer, "ledgerIndex", ledgerIndex); err != nil {
			return err
		}

		var relativeCountersPosition int64

		var outputCount uint64
		var slotDiffCount uint64

		// Outputs Count
		// The amount of UTXOs contained within this snapshot.
		if err := utils.WriteValueFunc(writer, "outputs count", outputCount, &relativeCountersPosition); err != nil {
			return err
		}

		// Slot Diffs Count
		// The amount of slot diffs contained within this snapshot.
		if err := utils.WriteValueFunc(writer, "slot diffs count", slotDiffCount, &relativeCountersPosition); err != nil {
			return err
		}

		// Get all UTXOs and sort them by outputID
		outputIDs, err := m.UnspentOutputsIDs(ReadLockLedger(false))
		if err != nil {
			return err
		}

		for _, outputID := range outputIDs.RemoveDupsAndSort() {
			output, err := m.ReadOutputByOutputID(outputID)
			if err != nil {
				return err
			}

			if err := utils.WriteBytesFunc(writer, "outputID", output.SnapshotBytes(), &relativeCountersPosition); err != nil {
				return err
			}

			outputCount++
		}

		for diffIndex := ledgerIndex; diffIndex > targetIndex; diffIndex-- {
			slotDiff, err := m.SlotDiffWithoutLocking(diffIndex)
			if err != nil {
				return err
			}

			written, err := WriteSlotDiffToSnapshotWriter(writer, slotDiff)
			if err != nil {
				return err
			}

			relativeCountersPosition += written
			slotDiffCount++
		}

		// seek back to the file position of the counters
		if _, err := writer.Seek(-relativeCountersPosition, io.SeekCurrent); err != nil {
			return fmt.Errorf("unable to seek to LS counter placeholders: %w", err)
		}

		var countersSize int64

		// Outputs Count
		// The amount of UTXOs contained within this snapshot.
		if err := utils.WriteValueFunc(writer, "outputs count", outputCount, &countersSize); err != nil {
			return err
		}

		// Slot Diffs Count
		// The amount of slot diffs contained within this snapshot.
		if err := utils.WriteValueFunc(writer, "slot diffs count", slotDiffCount, &countersSize); err != nil {
			return err
		}

		// seek back to the last write position
		if _, err := writer.Seek(relativeCountersPosition-countersSize, io.SeekCurrent); err != nil {
			return fmt.Errorf("unable to seek to LS last written position: %w", err)
		}
	*/

	return nil
}
