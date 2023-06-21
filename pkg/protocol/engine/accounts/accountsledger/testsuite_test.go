package accountsledger_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/accountsledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type TestSuite struct {
	T *testing.T

	accounts map[string]iotago.AccountID
	pubKeys  map[string]ed25519.PublicKey
	outputs  map[string]iotago.OutputID

	ProtocolParameters *iotago.ProtocolParameters

	slotData                 *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *slotData]
	accountsStatePerSlot     *shrinkingmap.ShrinkingMap[iotago.SlotIndex, map[iotago.AccountID]*AccountState]
	latestOutputIDPerAccount *shrinkingmap.ShrinkingMap[iotago.AccountID, *latestAccountOutputID]
	blocks                   *memstorage.IndexedStorage[iotago.SlotIndex, iotago.BlockID, *blocks.Block]
	Instance                 *accountsledger.Manager
}

func NewTestSuite(test *testing.T) *TestSuite {
	t := &TestSuite{
		T:        test,
		accounts: make(map[string]iotago.AccountID),
		pubKeys:  make(map[string]ed25519.PublicKey),
		outputs:  make(map[string]iotago.OutputID),

		ProtocolParameters: &iotago.ProtocolParameters{
			Version:     3,
			NetworkName: utils.RandString(255),
			Bech32HRP:   iotago.NetworkPrefix(utils.RandString(3)),
			MinPoWScore: utils.RandUint32(50000),
			RentStructure: iotago.RentStructure{
				VByteCost:    100,
				VBFactorData: 1,
				VBFactorKey:  10,
			},
			TokenSupply:           utils.RandAmount(),
			GenesisUnixTimestamp:  uint32(time.Now().Unix()),
			SlotDurationInSeconds: 10,
			MaxCommitableAge:      10,
		},

		blocks:                   memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.BlockID, *blocks.Block](),
		slotData:                 shrinkingmap.New[iotago.SlotIndex, *slotData](),
		accountsStatePerSlot:     shrinkingmap.New[iotago.SlotIndex, map[iotago.AccountID]*AccountState](),
		latestOutputIDPerAccount: shrinkingmap.New[iotago.AccountID, *latestAccountOutputID](),
	}

	t.Instance = t.initAccountLedger()

	return t
}

func (t *TestSuite) initAccountLedger() *accountsledger.Manager {
	prunableStores := make(map[iotago.SlotIndex]kvstore.KVStore)
	slotDiffFunc := func(index iotago.SlotIndex) *prunable.AccountDiffs {
		if _, exists := prunableStores[index]; !exists {
			prunableStores[index] = mapdb.NewMapDB()
		}

		p := prunable.NewAccountDiffs(index, prunableStores[index], t.API())

		return p
	}

	blockFunc := func(id iotago.BlockID) (*blocks.Block, bool) {
		storage := t.blocks.Get(id.Index())
		if storage == nil {
			return nil, false
		}

		return storage.Get(id)
	}

	manager := accountsledger.New(blockFunc, slotDiffFunc, mapdb.NewMapDB(), t.API())
	manager.SetMaxCommittableAge(iotago.SlotIndex(t.ProtocolParameters.MaxCommitableAge))

	return manager
}

func (t *TestSuite) API() iotago.API {
	return iotago.LatestAPI(t.ProtocolParameters)
}

