package account

import (
	"bytes"
	"io"
	"sort"

	"github.com/iotaledger/hive.go/core/safemath"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Accounts represent a collection of accounts and their pools.
type Accounts struct {
	accountPools *shrinkingmap.ShrinkingMap[iotago.AccountID, *Pool]

	totalStake          iotago.BaseToken
	totalValidatorStake iotago.BaseToken

	mutex syncutils.RWMutex
}

// NewAccounts creates a new Weights instance.
func NewAccounts() *Accounts {
	return &Accounts{
		accountPools: shrinkingmap.New[iotago.AccountID, *Pool](),
	}
}

func (a *Accounts) Has(id iotago.AccountID) bool {
	return a.accountPools.Has(id)
}

func (a *Accounts) Size() int {
	return a.accountPools.Size()
}

func (a *Accounts) IDs() []iotago.AccountID {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	ids := make([]iotago.AccountID, 0, a.accountPools.Size())
	a.accountPools.ForEachKey(func(id iotago.AccountID) bool {
		ids = append(ids, id)
		return true
	})

	return ids
}

// Get returns the weight of the given identity.
func (a *Accounts) Get(id iotago.AccountID) (pool *Pool, exists bool) {
	return a.accountPools.Get(id)
}

// setWithoutLocking sets the weight of the given identity.
func (a *Accounts) setWithoutLocking(id iotago.AccountID, pool *Pool) error {
	value, created := a.accountPools.GetOrCreate(id, func() *Pool {
		return pool
	})

	var safeMathErr error

	if !created {
		// if there was already an entry, we need to subtract the former
		// stake first and set the new value
		if a.totalStake, safeMathErr = safemath.SafeSub(a.totalStake, value.PoolStake); safeMathErr != nil {
			return ierrors.Wrapf(safeMathErr, "failed to subtract pool stake from total stake for account %s", id.String())
		}

		if a.totalValidatorStake, safeMathErr = safemath.SafeSub(a.totalValidatorStake, value.ValidatorStake); safeMathErr != nil {
			return ierrors.Wrapf(safeMathErr, "failed to subtract validator stake from total validator stake for account %s", id.String())
		}

		a.accountPools.Set(id, pool)
	}

	if a.totalStake, safeMathErr = safemath.SafeAdd(a.totalStake, pool.PoolStake); safeMathErr != nil {
		return ierrors.Wrapf(safeMathErr, "failed to add pool stake to total stake for account %s", id.String())
	}

	if a.totalValidatorStake, safeMathErr = safemath.SafeAdd(a.totalValidatorStake, pool.ValidatorStake); safeMathErr != nil {
		return ierrors.Wrapf(safeMathErr, "failed to add validator stake to total validator stake for account %s", id.String())
	}

	return nil
}

// Set sets the weight of the given identity.
func (a *Accounts) Set(id iotago.AccountID, pool *Pool) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	return a.setWithoutLocking(id, pool)
}

func (a *Accounts) TotalStake() iotago.BaseToken {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	return a.totalStake
}

func (a *Accounts) TotalValidatorStake() iotago.BaseToken {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	return a.totalValidatorStake
}

// ForEach iterates over all weights and calls the given callback for each of them.
func (a *Accounts) ForEach(callback func(id iotago.AccountID, pool *Pool) bool) {
	a.accountPools.ForEach(callback)
}

// SeatedAccounts creates a new SeatedAccounts instance,
// that maintains the seat indices of re-elected members from the previous committee, if provided.
func (a *Accounts) SeatedAccounts(optPrevCommittee ...*SeatedAccounts) *SeatedAccounts {
	// If optPrevCommittee not given, use empty SeatedAccounts instance as default to skip the behavior to handle re-elected members.
	prevCommittee := NewSeatedAccounts(NewAccounts())
	if len(optPrevCommittee) > 0 {
		prevCommittee = optPrevCommittee[0]
	}

	newCommittee := NewSeatedAccounts(a)

	// If previous committee exists the seats need to be assigned and re-elected committee members must retain their SeatIndex.
	// Keep track of re-elected members' seat indices.
	takenSeatIndices := ds.NewSet[SeatIndex]()

	// Assign re-elected members' SeatIndex.
	a.ForEach(func(id iotago.AccountID, _ *Pool) bool {
		if seatIndex, seatExists := prevCommittee.GetSeat(id); seatExists {
			newCommittee.Set(seatIndex, id)
			takenSeatIndices.Add(seatIndex)
		}

		return true
	})

	// Sort members lexicographically.
	committeeAccountIDs := a.IDs()
	sort.Slice(committeeAccountIDs, func(i int, j int) bool {
		return bytes.Compare(committeeAccountIDs[i][:], committeeAccountIDs[j][:]) < 0
	})

	// Assign SeatIndex to new committee members.
	currentSeatIndex := SeatIndex(0)
	for _, memberID := range committeeAccountIDs {
		// If SeatIndex is taken by re-elected member, then increment it.
		for takenSeatIndices.Has(currentSeatIndex) {
			currentSeatIndex++
		}

		// Assign SeatIndex of a fresh committee member.
		if _, seatExists := prevCommittee.GetSeat(memberID); !seatExists {
			newCommittee.Set(currentSeatIndex, memberID)

			currentSeatIndex++
		}
	}

	return newCommittee
}

func AccountsFromBytes(b []byte) (*Accounts, int, error) {
	reader := stream.NewByteReader(b)

	a, err := AccountsFromReader(reader)
	if err != nil {
		return nil, 0, ierrors.Wrap(err, "unable to read accounts from bytes")
	}

	return a, reader.BytesRead(), nil
}

func AccountsFromReader(reader io.Reader) (*Accounts, error) {
	a := NewAccounts()

	if err := stream.ReadCollection(reader, serializer.SeriLengthPrefixTypeAsUint32, func(i int) error {
		accountID, err := stream.Read[iotago.AccountID](reader)
		if err != nil {
			return ierrors.Wrapf(err, "unable to read accountID at index %d", i)
		}

		pool, err := stream.ReadObject(reader, poolBytesLength, PoolFromBytes)
		if err != nil {
			return ierrors.Wrapf(err, "unable to read pool at index %d", i)
		}

		if err := a.setWithoutLocking(accountID, pool); err != nil {
			return ierrors.Wrapf(err, "failed to set pool for account %s", accountID.String())
		}

		return nil
	}); err != nil {
		return nil, ierrors.Wrap(err, "failed to read account data")
	}

	return a, nil
}

func (a *Accounts) Bytes() ([]byte, error) {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	byteBuffer := stream.NewByteBuffer()

	if err := stream.WriteCollection(byteBuffer, serializer.SeriLengthPrefixTypeAsUint32, func() (elementsCount int, err error) {
		var innerErr error
		a.ForEach(func(id iotago.AccountID, pool *Pool) bool {
			if innerErr = stream.Write(byteBuffer, id); innerErr != nil {
				return false
			}

			if innerErr = stream.WriteObject(byteBuffer, pool, (*Pool).Bytes); innerErr != nil {
				return false
			}

			return true
		})
		if innerErr != nil {
			return 0, innerErr
		}

		return a.accountPools.Size(), nil
	}); err != nil {
		return nil, ierrors.Wrap(err, "failed to write accounts")
	}

	return byteBuffer.Bytes()
}
