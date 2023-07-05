package account

import (
	"bytes"
	"sort"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	iotago "github.com/iotaledger/iota.go/v4"
)

type SeatIndex int

type SeatedAccounts struct {
	accounts       *Accounts
	seatsByAccount *shrinkingmap.ShrinkingMap[iotago.AccountID, SeatIndex]
}

func NewSeatedAccounts(accounts *Accounts, optMembers ...iotago.AccountID) *SeatedAccounts {
	s := &SeatedAccounts{
		accounts:       accounts,
		seatsByAccount: shrinkingmap.New[iotago.AccountID, SeatIndex](),
	}
	sort.Slice(optMembers, func(i, j int) bool {
		return bytes.Compare(optMembers[i][:], optMembers[j][:]) < 0
	})

	for i, member := range optMembers {
		s.seatsByAccount.Set(member, SeatIndex(i))
	}

	return s
}

func (s *SeatedAccounts) Set(seat SeatIndex, id iotago.AccountID) bool {
	// Check if the account exists.
	if _, exists := s.accounts.Get(id); !exists {
		return false
	}

	// Check if the account already has a seat.
	if oldSeat, exists := s.seatsByAccount.Get(id); exists {
		if oldSeat != seat {
			return false
		}
	}

	return s.seatsByAccount.Set(id, seat)
}

func (s *SeatedAccounts) Delete(id iotago.AccountID) bool {
	return s.seatsByAccount.Delete(id)
}

func (s *SeatedAccounts) GetSeat(id iotago.AccountID) (seat SeatIndex, exists bool) {
	return s.seatsByAccount.Get(id)
}

func (s *SeatedAccounts) HasAccount(id iotago.AccountID) (has bool) {
	return s.seatsByAccount.Has(id)
}

func (s *SeatedAccounts) SeatCount() int {
	return s.seatsByAccount.Size()
}

func (s *SeatedAccounts) Accounts() *Accounts {
	accounts := NewAccounts()
	s.seatsByAccount.ForEach(func(id iotago.AccountID, index SeatIndex) bool {
		pool, exists := s.accounts.Get(id)
		if !exists {
			panic("account not found")
		}
		accounts.Set(id, pool)

		return true
	})

	return accounts
}
