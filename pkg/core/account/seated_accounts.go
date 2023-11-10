package account

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/stringify"
	iotago "github.com/iotaledger/iota.go/v4"
)

type SeatIndex uint8

func (s SeatIndex) Bytes() ([]byte, error) {
	return []byte{byte(s)}, nil
}

func SeatIndexFromBytes(b []byte) (SeatIndex, int, error) {
	if len(b) != 1 {
		return 0, 0, ierrors.New("invalid seat index bytes length")
	}

	return SeatIndex(b[0]), 1, nil
}

type SeatedAccounts struct {
	accounts       *Accounts
	seatsByAccount *shrinkingmap.ShrinkingMap[iotago.AccountID, SeatIndex]
}

func NewSeatedAccounts(accounts *Accounts, optMembers ...iotago.AccountID) *SeatedAccounts {
	s := &SeatedAccounts{
		accounts:       accounts,
		seatsByAccount: shrinkingmap.New[iotago.AccountID, SeatIndex](),
	}
	sort.Slice(optMembers, func(i int, j int) bool {
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

func (s *SeatedAccounts) Accounts() (*Accounts, error) {
	accounts := NewAccounts()
	var err error
	s.seatsByAccount.ForEachKey(func(id iotago.AccountID) bool {
		pool, exists := s.accounts.Get(id)
		if !exists {
			panic("account not found")
		}
		if err = accounts.Set(id, pool); err != nil {
			return false
		}

		return true
	})

	return accounts, err
}

func (s *SeatedAccounts) String() string {
	builder := stringify.NewStructBuilder("SeatedAccounts")

	for accountID, seat := range s.seatsByAccount.AsMap() {
		builder.AddField(stringify.NewStructField(fmt.Sprintf("seat%d", seat), accountID))
	}

	return builder.String()
}