func (t *TestSuite) ApplySlotActions(slotIndex iotago.SlotIndex, actions map[string]*AccountActions) {
	slotDetails := newSlotData()
	t.slotData.Set(slotIndex, slotDetails)

	// Commit an empty diff if no actions specified.
	if actions == nil || len(actions) == 0 {
		err := t.Instance.ApplyDiff(slotIndex, make(map[iotago.AccountID]*prunable.AccountDiff), advancedset.New[iotago.AccountID]())
		require.NoError(t.T, err)
		return
	}

	// Prepare the slot diff for each account based on the given actions.
	for alias, action := range actions {
		accountID := t.AccountID(alias, true)

		// Apply the burns to the manager.
		slotDetails.Burns[accountID] = lo.Sum[uint64](action.Burns...)
		for _, burn := range action.Burns {
			block := t.createBlockWithBurn(accountID, slotIndex, burn)
			t.blocks.Get(slotIndex, true).Set(block.ID(), block)
			t.Instance.TrackBlock(block)
		}

		if action.Destroyed {
			slotDetails.DestroyedAccounts.Add(accountID)
		}

		//
		prevAccountOutputID, exists := t.latestOutputIDPerAccount.Get(accountID)
		if !exists {
			prevAccountOutputID = &latestAccountOutputID{
				OutputID:  iotago.EmptyOutputID,
				UpdatedAt: 0,
			}
		}

		// Put everything together in the format that the manager expects.
		slotDetails.SlotDiff[accountID] = &prunable.AccountDiff{
			Change:         int64(action.TotalAllotments), // manager takes AccountDiff only with allotments filled in when applyDiff is triggered
			PubKeysAdded:   t.PublicKeys(action.AddedKeys, true),
			PubKeysRemoved: t.PublicKeys(action.RemovedKeys, true),

			PreviousUpdatedTime: prevAccountOutputID.UpdatedAt,
			PreviousOutputID:    prevAccountOutputID.OutputID,
		}

		// If an output ID is specified, we need to update the latest output ID for the account as we transitioned it within this slot.
		if action.NewOutputID != "" {
			outputID := t.OutputID(action.NewOutputID, true)
			slotDetails.SlotDiff[accountID].NewOutputID = outputID
			t.latestOutputIDPerAccount.Set(accountID, &latestAccountOutputID{
				OutputID:  outputID,
				UpdatedAt: slotIndex,
			})
		}
	}

	// Clone the diffs to prevent the manager from modifying it.
	diffs := make(map[iotago.AccountID]*prunable.AccountDiff)
	for accountID, diff := range slotDetails.SlotDiff {
		diffs[accountID] = diff.Clone()
	}

	err := t.Instance.ApplyDiff(slotIndex, diffs, slotDetails.DestroyedAccounts.Clone())
	require.NoError(t.T, err)
}

func (t *TestSuite) createBlockWithBurn(accountID iotago.AccountID, index iotago.SlotIndex, burn uint64) *blocks.Block {
	innerBlock := tpkg.RandBlockWithIssuerAndBurnedMana(accountID, burn)
	innerBlock.IssuingTime = t.API().SlotTimeProvider().SlotStartTime(index)
	modelBlock, err := model.BlockFromBlock(innerBlock, t.API())

	require.NoError(t.T, err)

	return blocks.NewBlock(modelBlock)
}

func (t *TestSuite) AssertAccountLedgerUntilWithoutNewState(slotIndex iotago.SlotIndex) {
	// Assert the state for each slot.
	for i := iotago.SlotIndex(1); i <= slotIndex; i++ {
		storedAccountsState, exists := t.accountsStatePerSlot.Get(i)
		require.True(t.T, exists)

		for accountID, expectedState := range storedAccountsState {
			t.assertAccountState(i, accountID, expectedState)
			t.assertDiff(i, accountID, expectedState)
		}
	}
}

func (t *TestSuite) AssertAccountLedgerUntil(slotIndex iotago.SlotIndex, accountsState map[string]*AccountState) {
	expectedAccountsStateForSlot := make(map[iotago.AccountID]*AccountState)
	t.accountsStatePerSlot.Set(slotIndex, expectedAccountsStateForSlot)

	// Populate accountsStatePerSlot with the expected state for the given slot.
	for alias, expectedState := range accountsState {
		accountID := t.AccountID(alias, false)
		expectedAccountsStateForSlot[accountID] = expectedState
	}

	t.AssertAccountLedgerUntilWithoutNewState(slotIndex)
}

func (t *TestSuite) assertAccountState(slotIndex iotago.SlotIndex, accountID iotago.AccountID, expectedState *AccountState) {
	expectedPubKeys := advancedset.New[ed25519.PublicKey](t.PublicKeys(expectedState.PubKeys, false)...)
	expectedCredits := accounts.NewBlockIssuanceCredits(int64(expectedState.Amount), expectedState.UpdatedTime)

	actualState, exists, err := t.Instance.Account(accountID, slotIndex)
	require.NoError(t.T, err)

	if expectedState.Destroyed {
		require.False(t.T, exists)

		return
	}

	require.True(t.T, exists)

	require.Equal(t.T, accountID, actualState.ID)
	require.Equal(t.T, expectedCredits, actualState.Credits, "slotIndex: %d, accountID %s: expected: %v, actual: %v", slotIndex, accountID, expectedCredits, actualState.Credits)
	require.Truef(t.T, expectedPubKeys.Equal(actualState.PubKeys), "slotIndex: %d, accountID %s: expected: %s, actual: %s", slotIndex, accountID, expectedPubKeys, actualState.PubKeys)

	require.Equal(t.T, t.OutputID(expectedState.OutputID, false), actualState.OutputID)
}

