package account

import (
	"bytes"
	"encoding/binary"
	"io"
	"sync/atomic"

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
	reused              atomic.Bool

	mutex syncutils.RWMutex
}

// NewAccounts creates a new Weights instance.
func NewAccounts() *Accounts {
	a := new(Accounts)
	a.initialize()

	return a
}

func (a *Accounts) initialize() {
	a.accountPools = shrinkingmap.New[iotago.AccountID, *Pool]()
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

func (a *Accounts) IsReused() bool {
	return a.reused.Load()
}

func (a *Accounts) SetReused() {
	a.reused.Store(true)
}

// Get returns the weight of the given identity.
func (a *Accounts) Get(id iotago.AccountID) (pool *Pool, exists bool) {
	return a.accountPools.Get(id)
}

// setWithoutLocking sets the weight of the given identity.
func (a *Accounts) setWithoutLocking(id iotago.AccountID, pool *Pool) {
	value, created := a.accountPools.GetOrCreate(id, func() *Pool {
		return pool
	})

	if !created {
		// if there was already an entry, we need to subtract the former
		// stake first and set the new value
		// TODO: use safemath
		a.totalStake -= value.PoolStake
		a.totalValidatorStake -= value.ValidatorStake

		a.accountPools.Set(id, pool)
	}

	a.totalStake += pool.PoolStake
	a.totalValidatorStake += pool.ValidatorStake
}

// Set sets the weight of the given identity.
func (a *Accounts) Set(id iotago.AccountID, pool *Pool) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	a.setWithoutLocking(id, pool)
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

// SelectCommittee creates a new SeatedAccounts instance, that maintains the seats of the given members.
func (a *Accounts) SelectCommittee(members ...iotago.AccountID) *SeatedAccounts {
	return NewSeatedAccounts(a, members...)
}

func AccountsFromBytes(b []byte) (*Accounts, int, error) {
	return AccountsFromReader(bytes.NewReader(b))
}

func AccountsFromReader(readSeeker io.ReadSeeker) (*Accounts, int, error) {
	a := new(Accounts)
	n, err := a.readFromReadSeeker(readSeeker)

	return a, n, err
}

func (a *Accounts) readFromReadSeeker(reader io.ReadSeeker) (n int, err error) {
	// TODO: improve this
	a.mutex.Lock()
	defer a.mutex.Unlock()

	a.initialize()

	var accountCount uint32
	if err = binary.Read(reader, binary.LittleEndian, &accountCount); err != nil {
		return n, ierrors.Wrap(err, "unable to read accounts count")
	}
	n += 4

	for i := uint32(0); i < accountCount; i++ {
		var accountID iotago.AccountID

		if _, err = io.ReadFull(reader, accountID[:]); err != nil {
			return 0, ierrors.Wrap(err, "unable to read accountID")
		}
		n += iotago.AccountIDLength

		poolBytes := make([]byte, poolBytesLength)
		if _, err = io.ReadFull(reader, poolBytes); err != nil {
			return 0, ierrors.Wrap(err, "unable to read pool bytes")
		}
		n += poolBytesLength

		pool, c, err := PoolFromBytes(poolBytes)
		if err != nil {
			return 0, ierrors.Wrap(err, "failed to parse pool")
		}

		if c != poolBytesLength {
			return 0, ierrors.Wrap(err, "invalid pool bytes length")
		}

		a.setWithoutLocking(accountID, pool)
	}

	var reused bool
	if err = binary.Read(reader, binary.LittleEndian, &reused); err != nil {
		return n, ierrors.Wrap(err, "unable to read reused flag")
	}
	a.reused.Store(reused)
	n++

	return n, nil
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

			if innerErr = stream.WriteFixedSizeObject(byteBuffer, pool, poolBytesLength, func(pool *Pool) ([]byte, error) {
				return pool.Bytes()
			}); innerErr != nil {
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

	if err := stream.Write(byteBuffer, a.reused.Load()); err != nil {
		return nil, ierrors.Wrap(err, "failed to write reused flag")
	}

	return byteBuffer.Bytes()
}
