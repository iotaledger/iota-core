package topstakers

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/activitytracker/activitytrackerv1"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/epochstore"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func TestTopStakers_InitializeCommittee(t *testing.T) {
	var testAPI = iotago.V3API(
		iotago.NewV3SnapshotProtocolParameters(
			iotago.WithWorkScoreOptions(0, 1, 0, 0, 0, 0, 0, 0, 0, 0), // all zero except block offset gives all blocks workscore = 1
			iotago.WithTargetCommitteeSize(3),
		),
	)
	testAPIProvider := iotago.SingleVersionProvider(testAPI)

	committeeStore := epochstore.NewStore(kvstore.Realm{}, mapdb.NewMapDB(), 0, (*account.Accounts).Bytes, account.AccountsFromBytes)

	topStakersSeatManager := &SeatManager{
		apiProvider:     testAPIProvider,
		committeeStore:  committeeStore,
		events:          seatmanager.NewEvents(),
		activityTracker: activitytrackerv1.NewActivityTracker(testAPIProvider),
	}

	// Try setting an empty committee.
	err := topStakersSeatManager.ReuseCommittee(0, account.NewAccounts())
	require.Error(t, err)

	// Create committee for epoch 0
	initialCommittee := account.NewAccounts()
	for i := 0; i < 3; i++ {
		if err := initialCommittee.Set(tpkg.RandAccountID(), &account.Pool{
			PoolStake:      1900,
			ValidatorStake: 900,
			FixedCost:      11,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Set committee for epoch 0.
	err = topStakersSeatManager.ReuseCommittee(0, initialCommittee)
	require.NoError(t, err)
	weightedSeats, exists := topStakersSeatManager.CommitteeInEpoch(0)
	require.True(t, exists)
	initialCommitteeAccountIDs := initialCommittee.IDs()

	// Online committee should be empty.
	require.True(t, topStakersSeatManager.OnlineCommittee().IsEmpty())

	// After initialization, the online committee should contain the seats of the initial committee.
	require.NoError(t, topStakersSeatManager.InitializeCommittee(0, time.Time{}))
	assertOnlineCommittee(t, topStakersSeatManager.OnlineCommittee(),
		lo.Return1(weightedSeats.GetSeat(initialCommitteeAccountIDs[0])),
		lo.Return1(weightedSeats.GetSeat(initialCommitteeAccountIDs[2])),
		lo.Return1(weightedSeats.GetSeat(initialCommitteeAccountIDs[2])),
	)
}

func TestTopStakers_RotateCommittee(t *testing.T) {
	var testAPI = iotago.V3API(
		iotago.NewV3SnapshotProtocolParameters(
			iotago.WithWorkScoreOptions(0, 1, 0, 0, 0, 0, 0, 0, 0, 0), // all zero except block offset gives all blocks workscore = 1
			iotago.WithTargetCommitteeSize(10),
		),
	)
	testAPIProvider := iotago.SingleVersionProvider(testAPI)

	committeeStore := epochstore.NewStore(kvstore.Realm{}, mapdb.NewMapDB(), 0, (*account.Accounts).Bytes, account.AccountsFromBytes)

	s := &SeatManager{
		apiProvider:     testAPIProvider,
		committeeStore:  committeeStore,
		events:          seatmanager.NewEvents(),
		activityTracker: activitytrackerv1.NewActivityTracker(testAPIProvider),
	}

	// Committee should not exist because it was never set.
	_, exists := s.CommitteeInSlot(10)
	require.False(t, exists)

	_, exists = s.CommitteeInEpoch(0)
	require.False(t, exists)

	var committeeInEpoch0 *account.SeatedAccounts
	var committeeInEpoch0IDs []iotago.AccountID
	expectedCommitteeInEpoch0 := account.NewAccounts()

	// Create committee for epoch 0
	{
		addCommitteeMember(t, expectedCommitteeInEpoch0, &account.Pool{PoolStake: 1900, ValidatorStake: 900, FixedCost: 11})
		addCommitteeMember(t, expectedCommitteeInEpoch0, &account.Pool{PoolStake: 1900, ValidatorStake: 900, FixedCost: 11})
		addCommitteeMember(t, expectedCommitteeInEpoch0, &account.Pool{PoolStake: 1900, ValidatorStake: 900, FixedCost: 11})

		// We should be able to set a committee with only 3 members for epoch 0 (this could be set e.g. via the snapshot).
		err := s.ReuseCommittee(0, expectedCommitteeInEpoch0)
		require.NoError(t, err)

		// Make sure that the online committee is handled correctly.
		{
			committeeInEpoch0, exists = s.CommitteeInEpoch(0)
			require.True(t, exists)
			committeeInEpoch0IDs = expectedCommitteeInEpoch0.IDs()

			require.True(t, s.OnlineCommittee().IsEmpty())

			s.activityTracker.MarkSeatActive(lo.Return1(committeeInEpoch0.GetSeat(committeeInEpoch0IDs[0])), committeeInEpoch0IDs[0], testAPI.TimeProvider().SlotStartTime(1))
			assertOnlineCommittee(t, s.OnlineCommittee(), lo.Return1(committeeInEpoch0.GetSeat(committeeInEpoch0IDs[0])))

			s.activityTracker.MarkSeatActive(lo.Return1(committeeInEpoch0.GetSeat(committeeInEpoch0IDs[1])), committeeInEpoch0IDs[1], testAPI.TimeProvider().SlotStartTime(2))
			assertOnlineCommittee(t, s.OnlineCommittee(), lo.Return1(committeeInEpoch0.GetSeat(committeeInEpoch0IDs[0])), lo.Return1(committeeInEpoch0.GetSeat(committeeInEpoch0IDs[1])))

			s.activityTracker.MarkSeatActive(lo.Return1(committeeInEpoch0.GetSeat(committeeInEpoch0IDs[2])), committeeInEpoch0IDs[2], testAPI.TimeProvider().SlotStartTime(3))
			assertOnlineCommittee(t, s.OnlineCommittee(), lo.Return1(committeeInEpoch0.GetSeat(committeeInEpoch0IDs[0])), lo.Return1(committeeInEpoch0.GetSeat(committeeInEpoch0IDs[1])), lo.Return1(committeeInEpoch0.GetSeat(committeeInEpoch0IDs[2])))

			// Make sure that after a period of inactivity, the inactive seats are marked as offline.
			s.activityTracker.MarkSeatActive(lo.Return1(committeeInEpoch0.GetSeat(committeeInEpoch0IDs[2])), committeeInEpoch0IDs[2], testAPI.TimeProvider().SlotEndTime(2+testAPI.ProtocolParameters().MinCommittableAge()))
			assertOnlineCommittee(t, s.OnlineCommittee(), lo.Return1(committeeInEpoch0.GetSeat(committeeInEpoch0IDs[2])))
		}

		// Make sure that the committee was assigned to the correct epoch.
		_, exists = s.CommitteeInEpoch(1)
		require.False(t, exists)

		// Make sure that the committee members match the expected ones.
		assertCommitteeInEpoch(t, s, testAPI, 0, expectedCommitteeInEpoch0)

		// Make sure that the committee size is correct for this epoch
		assertCommitteeSizeInEpoch(t, s, testAPI, 0, 3)
	}

	expectedCommitteeInEpoch1 := account.NewAccounts()
	// Design candidate list and expected committee members for epoch 1.
	{
		epoch := iotago.EpochIndex(1)
		accountsData := make(accounts.AccountsData, 0)
		numCandidates := 15
		expectedCommitteeSize := testAPI.ProtocolParameters().TargetCommitteeSize()
		require.EqualValues(t, expectedCommitteeSize, s.SeatCountInEpoch(epoch))

		s.SeatCountInEpoch(epoch)

		// Add some candidates that have the same fields to test sorting by secondary fields.
		{
			candidate0ID := tpkg.RandAccountID()
			candidate0ID.RegisterAlias("candidate0")
			accountsData = append(accountsData, &accounts.AccountData{
				ID:              candidate0ID,
				ValidatorStake:  100,
				DelegationStake: 800 - 399,
				FixedCost:       3,
				StakeEndEpoch:   iotago.MaxEpochIndex,
			})

			candidate1ID := tpkg.RandAccountID()
			candidate1ID.RegisterAlias("candidate1")
			accountsData = append(accountsData, &accounts.AccountData{
				ID:              candidate1ID,
				ValidatorStake:  100,
				DelegationStake: 800 - 399,
				FixedCost:       3,
				StakeEndEpoch:   iotago.MaxEpochIndex,
			})
		}

		for i := 2; i <= numCandidates; i++ {
			candidateAccountID := tpkg.RandAccountID()
			candidateAccountID.RegisterAlias(fmt.Sprintf("candidate%d", i))
			candidatePool := &account.Pool{
				PoolStake:      iotago.BaseToken(i * 100),
				ValidatorStake: iotago.BaseToken(i * 50),
				FixedCost:      iotago.Mana(i),
			}
			accountsData = append(accountsData, &accounts.AccountData{
				ID:              candidateAccountID,
				ValidatorStake:  iotago.BaseToken(i * 50),
				DelegationStake: iotago.BaseToken(i*100) - iotago.BaseToken(i*50),
				FixedCost:       tpkg.RandMana(iotago.MaxMana),
				StakeEndEpoch:   tpkg.RandEpoch(),
			})

			if i+int(expectedCommitteeSize) > numCandidates {
				require.NoError(t, expectedCommitteeInEpoch1.Set(candidateAccountID, candidatePool))
			}
		}

		// Rotate the committee and make sure that the returned committee matches the expected.
		rotatedCommitteeInEpoch1, err := s.RotateCommittee(epoch, accountsData)
		require.NoError(t, err)
		assertCommittee(t, expectedCommitteeInEpoch1, rotatedCommitteeInEpoch1)

		// Make sure that after committee rotation, the online committee is not changed.
		assertOnlineCommittee(t, s.OnlineCommittee(), lo.Return1(committeeInEpoch0.GetSeat(committeeInEpoch0IDs[2])))

		committeeInEpoch1Accounts, err := rotatedCommitteeInEpoch1.Accounts()
		require.NoError(t, err)
		newCommitteeMemberIDs := committeeInEpoch1Accounts.IDs()

		// A new committee member appears online and makes the previously active committee seat inactive.
		s.activityTracker.MarkSeatActive(lo.Return1(committeeInEpoch0.GetSeat(newCommitteeMemberIDs[0])), newCommitteeMemberIDs[0], testAPI.TimeProvider().SlotEndTime(2+2*testAPI.ProtocolParameters().MinCommittableAge()))
		assertOnlineCommittee(t, s.OnlineCommittee(), lo.Return1(committeeInEpoch0.GetSeat(newCommitteeMemberIDs[0])))

		// Make sure that the committee retrieved from the committee store matches the expected.
		assertCommitteeInEpoch(t, s, testAPI, 1, expectedCommitteeInEpoch1)
		assertCommitteeSizeInEpoch(t, s, testAPI, 1, 10)

		// Make sure that the previous committee was not modified and is still accessible.
		assertCommitteeInEpoch(t, s, testAPI, 0, expectedCommitteeInEpoch0)
		assertCommitteeSizeInEpoch(t, s, testAPI, 0, 3)
	}

	// Rotate committee again with fewer candidates than the target committee size.
	expectedCommitteeInEpoch2 := account.NewAccounts()
	{
		epoch := iotago.EpochIndex(2)
		accountsData := make(accounts.AccountsData, 0)

		candidate0ID := tpkg.RandAccountID()
		candidate0ID.RegisterAlias("candidate0-epoch2")
		accountsData = append(accountsData, &accounts.AccountData{
			ID:              candidate0ID,
			ValidatorStake:  100,
			DelegationStake: 800 - 399,
			FixedCost:       3,
			StakeEndEpoch:   iotago.MaxEpochIndex,
		})
		require.NoError(t, expectedCommitteeInEpoch2.Set(candidate0ID, &account.Pool{PoolStake: 1900, ValidatorStake: 900, FixedCost: 11}))

		// Rotate the committee and make sure that the returned committee matches the expected.
		rotatedCommitteeInEpoch2, err := s.RotateCommittee(epoch, accountsData)
		require.NoError(t, err)
		assertCommittee(t, expectedCommitteeInEpoch2, rotatedCommitteeInEpoch2)

		assertCommitteeInEpoch(t, s, testAPI, 2, expectedCommitteeInEpoch2)
		assertCommitteeSizeInEpoch(t, s, testAPI, 2, 1)

		// Make sure that the committee retrieved from the committee store matches the expected.
		assertCommitteeInEpoch(t, s, testAPI, 1, expectedCommitteeInEpoch1)
		assertCommitteeSizeInEpoch(t, s, testAPI, 1, 10)

		// Make sure that the previous committee was not modified and is still accessible.
		assertCommitteeInEpoch(t, s, testAPI, 0, expectedCommitteeInEpoch0)
		assertCommitteeSizeInEpoch(t, s, testAPI, 0, 3)
	}

	// Try to rotate committee with no candidates. Instead, set reuse of committee.
	{
		epoch := iotago.EpochIndex(3)
		accountsData := make(accounts.AccountsData, 0)

		_, err := s.RotateCommittee(epoch, accountsData)
		require.Error(t, err)

		// Set reuse of committee manually.
		expectedCommitteeInEpoch2.SetReused()
		err = s.ReuseCommittee(epoch, expectedCommitteeInEpoch2)
		require.NoError(t, err)

		assertCommitteeInEpoch(t, s, testAPI, 3, expectedCommitteeInEpoch2)
		assertCommitteeSizeInEpoch(t, s, testAPI, 3, 1)

		assertCommitteeInEpoch(t, s, testAPI, 2, expectedCommitteeInEpoch2)
		assertCommitteeSizeInEpoch(t, s, testAPI, 2, 1)

		// Make sure that the committee retrieved from the committee store matches the expected (with reused flag set).
		loadedCommittee, err := s.committeeStore.Load(epoch)
		require.NoError(t, err)
		require.True(t, loadedCommittee.IsReused())
		assertCommittee(t, expectedCommitteeInEpoch2, loadedCommittee.SeatedAccounts(loadedCommittee.IDs()...))
	}
}

func addCommitteeMember(t *testing.T, committee *account.Accounts, pool *account.Pool) iotago.AccountID {
	accountID := tpkg.RandAccountID()
	require.NoError(t, committee.Set(accountID, pool))

	return accountID
}

func assertCommitteeSizeInEpoch(t *testing.T, seatManager *SeatManager, testAPI iotago.API, epoch iotago.EpochIndex, expectedCommitteeSize int) {
	require.Equal(t, expectedCommitteeSize, seatManager.SeatCountInEpoch(epoch))
	require.Equal(t, expectedCommitteeSize, seatManager.SeatCountInSlot(testAPI.TimeProvider().EpochStart(epoch)))
	require.Equal(t, expectedCommitteeSize, seatManager.SeatCountInSlot(testAPI.TimeProvider().EpochEnd(epoch)))
}

func assertCommitteeInEpoch(t *testing.T, seatManager *SeatManager, testAPI iotago.API, epoch iotago.EpochIndex, expectedCommittee *account.Accounts) {
	committee, exists := seatManager.CommitteeInEpoch(epoch)
	require.True(t, exists)
	assertCommittee(t, expectedCommittee, committee)

	committee, exists = seatManager.CommitteeInSlot(testAPI.TimeProvider().EpochStart(epoch))
	require.True(t, exists)
	assertCommittee(t, expectedCommittee, committee)

	committee, exists = seatManager.CommitteeInSlot(testAPI.TimeProvider().EpochEnd(epoch))
	require.True(t, exists)
	assertCommittee(t, expectedCommittee, committee)
}

func assertCommittee(t *testing.T, expectedCommittee *account.Accounts, actualCommittee *account.SeatedAccounts) {
	actualAccounts, err := actualCommittee.Accounts()
	require.NoError(t, err)
	require.Equal(t, actualAccounts.Size(), expectedCommittee.Size())
	for _, memberID := range expectedCommittee.IDs() {
		require.Truef(t, actualAccounts.Has(memberID), "expected account %s to be part of committee, but it is not, actual committee members: %s", memberID, actualAccounts.IDs())
	}
}

func assertOnlineCommittee(t *testing.T, onlineCommittee ds.Set[account.SeatIndex], expectedOnlineSeats ...account.SeatIndex) {
	require.Equal(t, onlineCommittee.Size(), len(expectedOnlineSeats))
	for _, seatIndex := range expectedOnlineSeats {
		require.Truef(t, onlineCommittee.Has(seatIndex), "expected account %s to be part of committee, but it is not, actual committee members: %s", seatIndex, onlineCommittee)
	}
}
