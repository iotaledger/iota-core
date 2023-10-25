package topstakers

import (
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
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func TestTopStakers_InitializeCommittee(t *testing.T) {
	committeeStore := epochstore.NewStore(kvstore.Realm{}, kvstore.Realm{}, mapdb.NewMapDB(), 0, (*account.Accounts).Bytes, account.AccountsFromBytes)

	topStakersSeatManager := &SeatManager{
		apiProvider:     api.SingleVersionProvider(tpkg.TestAPI),
		committeeStore:  committeeStore,
		events:          seatmanager.NewEvents(),
		activityTracker: activitytrackerv1.NewActivityTracker(time.Second * 30),

		optsSeatCount: 3,
	}

	// Create committee for epoch 0
	initialCommittee := account.NewAccounts()
	for i := 0; i < 3; i++ {
		initialCommittee.Set(tpkg.RandAccountID(), &account.Pool{
			PoolStake:      1900,
			ValidatorStake: 900,
			FixedCost:      11,
		})
	}
	// Try setting committee that is too small - should return an error.
	err := topStakersSeatManager.SetCommittee(0, initialCommittee)
	require.NoError(t, err)
	weightedSeats, exists := topStakersSeatManager.CommitteeInEpoch(0)
	require.True(t, exists)
	initialCommitteeAccountIDs := initialCommittee.IDs()

	// Make sure that the online committee is handled correctly.
	require.True(t, topStakersSeatManager.OnlineCommittee().IsEmpty())

	require.NoError(t, topStakersSeatManager.InitializeCommittee(0, time.Time{}))
	assertOnlineCommittee(t, topStakersSeatManager.OnlineCommittee(), lo.Return1(weightedSeats.GetSeat(initialCommitteeAccountIDs[0])), lo.Return1(weightedSeats.GetSeat(initialCommitteeAccountIDs[2])), lo.Return1(weightedSeats.GetSeat(initialCommitteeAccountIDs[2])))

}

func TestTopStakers_RotateCommittee(t *testing.T) {
	committeeStore := epochstore.NewStore(kvstore.Realm{}, kvstore.Realm{}, mapdb.NewMapDB(), 0, (*account.Accounts).Bytes, account.AccountsFromBytes)

	topStakersSeatManager := &SeatManager{
		apiProvider:     api.SingleVersionProvider(tpkg.TestAPI),
		committeeStore:  committeeStore,
		events:          seatmanager.NewEvents(),
		activityTracker: activitytrackerv1.NewActivityTracker(time.Second * 30),

		optsSeatCount: 3,
	}

	// Committee should not exist because it was never set.
	_, exists := topStakersSeatManager.CommitteeInSlot(10)
	require.False(t, exists)

	_, exists = topStakersSeatManager.CommitteeInEpoch(0)
	require.False(t, exists)

	// Create committee for epoch 0
	initialCommittee := account.NewAccounts()
	initialCommittee.Set(tpkg.RandAccountID(), &account.Pool{
		PoolStake:      1900,
		ValidatorStake: 900,
		FixedCost:      11,
	})

	initialCommittee.Set(tpkg.RandAccountID(), &account.Pool{
		PoolStake:      1900,
		ValidatorStake: 900,
		FixedCost:      11,
	})

	// Try setting committee that is too small - should return an error.
	err := topStakersSeatManager.SetCommittee(0, initialCommittee)
	require.Error(t, err)

	initialCommittee.Set(tpkg.RandAccountID(), &account.Pool{
		PoolStake:      1900,
		ValidatorStake: 900,
		FixedCost:      11,
	})

	// Set committee with the correct size
	err = topStakersSeatManager.SetCommittee(0, initialCommittee)
	require.NoError(t, err)
	weightedSeats, exists := topStakersSeatManager.CommitteeInEpoch(0)
	require.True(t, exists)
	initialCommitteeAccountIDs := initialCommittee.IDs()

	// Make sure that the online committee is handled correctly.
	require.True(t, topStakersSeatManager.OnlineCommittee().IsEmpty())

	topStakersSeatManager.activityTracker.MarkSeatActive(lo.Return1(weightedSeats.GetSeat(initialCommitteeAccountIDs[0])), initialCommitteeAccountIDs[0], tpkg.TestAPI.TimeProvider().SlotStartTime(1))
	assertOnlineCommittee(t, topStakersSeatManager.OnlineCommittee(), lo.Return1(weightedSeats.GetSeat(initialCommitteeAccountIDs[0])))

	topStakersSeatManager.activityTracker.MarkSeatActive(lo.Return1(weightedSeats.GetSeat(initialCommitteeAccountIDs[1])), initialCommitteeAccountIDs[1], tpkg.TestAPI.TimeProvider().SlotStartTime(2))
	assertOnlineCommittee(t, topStakersSeatManager.OnlineCommittee(), lo.Return1(weightedSeats.GetSeat(initialCommitteeAccountIDs[0])), lo.Return1(weightedSeats.GetSeat(initialCommitteeAccountIDs[1])))

	topStakersSeatManager.activityTracker.MarkSeatActive(lo.Return1(weightedSeats.GetSeat(initialCommitteeAccountIDs[2])), initialCommitteeAccountIDs[2], tpkg.TestAPI.TimeProvider().SlotStartTime(3))
	assertOnlineCommittee(t, topStakersSeatManager.OnlineCommittee(), lo.Return1(weightedSeats.GetSeat(initialCommitteeAccountIDs[0])), lo.Return1(weightedSeats.GetSeat(initialCommitteeAccountIDs[1])), lo.Return1(weightedSeats.GetSeat(initialCommitteeAccountIDs[2])))

	// Make sure that after a period of inactivity, the inactive seats are marked as offline.
	topStakersSeatManager.activityTracker.MarkSeatActive(lo.Return1(weightedSeats.GetSeat(initialCommitteeAccountIDs[2])), initialCommitteeAccountIDs[2], tpkg.TestAPI.TimeProvider().SlotEndTime(7))
	assertOnlineCommittee(t, topStakersSeatManager.OnlineCommittee(), lo.Return1(weightedSeats.GetSeat(initialCommitteeAccountIDs[2])))

	// Make sure that the committee was assigned to the correct epoch.
	_, exists = topStakersSeatManager.CommitteeInEpoch(1)
	require.False(t, exists)

	// Make sure that the committee members match the expected ones.
	committee, exists := topStakersSeatManager.CommitteeInEpoch(0)
	require.True(t, exists)
	assertCommittee(t, initialCommittee, committee)

	committee, exists = topStakersSeatManager.CommitteeInSlot(3)
	require.True(t, exists)
	assertCommittee(t, initialCommittee, committee)

	// Design candidate list and expected committee members.
	accountsContext := make(accounts.AccountsData, 0)
	expectedCommittee := account.NewAccounts()
	numCandidates := 10

	// Add some candidates that have the same fields to test sorting by secondary fields.
	candidate1ID := tpkg.RandAccountID()
	accountsContext = append(accountsContext, &accounts.AccountData{
		ID:              candidate1ID,
		ValidatorStake:  399,
		DelegationStake: 800 - 399,
		FixedCost:       3,
		StakeEndEpoch:   iotago.MaxEpochIndex,
	})

	candidate2ID := tpkg.RandAccountID()
	accountsContext = append(accountsContext, &accounts.AccountData{
		ID:              candidate2ID,
		ValidatorStake:  399,
		DelegationStake: 800 - 399,
		FixedCost:       3,
		StakeEndEpoch:   iotago.MaxEpochIndex,
	})

	for i := 1; i <= numCandidates; i++ {
		candidateAccountID := tpkg.RandAccountID()
		candidatePool := &account.Pool{
			PoolStake:      iotago.BaseToken(i * 100),
			ValidatorStake: iotago.BaseToken(i * 50),
			FixedCost:      iotago.Mana(i),
		}
		accountsContext = append(accountsContext, &accounts.AccountData{
			ID:              candidateAccountID,
			ValidatorStake:  iotago.BaseToken(i * 50),
			DelegationStake: iotago.BaseToken(i*100) - iotago.BaseToken(i*50),
			FixedCost:       tpkg.RandMana(iotago.MaxMana),
			StakeEndEpoch:   tpkg.RandEpoch(),
		})

		if i+topStakersSeatManager.SeatCount() > numCandidates {
			expectedCommittee.Set(candidateAccountID, candidatePool)
		}
	}

	// Rotate the committee and make sure that the returned committee matches the expected.
	newCommittee, err := topStakersSeatManager.RotateCommittee(1, accountsContext)
	require.NoError(t, err)
	assertCommittee(t, expectedCommittee, newCommittee)

	// Make sure that after committee rotation, the online committee is not changed.
	assertOnlineCommittee(t, topStakersSeatManager.OnlineCommittee(), lo.Return1(weightedSeats.GetSeat(initialCommitteeAccountIDs[2])))

	newCommitteeMemberIDs := newCommittee.Accounts().IDs()

	// A new committee member appears online and makes the previously active committee seat inactive.
	topStakersSeatManager.activityTracker.MarkSeatActive(lo.Return1(weightedSeats.GetSeat(newCommitteeMemberIDs[0])), newCommitteeMemberIDs[0], tpkg.TestAPI.TimeProvider().SlotEndTime(14))
	assertOnlineCommittee(t, topStakersSeatManager.OnlineCommittee(), lo.Return1(weightedSeats.GetSeat(newCommitteeMemberIDs[0])))

	// Make sure that the committee retrieved from the committee store matches the expected.
	committee, exists = topStakersSeatManager.CommitteeInEpoch(1)
	require.True(t, exists)
	assertCommittee(t, expectedCommittee, committee)

	committee, exists = topStakersSeatManager.CommitteeInSlot(tpkg.TestAPI.TimeProvider().EpochStart(1))
	require.True(t, exists)
	assertCommittee(t, expectedCommittee, committee)

	// Make sure that the previous committee was not modified and is still accessible.
	committee, exists = topStakersSeatManager.CommitteeInEpoch(0)
	require.True(t, exists)
	assertCommittee(t, initialCommittee, committee)

	committee, exists = topStakersSeatManager.CommitteeInSlot(tpkg.TestAPI.TimeProvider().EpochEnd(0))
	require.True(t, exists)
	assertCommittee(t, initialCommittee, committee)
}

func assertCommittee(t *testing.T, expectedCommittee *account.Accounts, actualCommittee *account.SeatedAccounts) {
	require.Equal(t, actualCommittee.Accounts().Size(), expectedCommittee.Size())
	for _, memberID := range expectedCommittee.IDs() {
		require.Truef(t, actualCommittee.Accounts().Has(memberID), "expected account %s to be part of committee, but it is not, actual committee members: %s", memberID, actualCommittee.Accounts().IDs())
	}
}

func assertOnlineCommittee(t *testing.T, onlineCommittee ds.Set[account.SeatIndex], expectedOnlineSeats ...account.SeatIndex) {
	require.Equal(t, onlineCommittee.Size(), len(expectedOnlineSeats))
	for _, seatIndex := range expectedOnlineSeats {
		require.Truef(t, onlineCommittee.Has(seatIndex), "expected account %s to be part of committee, but it is not, actual committee members: %s", seatIndex, onlineCommittee)
	}
}
