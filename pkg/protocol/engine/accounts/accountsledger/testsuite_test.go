package accountsledger_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/accountsledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type TestSuite struct {
	T *testing.T

	apiProvider iotago.APIProvider

	accounts        map[string]iotago.AccountID
	blockIssuerKeys map[string]iotago.BlockIssuerKey
	outputs         map[string]iotago.OutputID

	slotData               *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *slotData]
	accountsStatePerSlot   *shrinkingmap.ShrinkingMap[iotago.SlotIndex, map[iotago.AccountID]*AccountState]
	latestFieldsPerAccount *shrinkingmap.ShrinkingMap[iotago.AccountID, *latestAccountFields]
	blocks                 *memstorage.IndexedStorage[iotago.SlotIndex, iotago.BlockID, *blocks.Block]
	Instance               *accountsledger.Manager
}

func NewTestSuite(test *testing.T) *TestSuite {
	testAPI := tpkg.TestAPI

	t := &TestSuite{
		T:               test,
		apiProvider:     iotago.SingleVersionProvider(testAPI),
		accounts:        make(map[string]iotago.AccountID),
		blockIssuerKeys: make(map[string]iotago.BlockIssuerKey),
		outputs:         make(map[string]iotago.OutputID),

		blocks:                 memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.BlockID, *blocks.Block](),
		slotData:               shrinkingmap.New[iotago.SlotIndex, *slotData](),
		accountsStatePerSlot:   shrinkingmap.New[iotago.SlotIndex, map[iotago.AccountID]*AccountState](),
		latestFieldsPerAccount: shrinkingmap.New[iotago.AccountID, *latestAccountFields](),
	}

	t.Instance = t.initAccountLedger()

	return t
}

func (t *TestSuite) initAccountLedger() *accountsledger.Manager {
	prunableStores := make(map[iotago.SlotIndex]kvstore.KVStore)
	slotDiffFunc := func(slot iotago.SlotIndex) (*slotstore.AccountDiffs, error) {
		if _, exists := prunableStores[slot]; !exists {
			prunableStores[slot] = mapdb.NewMapDB()
		}

		p := slotstore.NewAccountDiffs(slot, prunableStores[slot], tpkg.TestAPI)

		return p, nil
	}

	blockFunc := func(id iotago.BlockID) (*blocks.Block, bool) {
		storage := t.blocks.Get(id.Slot())
		if storage == nil {
			return nil, false
		}

		return storage.Get(id)
	}

	manager := accountsledger.New(t.apiProvider, blockFunc, slotDiffFunc, mapdb.NewMapDB())

	return manager
}