func (t *TestSuite) assertDiff(slotIndex iotago.SlotIndex, accountID iotago.AccountID, expectedState *AccountState) {
	actualDiff, destroyed, err := t.Instance.LoadSlotDiff(slotIndex, accountID)
	if expectedState.UpdatedTime < slotIndex {
		require.Errorf(t.T, err, "expected error for account %s at slot %d", accountID, slotIndex)
		return
	}
	require.NoError(t.T, err)

	accountsSlotBuildData, exists := t.slotData.Get(slotIndex)
	require.True(t.T, exists)
	expectedAccountDiff := accountsSlotBuildData.SlotDiff[accountID]

	require.Equal(t.T, expectedAccountDiff.PreviousOutputID, actualDiff.PreviousOutputID)
	require.Equal(t.T, expectedAccountDiff.PreviousUpdatedTime, actualDiff.PreviousUpdatedTime)

	if expectedState.Destroyed {
		require.Equal(t.T, expectedState.Destroyed, destroyed)
		require.True(t.T, accountsSlotBuildData.DestroyedAccounts.Has(accountID))

		if slotIndex > 1 {
			previousAccountState, exists := t.accountsStatePerSlot.Get(slotIndex - 1)
			require.True(t.T, exists)

			require.Equal(t.T, t.PublicKeys(previousAccountState[accountID].PubKeys, false), actualDiff.PubKeysRemoved)
			require.Equal(t.T, -int64(previousAccountState[accountID].Amount), actualDiff.Change)
			require.Equal(t.T, iotago.EmptyOutputID, actualDiff.NewOutputID)

			return
		}
	}

	require.Equal(t.T, expectedAccountDiff.NewOutputID, actualDiff.NewOutputID)
	require.Equal(t.T, expectedAccountDiff.Change-int64(accountsSlotBuildData.Burns[accountID]), actualDiff.Change)
	require.Equal(t.T, expectedAccountDiff.PubKeysAdded, actualDiff.PubKeysAdded)
	require.Equal(t.T, expectedAccountDiff.PubKeysRemoved, actualDiff.PubKeysRemoved)
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

func (t *TestSuite) PublicKey(alias string, createIfNotExists bool) ed25519.PublicKey {
	if pubKey, exists := t.pubKeys[alias]; exists {
		return pubKey
	} else if !createIfNotExists {
		panic(fmt.Sprintf("public key with alias '%s' does not exist", alias))
	}

	t.pubKeys[alias] = utils.RandPubKey()

	return t.pubKeys[alias]
}

func (t *TestSuite) PublicKeys(pubKeys []string, createIfNotExists bool) []ed25519.PublicKey {
	keys := make([]ed25519.PublicKey, len(pubKeys))
	for i, pubKey := range pubKeys {
		keys[i] = t.PublicKey(pubKey, createIfNotExists)
	}

	return keys
}

type AccountActions struct {
	TotalAllotments uint64
	Burns           []uint64
	Destroyed       bool
	AddedKeys       []string
	RemovedKeys     []string

	NewOutputID string
}

type AccountState struct {
	Amount   uint64
	OutputID string
	PubKeys  []string

	UpdatedTime iotago.SlotIndex

	Destroyed bool
}

type latestAccountOutputID struct {
	OutputID  iotago.OutputID
	UpdatedAt iotago.SlotIndex
}

type slotData struct {
	Burns             map[iotago.AccountID]uint64
	SlotDiff          map[iotago.AccountID]*prunable.AccountDiff
	DestroyedAccounts *advancedset.AdvancedSet[iotago.AccountID]
}

func newSlotData() *slotData {
	return &slotData{
		DestroyedAccounts: advancedset.New[iotago.AccountID](),
		Burns:             make(map[iotago.AccountID]uint64),
		SlotDiff:          make(map[iotago.AccountID]*prunable.AccountDiff),
	}
}
