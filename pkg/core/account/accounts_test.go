package account_test

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/core/account"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestAccounts(t *testing.T) {
	issuersCount := 10

	issuers, totalStake := generateAccounts(issuersCount)

	accounts := account.NewAccounts()

	// check "Set"
	for id, stake := range issuers {
		accounts.Set(id, &account.Pool{
			PoolStake:      iotago.BaseToken(stake),
			ValidatorStake: iotago.BaseToken(stake) * 2,
			FixedCost:      iotago.Mana(stake) * 3,
		})
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
	_, exists := accounts.Get([32]byte{0, randByte()})
	require.False(t, exists)

	for id, stake := range issuers {
		// check "Has"
		require.True(t, accounts.Has(id))

		// check "Get"
		account, exists := accounts.Get(id)
		require.True(t, exists)

		// check the account itself
		require.Equal(t, stake, account.PoolStake)
		require.Equal(t, stake*2, account.ValidatorStake)
		require.Equal(t, iotago.Mana(stake)*3, account.FixedCost)
	}

	// check total stakes
	require.Equal(t, totalStake, accounts.TotalStake())
	require.Equal(t, totalStake*2, accounts.TotalValidatorStake())

	// update an existing accounts stake
	account1, exists := accounts.Get(accountIDs[0])
	require.True(t, exists)

	accounts.Set(accountIDs[0], &account.Pool{
		PoolStake:      10 * account1.PoolStake,
		ValidatorStake: 10 * account1.ValidatorStake,
		FixedCost:      10 * account1.FixedCost,
	})

	// check if the updated total stakes are correct
	require.Equal(t, (totalStake-account1.PoolStake)+(account1.PoolStake*10), accounts.TotalStake())
	require.Equal(t, (totalStake*2-account1.ValidatorStake)+(account1.ValidatorStake*10), accounts.TotalValidatorStake())

	// check "Bytes"
	accountBytes, err := accounts.Bytes()
	require.NoError(t, err)

	// check "FromBytes"
	accounts2 := account.NewAccounts()
	bytesRead, err := accounts2.FromBytes(accountBytes)
	require.NoError(t, err)

	// check if we read all the bytes
	require.Equal(t, len(accountBytes), bytesRead)

	// check if the new account is the same
	require.Equal(t, accounts, accounts2)

	// check "FromReader"
	accounts3 := account.NewAccounts()
	require.NoError(t, accounts3.FromReader(bytes.NewReader(accountBytes)))

	// check if the new account is the same
	require.Equal(t, accounts, accounts3)

	// check "SelectCommittee"

	// get 1 issuer
	seated := accounts.SelectCommittee(accountIDs[0])
	require.Equal(t, 1, seated.SeatCount())

	// get all issuers
	seated = accounts.SelectCommittee(accountIDs...)
	require.Equal(t, len(accountIDs), seated.SeatCount())

	/*

		// Get a non existed account
		_, exist = accounts.Get([32]byte{randByte()})
		require.False(t, exist)

	*/
}

func generateAccounts(count int) (map[iotago.AccountID]iotago.BaseToken, iotago.BaseToken) {
	seenIDs := make(map[iotago.AccountID]bool)
	issuers := make(map[iotago.AccountID]iotago.BaseToken)

	var totalStake iotago.BaseToken

	for i := 0; i < count; i++ {
		id := iotago.AccountID([32]byte{randByte()})
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

func randByte() byte {
	return byte(rand.Intn(256))
}
