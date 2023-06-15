package accountsledger

import (
	"bytes"
	"testing"

	"github.com/orcaman/writerseeker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/accountsledger/tpkg"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestSlotDiffSnapshotWriter(t *testing.T) {
	accountID := utils.RandAccountID()
	accountDiff := tpkg.RandomAccountDiff()
	writer := &writerseeker.WriterSeeker{}
	pWriter := utils.NewPositionedWriter(writer)
	err := writeSlotDiff(pWriter, accountID, *accountDiff, true)

	accountDiffRead, accountIDRead, destroyedRead, err := readSlotDiff(writer.BytesReader())
	require.NoError(t, err)
	require.Equal(t, accountDiff, accountDiffRead)
	require.Equal(t, accountID, accountIDRead)
	require.Equal(t, true, destroyedRead)
}

func TestAccountDataSnapshotWriter(t *testing.T) {
	accountData := tpkg.RandomAccountData()
	accountsDataBytes, _ := accountData.Bytes()
	buf := bytes.NewReader(accountsDataBytes)

	readAccountsData, err := readAccountData(tpkg.API(), buf)
	require.NoError(t, err)

	tpkg.EqualAccountData(t, accountData, readAccountsData)
}

func TestAccountDiffSnapshotWriter(t *testing.T) {
	accountData := tpkg.RandomAccountData()
	writer := &writerseeker.WriterSeeker{}
	pWriter := utils.NewPositionedWriter(writer)
	err := writeAccountData(pWriter, accountData)
	require.NoError(t, err)
	readAccountsData, err := readAccountData(tpkg.API(), writer.BytesReader())
	require.NoError(t, err)
	tpkg.EqualAccountData(t, accountData, readAccountsData)
}

func TestManager_Import_Export(t *testing.T) {
	scenarioBuildData, expectedData, blockFunc, burnedBlocks := tpkg.InitScenario(t, tpkg.Scenario1)
	params := tpkg.ProtocolParams()

	manager := InitAccountLedger(t, blockFunc, params.MaxCommitableAge, scenarioBuildData, burnedBlocks)
	writer := &writerseeker.WriterSeeker{}

	err := manager.Export(writer, iotago.SlotIndex(1))
	require.NoError(t, err)
	slotDiffFunc, _ := tpkg.InitSlotDiff()
	accountsStore := mapdb.NewMapDB()

	newManager := New(blockFunc, slotDiffFunc, accountsStore, tpkg.API(), params.MaxCommitableAge)
	err = newManager.Import(writer.BytesReader())
	require.NoError(t, err)

	AssertAccountManagerState(t, newManager, expectedData)
}

func InitAccountLedger(t *testing.T, blockFunc func(iotago.BlockID) (*blocks.Block, bool), mca uint32, scenarioBuildData map[iotago.SlotIndex]*tpkg.AccountsSlotBuildData, burnedBlocks map[iotago.SlotIndex][]iotago.BlockID) *Manager {
	slotDiffFunc, _ := tpkg.InitSlotDiff()
	accountsStore := mapdb.NewMapDB()

	// feed the manager with the data
	manager := New(blockFunc, slotDiffFunc, accountsStore, tpkg.API(), mca)
	for index := iotago.SlotIndex(1); index <= iotago.SlotIndex(len(scenarioBuildData)); index++ {
		for _, burningBlock := range burnedBlocks[index] {
			block, exists := blockFunc(burningBlock)
			assert.True(t, exists)
			manager.TrackBlock(block)
		}
		slotBuildData := scenarioBuildData[index]
		err := manager.ApplyDiff(index, slotBuildData.SlotDiff, slotBuildData.DestroyedAccounts)
		require.NoError(t, err)
	}
	return manager
}

// AssertAccountManagerState asserts the state of the account manager for diffs per slot, and for accountLedger at the end.
func AssertAccountManagerState(t *testing.T, manager *Manager, scenarioExpected tpkg.ExpectedAccountsLedgers) {
	// assert diffs for each slot without asserting the vector
	for index := iotago.SlotIndex(1); index <= iotago.SlotIndex(len(scenarioExpected)); index++ {
		expectedData := scenarioExpected[index]
		for accID, expectedDiff := range expectedData.AccountsDiffs {
			actualAccDiff, _, err2 := manager.LoadSlotDiff(expectedData.LatestCommittedSlotIndex, accID)
			require.NoError(t, err2)
			assert.Equal(t, expectedDiff, actualAccDiff)
		}
	}
	expectedEndData := scenarioExpected[iotago.SlotIndex(len(scenarioExpected))]
	for accID, expectedAccData := range expectedEndData.AccountsLedger {
		actualData, exists, err2 := manager.Account(accID)
		assert.NoError(t, err2)
		assert.True(t, exists)
		assert.Equal(t, expectedAccData, actualData)
	}
}
