package account_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/account"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func TestAccounts(t *testing.T) {
	issuersCount := 10

	issuers, totalStake := generateAccounts(issuersCount)

	accounts := account.NewAccounts()

	// check "Set"
	for id, stake := range issuers {
		require.NoError(t, accounts.Set(id, &account.Pool{
			PoolStake:      stake,
			ValidatorStake: iotago.BaseToken(stake) * 2,
			FixedCost:      iotago.Mana(stake) * 3,
		}))
	}

	// check "Size"
	require.Equal(t, issuersCount, accounts.Size())

	// check "IDs"
	accountIDs := accounts.IDs()
	require.Equal(t, issuersCount, len(accountIDs))
	for _, accountID := range accountIDs {
		_, exists := issuers[accountID]
		require.True(t, exists)
	}

	// check "ForEach"
	accounts.ForEach(func(id iotago.AccountID, pool *account.Pool) bool {
		_, exists := issuers[id]
		require.True(t, exists)

		return true
	})

	// check "Get" for non existing IDs
	_, exists := accounts.Get([32]byte{0, tpkg.RandByte()})
	require.False(t, exists)

	for id, stake := range issuers {
		// check "Has"
		require.True(t, accounts.Has(id))

		// check "Get"
		member, exists := accounts.Get(id)
		require.True(t, exists)

		// check the account itself
		require.Equal(t, stake, member.PoolStake)
		require.Equal(t, stake*2, member.ValidatorStake)
		require.Equal(t, iotago.Mana(stake)*3, member.FixedCost)
	}

	// check total stakes
	require.Equal(t, totalStake, accounts.TotalStake())
	require.Equal(t, totalStake*2, accounts.TotalValidatorStake())

	// update an existing accounts stake
	account1, exists := accounts.Get(accountIDs[0])
	require.True(t, exists)

	require.NoError(t, accounts.Set(accountIDs[0], &account.Pool{
		PoolStake:      10 * account1.PoolStake,
		ValidatorStake: 10 * account1.ValidatorStake,
		FixedCost:      10 * account1.FixedCost,
	}))

	// check if the updated total stakes are correct
	require.Equal(t, (totalStake-account1.PoolStake)+(account1.PoolStake*10), accounts.TotalStake())
	require.Equal(t, (totalStake*2-account1.ValidatorStake)+(account1.ValidatorStake*10), accounts.TotalValidatorStake())

	// check "Bytes"
	accountBytes, err := accounts.Bytes()
	require.NoError(t, err)

	// check "AccountsFromBytes"
	accounts2, bytesRead, err := account.AccountsFromBytes(accountBytes)
	require.NoError(t, err)

	// check if we read all the bytes
	require.Equal(t, len(accountBytes), bytesRead)

	// check if the new account is the same
	require.Equal(t, accounts, accounts2)

	// check "AccountsFromReader"
	accounts3, err := account.AccountsFromReader(bytes.NewReader(accountBytes))
	require.NoError(t, err)

	// check if the new account is the same
	require.Equal(t, accounts, accounts3)

	// check "SelectCommittee"

	// get all issuers
	seated := accounts.SeatedAccounts()
	require.Equal(t, len(accountIDs), seated.SeatCount())

	/*

		// Get a non existed account
		_, exist = accounts.Get([32]byte{tpkg.RandByte()})
		require.False(t, exist)

	*/
}

func TestAccounts_SeatedAccounts(t *testing.T) {
	committeeSize := 20

	prevCommitteeIssuers, _ := generateAccounts(committeeSize)
	newCommitteeIssuers, _ := generateAccounts(committeeSize/2 - 1)

	newCommitteeAccounts := account.NewAccounts()
	prevCommitteeAccounts := account.NewAccounts()

	idx := 0
	for id, stake := range prevCommitteeIssuers {
		// half of the accounts from the previous committee should also be added to the new committee
		if idx%2 == 0 || idx == 1 {
			require.NoError(t, newCommitteeAccounts.Set(id, &account.Pool{
				PoolStake:      stake,
				ValidatorStake: iotago.BaseToken(stake) * 2,
				FixedCost:      iotago.Mana(stake) * 3,
			}))
		}

		require.NoError(t, prevCommitteeAccounts.Set(id, &account.Pool{
			PoolStake:      stake,
			ValidatorStake: iotago.BaseToken(stake) * 2,
			FixedCost:      iotago.Mana(stake) * 3,
		}))

		idx++
	}

	// Add remaining accounts to new committee accounts.
	for id, stake := range newCommitteeIssuers {
		// half of the accounts from the previous committee should also be added to the new committee
		require.NoError(t, newCommitteeAccounts.Set(id, &account.Pool{
			PoolStake:      stake,
			ValidatorStake: iotago.BaseToken(stake) * 2,
			FixedCost:      iotago.Mana(stake) * 3,
		}))
	}

	// Get prev committee and assign seats by default.
	prevCommittee := prevCommitteeAccounts.SeatedAccounts()

	// New committee seats should be assigned taking previous committee into account.
	newCommittee := newCommitteeAccounts.SeatedAccounts(prevCommittee)

	prevCommitteeAccounts.ForEach(func(id iotago.AccountID, _ *account.Pool) bool {
		if seatIndex, exists := newCommittee.GetSeat(id); exists {
			require.Equal(t, lo.Return1(prevCommittee.GetSeat(id)), seatIndex)
		}

		return true
	})

	// Make sure that the size of the committee and SeatIndex values are sane.
	require.Equal(t, newCommittee.SeatCount(), committeeSize)

	// Check that SeatIndices are within <0; committeeSize) range.
	newCommitteeAccounts.ForEach(func(id iotago.AccountID, _ *account.Pool) bool {
		// SeatIndex must be smaller than CommitteeSize.
		require.Less(t, int(lo.Return1(newCommittee.GetSeat(id))), committeeSize)

		// SeatIndex must be bigger or equal to 0.
		require.GreaterOrEqual(t, int(lo.Return1(newCommittee.GetSeat(id))), 0)

		return true
	})
}

func generateAccounts(count int) (map[iotago.AccountID]iotago.BaseToken, iotago.BaseToken) {
	seenIDs := make(map[iotago.AccountID]bool)
	issuers := make(map[iotago.AccountID]iotago.BaseToken)

	var totalStake iotago.BaseToken

	for i := 0; i < count; i++ {
		id := iotago.AccountID(tpkg.Rand32ByteArray())
		if _, exist := seenIDs[id]; exist {
			i--
			continue
		}

		issuers[id] = iotago.BaseToken(i)
		totalStake += iotago.BaseToken(i)

		seenIDs[id] = true
	}

	return issuers, totalStake
}
