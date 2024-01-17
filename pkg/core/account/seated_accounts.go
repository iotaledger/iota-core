package account

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"sync/atomic"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
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
	reused         atomic.Bool
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

func (s *SeatedAccounts) IsReused() bool {
	return s.reused.Load()
}

func (s *SeatedAccounts) SetReused() {
	s.reused.Store(true)
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

func (s *SeatedAccounts) IDs() []iotago.AccountID {
	return s.accounts.IDs()
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

func (s *SeatedAccounts) Reuse() (*SeatedAccounts, error) {
	committeeAccounts, err := s.Accounts()
	if err != nil {
		return nil, err
	}

	newCommittee := NewSeatedAccounts(committeeAccounts)
	newCommittee.SetReused()

	s.seatsByAccount.ForEach(func(id iotago.AccountID, index SeatIndex) bool {
		newCommittee.Set(index, id)

		return true
	})

	return newCommittee, nil
}

func (s *SeatedAccounts) String() string {
	builder := stringify.NewStructBuilder("SeatedAccounts")

	for accountID, seat := range s.seatsByAccount.AsMap() {
		builder.AddField(stringify.NewStructField(fmt.Sprintf("seat%d", seat), accountID))
	}

	return builder.String()
}

func SeatedAccountsFromBytes(b []byte) (*SeatedAccounts, int, error) {
	reader := stream.NewByteReader(b)

	s, err := SeatedAccountsFromReader(reader)
	if err != nil {
		return nil, 0, ierrors.Wrap(err, "unable to read accounts from bytes")
	}

	return s, reader.BytesRead(), nil
}

func SeatedAccountsFromReader(reader io.Reader) (*SeatedAccounts, error) {
	accounts, err := AccountsFromReader(reader)
	if err != nil {
		return nil, ierrors.Wrap(err, "unable to read accounts from bytes")
	}

	seatsByAccount := shrinkingmap.New[iotago.AccountID, SeatIndex]()

	if err := stream.ReadCollection(reader, serializer.SeriLengthPrefixTypeAsUint32, func(i int) error {
		accountID, err := stream.Read[iotago.AccountID](reader)
		if err != nil {
			return ierrors.Wrapf(err, "unable to read accountID at index %d", i)
		}

		seatIndex, err := stream.Read[SeatIndex](reader)
		if err != nil {
			return ierrors.Wrapf(err, "unable to read seatIndex at index %d", i)
		}

		seatsByAccount.Set(accountID, seatIndex)

		return nil
	}); err != nil {
		return nil, ierrors.Wrap(err, "failed to read account data")
	}

	reused, err := stream.Read[bool](reader)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to read reused flag")
	}

	committee := &SeatedAccounts{accounts: accounts, seatsByAccount: seatsByAccount}

	committee.reused.Store(reused)

	return committee, nil
}

func (s *SeatedAccounts) Bytes() ([]byte, error) {
	byteBuffer := stream.NewByteBuffer()

	accountsBytes, err := s.accounts.Bytes()
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to serialize committee accounts")
	}

	if err := stream.WriteBytes(byteBuffer, accountsBytes); err != nil {
		return nil, ierrors.Wrap(err, "failed to write account bytes")
	}

	if err := stream.WriteCollection(byteBuffer, serializer.SeriLengthPrefixTypeAsUint32, func() (elementsCount int, err error) {
		var innerErr error
		s.seatsByAccount.ForEach(func(id iotago.AccountID, seatIndex SeatIndex) bool {
			if innerErr = stream.Write(byteBuffer, id); innerErr != nil {
				return false
			}

			if innerErr = stream.Write(byteBuffer, seatIndex); innerErr != nil {
				return false
			}

			return true
		})
		if innerErr != nil {
			return 0, innerErr
		}

		return s.seatsByAccount.Size(), nil
	}); err != nil {
		return nil, ierrors.Wrap(err, "failed to write seats by account map")
	}

	if err := stream.Write(byteBuffer, s.reused.Load()); err != nil {
		return nil, ierrors.Wrap(err, "failed to write reused flag")
	}

	return byteBuffer.Bytes()
}