func (t *TestSuite) ApplySlotActions(slot iotago.SlotIndex, rmc iotago.Mana, actions map[string]*AccountActions) {
	slotDetails := newSlotData()
	t.slotData.Set(slot, slotDetails)

	// Commit an empty diff if no actions specified.
	if len(actions) == 0 {
		err := t.Instance.ApplyDiff(slot, rmc, make(map[iotago.AccountID]*model.AccountDiff), ds.NewSet[iotago.AccountID]())
		require.NoError(t.T, err)
		return
	}

	// Prepare the slot diff for each account based on the given actions.
	for alias, action := range actions {
		accountID := t.AccountID(alias, true)

		// Apply the burns to the manager.
		slotDetails.Burns[accountID] = iotago.Mana(action.NumBlocks) * rmc // this line assumes that the workscore of all block is 1
		for i := 0; i < action.NumBlocks; i++ {
			block := t.createBlockWithRMC(accountID, slot, rmc)
			t.blocks.Get(slot, true).Set(block.ID(), block)
			t.Instance.TrackBlock(block)
		}

		if action.Destroyed {
			slotDetails.DestroyedAccounts.Add(accountID)
		}

		prevAccountFields, exists := t.latestFieldsPerAccount.Get(accountID)
		if !exists {
			prevAccountFields = &latestAccountFields{
				OutputID:                      iotago.EmptyOutputID,
				BICUpdatedAt:                  0,
				UpdatedInSlots:                ds.NewSet[iotago.SlotIndex](),
				ExpirySlot:                    0,
				LatestSupportedVersionAndHash: model.VersionAndHash{},
			}
			t.latestFieldsPerAccount.Set(accountID, prevAccountFields)
		}
		prevAccountFields.UpdatedInSlots.Add(slot)

		// Put everything together in the format that the manager expects.
		slotDetails.SlotDiff[accountID] = &model.AccountDiff{
			BICChange:              iotago.BlockIssuanceCredits(action.TotalAllotments), // manager takes AccountDiff only with allotments filled in when applyDiff is triggered
			BlockIssuerKeysAdded:   t.BlockIssuerKeys(action.AddedKeys, true),
			BlockIssuerKeysRemoved: t.BlockIssuerKeys(action.RemovedKeys, true),
			PreviousUpdatedSlot:    prevAccountFields.BICUpdatedAt,
			NewExpirySlot:          prevAccountFields.ExpirySlot,

			DelegationStakeChange: action.DelegationStakeChange,
			ValidatorStakeChange:  action.ValidatorStakeChange,
			StakeEndEpochChange:   action.StakeEndEpochChange,
			FixedCostChange:       action.FixedCostChange,
		}

		if action.TotalAllotments+iotago.Mana(action.NumBlocks)*rmc != 0 || !exists { // this line assumes that workscore of all blocks is 1
			prevAccountFields.BICUpdatedAt = slot
		}

		// If an output ID is specified, we need to update the latest output ID for the account as we transitioned it within this slot.
		// Account creation and transition.
		if action.NewOutputID != "" {
			outputID := t.OutputID(action.NewOutputID, true)
			slotDiff := slotDetails.SlotDiff[accountID]

			slotDiff.NewOutputID = outputID
			slotDiff.PreviousOutputID = prevAccountFields.OutputID

			prevAccountFields.OutputID = outputID

		} else if action.StakeEndEpochChange != 0 || action.ValidatorStakeChange != 0 || action.FixedCostChange != 0 {
			panic("need to update outputID when updating stake end epoch or staking change")
		}

		// Account destruction.
		if prevAccountFields.OutputID != iotago.EmptyOutputID && slotDetails.SlotDiff[accountID].NewOutputID == iotago.EmptyOutputID {
			slotDetails.SlotDiff[accountID].PreviousOutputID = prevAccountFields.OutputID
		}

		// Sanity check for an incorrect case.
		if prevAccountFields.OutputID == iotago.EmptyOutputID && slotDetails.SlotDiff[accountID].NewOutputID == iotago.EmptyOutputID {
			panic("previous account OutputID in the local map and NewOutputID in slot diff cannot be empty")
		}

		if prevAccountFields.LatestSupportedVersionAndHash != action.LatestSupportedProtocolVersionAndHash {
			slotDetails.SlotDiff[accountID].PrevLatestSupportedVersionAndHash = prevAccountFields.LatestSupportedVersionAndHash
			slotDetails.SlotDiff[accountID].NewLatestSupportedVersionAndHash = action.LatestSupportedProtocolVersionAndHash

			prevAccountFields.LatestSupportedVersionAndHash = action.LatestSupportedProtocolVersionAndHash

		}
	}

	// Clone the diffs to prevent the manager from modifying it.
	diffs := make(map[iotago.AccountID]*model.AccountDiff)
	for accountID, diff := range slotDetails.SlotDiff {
		diffs[accountID] = diff.Clone()
	}

	err := t.Instance.ApplyDiff(slot, rmc, diffs, slotDetails.DestroyedAccounts.Clone())
	require.NoError(t.T, err)
}

func (t *TestSuite) createBlockWithRMC(accountID iotago.AccountID, slot iotago.SlotIndex, rmc iotago.Mana) *blocks.Block {
	innerBlock := tpkg.RandBasicBlockWithIssuerAndRMC(tpkg.TestAPI, accountID, rmc)
	innerBlock.Header.IssuingTime = tpkg.TestAPI.TimeProvider().SlotStartTime(slot)
	modelBlock, err := model.BlockFromBlock(innerBlock)

	require.NoError(t.T, err)

	return blocks.NewBlock(modelBlock)
}

func (t *TestSuite) AssertAccountLedgerUntilWithoutNewState(slot iotago.SlotIndex) {
	// Assert the state for each slot.
	for i := iotago.SlotIndex(1); i <= slot; i++ {
		storedAccountsState, exists := t.accountsStatePerSlot.Get(i)
		require.True(t.T, exists, "accountsStatePerSlot should exist for slot %d should exist", i)

		for accountID, expectedState := range storedAccountsState {
			t.assertAccountState(i, accountID, expectedState)
			t.assertDiff(i, accountID, expectedState)
		}
	}
}

func (t *TestSuite) AssertAccountLedgerUntil(slot iotago.SlotIndex, accountsState map[string]*AccountState) {
	expectedAccountsStateForSlot := make(map[iotago.AccountID]*AccountState)
	t.accountsStatePerSlot.Set(slot, expectedAccountsStateForSlot)

	// Populate accountsStatePerSlot with the expected state for the given slot.
	for alias, expectedState := range accountsState {
		accountID := t.AccountID(alias, false)
		expectedAccountsStateForSlot[accountID] = expectedState
	}

	t.AssertAccountLedgerUntilWithoutNewState(slot)
}

