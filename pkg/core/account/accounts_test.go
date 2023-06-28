package account_test

import (
	"math/rand"
	"testing"

	iotago "github.com/iotaledger/iota.go/v4"
)

func TestAccounts(t *testing.T) {
	//accounts := account.NewAccounts()
	//issuers, totalWeight := generateAccounts()
	//
	//// Add accounts
	//for id, weight := range issuers {
	//	accounts.Set(id, weight)
	//}
	//
	//// Test Map
	//wMap, err := accounts.Map()
	//require.NoError(t, err)
	//require.EqualValues(t, issuers, wMap)
	//
	//// update issuer's weight
	//issuerIDs := lo.Keys(wMap)
	//oldWeight, exist := accounts.Get(issuerIDs[0])
	//require.True(t, exist)
	//
	//// Get a non existed account
	//_, exist = accounts.Get([32]byte{randByte()})
	//require.False(t, exist)
	//
	//// Test Selected Accounts, get 1 issuer
	//seated := accounts.SelectCommittee(issuerIDs[0])
	//require.Equal(t, 1, seated.SeatCount())
	//
	//// Test Selected Accounts, get all issuers
	//seated = accounts.SelectCommittee(issuerIDs...)
	//require.Equal(t, len(issuerIDs), seated.SeatCount())
}

func generateAccounts() (map[iotago.AccountID]int64, int64) {
	seenIDs := make(map[iotago.AccountID]bool)
	issuers := make(map[iotago.AccountID]int64)
	var totalWeight int64

	for i := 0; i < 10; i++ {
		id := iotago.AccountID([32]byte{randByte()})
		if _, exist := seenIDs[id]; exist {
			i--
			continue
		}
		issuers[id] = int64(i)
		totalWeight += int64(i)
		seenIDs[id] = true
	}

	return issuers, totalWeight
}

func randByte() byte {
	return byte(rand.Intn(256))
}
