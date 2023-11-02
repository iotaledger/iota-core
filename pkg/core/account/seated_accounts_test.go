package account_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/core/account"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestSelectedAccounts(t *testing.T) {
	// Create a new set of accounts
	accounts := account.NewAccounts()

	// Add some accounts
	account1 := iotago.AccountID([32]byte{1})
	account2 := iotago.AccountID([32]byte{2})
	account3 := iotago.AccountID([32]byte{3})
	account4 := iotago.AccountID([32]byte{4})

	require.NoError(t, accounts.Set(account1, &account.Pool{}))
	require.NoError(t, accounts.Set(account2, &account.Pool{}))
	require.NoError(t, accounts.Set(account3, &account.Pool{}))

	// Create a new set of selected accounts
	seatedAccounts := account.NewSeatedAccounts(accounts, account1, account3)
	require.Equal(t, 2, seatedAccounts.SeatCount())

	// Test the "Set" method
	added := seatedAccounts.Set(account.SeatIndex(3), account2)
	require.True(t, added)
	require.Equal(t, 3, seatedAccounts.SeatCount())

	// Try adding an account again with a different seat.
	added = seatedAccounts.Set(account.SeatIndex(2), account2)
	require.False(t, added)
	require.Equal(t, 3, seatedAccounts.SeatCount())

	// Try adding an account that does not exist in accounts.
	added = seatedAccounts.Set(account.SeatIndex(4), account4)
	require.False(t, added)
	require.Equal(t, 3, seatedAccounts.SeatCount())

	// Test the "Delete" method
	removed := seatedAccounts.Delete(account1)
	require.True(t, removed)
	require.Equal(t, 2, seatedAccounts.SeatCount())

	// Try to get the deleted account
	_, exists := seatedAccounts.GetSeat(account1)
	require.False(t, exists)

	// Test the "Get" method
	seat, exists := seatedAccounts.GetSeat(account2)
	require.True(t, exists)
	require.Equal(t, account.SeatIndex(3), seat)

	// Test the "Get" method with account that's not in accounts.
	_, exists = seatedAccounts.GetSeat(account4)
	require.False(t, exists)

	// Test the "Has" method
	has := seatedAccounts.HasAccount(account3)
	require.True(t, has)

	// Test the "Members" method
	members, err := seatedAccounts.Accounts()
	require.NoError(t, err)
	require.Equal(t, 2, members.Size())
	require.True(t, members.Has(account2))
	require.True(t, members.Has(account3))
}