func (t *TestSuite) assertAccountState(slot iotago.SlotIndex, accountID iotago.AccountID, expectedState *AccountState) {
	expectedBlockIssuerKeys := t.BlockIssuerKeys(expectedState.BlockIssuerKeys, false)
	expectedCredits := accounts.NewBlockIssuanceCredits(iotago.BlockIssuanceCredits(expectedState.BICAmount), expectedState.BICUpdatedTime)

	actualState, exists, err := t.Instance.Account(accountID, slot)
	require.NoError(t.T, err)

	if expectedState.Destroyed {
		require.False(t.T, exists)

		return
	}

	require.True(t.T, exists)

	require.Equal(t.T, accountID, actualState.ID)
	require.Equal(t.T, expectedCredits, actualState.Credits, "slot: %d, accountID %s: expected: %v, actual: %v", slot, accountID, expectedCredits, actualState.Credits)
	require.True(t.T, expectedBlockIssuerKeys.Equal(actualState.BlockIssuerKeys), "slot: %d, accountID %s: expected: %s, actual: %s", slot, accountID, expectedBlockIssuerKeys, actualState.BlockIssuerKeys)

	require.Equal(t.T, t.OutputID(expectedState.OutputID, false), actualState.OutputID)
	require.Equal(t.T, expectedState.StakeEndEpoch, actualState.StakeEndEpoch, "slot: %d, accountID %s: expected StakeEndEpoch: %d, actual: %d", slot, accountID, expectedState.StakeEndEpoch, actualState.StakeEndEpoch)
	require.Equal(t.T, expectedState.ValidatorStake, actualState.ValidatorStake, "slot: %d, accountID %s: expected ValidatorStake: %d, actual: %d", slot, accountID, expectedState.ValidatorStake, actualState.ValidatorStake)
	require.Equal(t.T, expectedState.FixedCost, actualState.FixedCost, "slot: %d, accountID %s: expected FixedCost: %d, actual: %d", slot, accountID, expectedState.FixedCost, actualState.FixedCost)
	require.Equal(t.T, expectedState.DelegationStake, actualState.DelegationStake, "slot: %d, accountID %s: expected DelegationStake: %d, actual: %d", slot, accountID, expectedState.DelegationStake, actualState.DelegationStake)
	require.Equal(t.T, expectedState.LatestSupportedProtocolVersionAndHash, actualState.LatestSupportedProtocolVersionAndHash, "slot: %d, accountID %s: expected LatestSupportedProtocolVersionAndHash: %d, actual: %d", slot, accountID, expectedState.LatestSupportedProtocolVersionAndHash, actualState.LatestSupportedProtocolVersionAndHash)

}

func (t *TestSuite) assertDiff(slot iotago.SlotIndex, accountID iotago.AccountID, expectedState *AccountState) {
	actualDiff, destroyed, err := t.Instance.LoadSlotDiff(slot, accountID)
	if !lo.Return1(t.latestFieldsPerAccount.Get(accountID)).UpdatedInSlots.Has(slot) {
		require.Errorf(t.T, err, "expected error for account %s at slot %d", accountID, slot)
		return
	}
	require.NoError(t.T, err)

	accountsSlotBuildData, exists := t.slotData.Get(slot)
	require.True(t.T, exists)
	expectedAccountDiff := accountsSlotBuildData.SlotDiff[accountID]

	require.Equal(t.T, expectedAccountDiff.PreviousOutputID, actualDiff.PreviousOutputID)
	require.Equal(t.T, expectedAccountDiff.PreviousUpdatedSlot, actualDiff.PreviousUpdatedSlot)
	require.Equal(t.T, expectedAccountDiff.NewExpirySlot, actualDiff.NewExpirySlot)
	require.Equal(t.T, expectedAccountDiff.PreviousExpirySlot, actualDiff.PreviousExpirySlot)

	if expectedState.Destroyed {
		require.Equal(t.T, expectedState.Destroyed, destroyed)
		require.True(t.T, accountsSlotBuildData.DestroyedAccounts.Has(accountID))

		if slot > 1 {
			previousAccountState, exists := t.accountsStatePerSlot.Get(slot - 1)
			require.True(t.T, exists)

			require.True(t.T, t.BlockIssuerKeys(previousAccountState[accountID].BlockIssuerKeys, false).Equal(actualDiff.BlockIssuerKeysRemoved))
			require.Equal(t.T, -iotago.BlockIssuanceCredits(previousAccountState[accountID].BICAmount), actualDiff.BICChange)
			require.Equal(t.T, iotago.EmptyOutputID, actualDiff.NewOutputID)
			require.Equal(t.T, iotago.SlotIndex(0), actualDiff.NewExpirySlot)

			return
		}
	}

	require.Equal(t.T, expectedAccountDiff.NewOutputID, actualDiff.NewOutputID)
	require.Equal(t.T, expectedAccountDiff.NewExpirySlot, actualDiff.NewExpirySlot)
	require.Equal(t.T, expectedAccountDiff.BICChange-iotago.BlockIssuanceCredits(accountsSlotBuildData.Burns[accountID]), actualDiff.BICChange)
	require.Equal(t.T, expectedAccountDiff.BlockIssuerKeysAdded, actualDiff.BlockIssuerKeysAdded)
	require.Equal(t.T, expectedAccountDiff.BlockIssuerKeysRemoved, actualDiff.BlockIssuerKeysRemoved)
	require.Equal(t.T, expectedAccountDiff.PrevLatestSupportedVersionAndHash, actualDiff.PrevLatestSupportedVersionAndHash)
	require.Equal(t.T, expectedAccountDiff.NewLatestSupportedVersionAndHash, actualDiff.NewLatestSupportedVersionAndHash)

}

func (t *TestSuite) AccountID(alias string, createIfNotExists bool) iotago.AccountID {
	if accID, exists := t.accounts[alias]; exists {
		return accID
	} else if !createIfNotExists {
		panic(fmt.Sprintf("account with alias '%s' does not exist", alias))
	}

	t.accounts[alias] = tpkg.RandAccountID()
	t.accounts[alias].RegisterAlias(alias)

	return t.accounts[alias]
}

func (t *TestSuite) OutputID(alias string, createIfNotExists bool) iotago.OutputID {
	if outputID, exists := t.outputs[alias]; exists {
		return outputID
	} else if !createIfNotExists {
		panic(fmt.Sprintf("output with alias '%s' does not exist", alias))
	}
	t.outputs[alias] = tpkg.RandOutputID(1)

	return t.outputs[alias]
}

func (t *TestSuite) BlockIssuerKey(alias string, createIfNotExists bool) iotago.BlockIssuerKey {
	if blockIssuerKey, exists := t.blockIssuerKeys[alias]; exists {
		return blockIssuerKey
	} else if !createIfNotExists {
		panic(fmt.Sprintf("block issuer key with alias '%s' does not exist", alias))
	}

	t.blockIssuerKeys[alias] = utils.RandBlockIssuerKey()

	return t.blockIssuerKeys[alias]
}

func (t *TestSuite) BlockIssuerKeys(blockIssuerKeys []string, createIfNotExists bool) iotago.BlockIssuerKeys {
	keys := iotago.NewBlockIssuerKeys()
	for _, blockIssuerKey := range blockIssuerKeys {
		keys.Add(t.BlockIssuerKey(blockIssuerKey, createIfNotExists))
	}

	return keys
}

type AccountActions struct {
	TotalAllotments iotago.Mana
	NumBlocks       int
	Destroyed       bool
	AddedKeys       []string
	RemovedKeys     []string

	ValidatorStakeChange                  int64
	StakeEndEpochChange                   int64
	FixedCostChange                       int64
	LatestSupportedProtocolVersionAndHash model.VersionAndHash

	DelegationStakeChange int64

	NewOutputID string
}

type AccountState struct {
	BICAmount      iotago.Mana
	BICUpdatedTime iotago.SlotIndex

	OutputID        string
	BlockIssuerKeys []string

	ValidatorStake                        iotago.BaseToken
	DelegationStake                       iotago.BaseToken
	FixedCost                             iotago.Mana
	StakeEndEpoch                         iotago.EpochIndex
	LatestSupportedProtocolVersionAndHash model.VersionAndHash

	Destroyed bool
}

type latestAccountFields struct {
	OutputID                      iotago.OutputID
	BICUpdatedAt                  iotago.SlotIndex
	UpdatedInSlots                ds.Set[iotago.SlotIndex]
	ExpirySlot                    iotago.SlotIndex
	LatestSupportedVersionAndHash model.VersionAndHash
}

type slotData struct {
	Burns             map[iotago.AccountID]iotago.Mana
	SlotDiff          map[iotago.AccountID]*model.AccountDiff
	DestroyedAccounts ds.Set[iotago.AccountID]
}

func newSlotData() *slotData {
	return &slotData{
		DestroyedAccounts: ds.NewSet[iotago.AccountID](),
		Burns:             make(map[iotago.AccountID]iotago.Mana),
		SlotDiff:          make(map[iotago.AccountID]*model.AccountDiff),
	}
}
