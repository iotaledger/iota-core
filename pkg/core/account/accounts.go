package account

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Accounts represents a collection of accounts and their pools.
type Accounts struct {
	accountPools *shrinkingmap.ShrinkingMap[iotago.AccountID, *Pool]

	totalStake          iotago.BaseToken
	totalValidatorStake iotago.BaseToken
}

func AccountsFromBytes(bytes []byte) (*Accounts, error) {
	a := NewAccounts()
	_, err := a.FromBytes(bytes)
	if err != nil {
		return nil, err
	}

	return a, nil
}

// NewAccounts creates a new Weights instance.
func NewAccounts() *Accounts {
	a := new(Accounts)
	a.initialize()

	return a
}

func (w *Accounts) initialize() {
	w.accountPools = shrinkingmap.New[iotago.AccountID, *Pool]()
}

func (w *Accounts) Has(id iotago.AccountID) bool {
	_, has := w.accountPools.Get(id)
	return has
}

func (w *Accounts) Size() int {
	return w.accountPools.Size()
}

func (w *Accounts) IDs() []iotago.AccountID {
	ids := make([]iotago.AccountID, 0, w.accountPools.Size())
	w.ForEach(func(id iotago.AccountID, pool *Pool) bool {
		ids = append(ids, id)
		return true
	})

	return ids
}

// Get returns the weight of the given identity.
func (w *Accounts) Get(id iotago.AccountID) (pool *Pool, exists bool) {
	return w.accountPools.Get(id)
}

// Set sets the weight of the given identity.
func (w *Accounts) Set(id iotago.AccountID, pool *Pool) {
	w.accountPools.Set(id, pool)

	w.totalStake += pool.PoolStake
	w.totalValidatorStake += pool.ValidatorStake

	return
}

func (w *Accounts) TotalStake() iotago.BaseToken {
	return w.totalStake
}

func (w *Accounts) TotalValidatorStake() iotago.BaseToken {
	return w.totalValidatorStake
}

// ForEach iterates over all weights and calls the given callback for each of them.
func (w *Accounts) ForEach(callback func(id iotago.AccountID, pool *Pool) bool) {
	w.accountPools.ForEach(callback)
}

// SelectCommittee creates a new SeatedAccounts instance, that maintains the seats of the given members.
func (w *Accounts) SelectCommittee(members ...iotago.AccountID) *SeatedAccounts {
	return NewSeatedAccounts(w, members...)
}

func (w *Accounts) FromBytes(bytes []byte) (n int, err error) {
	w.initialize()
	m := marshalutil.New(bytes)

	count, err := m.ReadUint32()
	if err != nil {
		return m.ReadOffset(), errors.Wrap(err, "failed to read count")
	}

	for i := uint32(0); i < count; i++ {
		accountIDBytes, err := m.ReadBytes(iotago.AccountIDLength)
		if err != nil {
			return m.ReadOffset(), errors.Wrap(err, "failed to read account id")
		}
		poolBytes, err := m.ReadBytes(poolBytesLength)
		if err != nil {
			return m.ReadOffset(), errors.Wrap(err, "failed to read pool")
		}

		pool := new(Pool)
		if _, err := pool.FromBytes(poolBytes); err != nil {
			return m.ReadOffset(), errors.Wrap(err, "failed to parse pool")
		}

		w.Set(iotago.AccountID(accountIDBytes), pool)
	}

	return m.ReadOffset(), nil
}

func (w *Accounts) Bytes() (bytes []byte, err error) {
	m := marshalutil.New()

	m.WriteUint32(uint32(w.accountPools.Size()))

	var innerErr error
	w.ForEach(func(id iotago.AccountID, pool *Pool) bool {
		m.WriteBytes(id[:])
		poolBytes, err := pool.Bytes()
		if err != nil {
			innerErr = err
			return false
		}
		m.WriteBytes(poolBytes)

		return true
	})

	if innerErr != nil {
		return nil, innerErr
	}

	return m.Bytes(), nil
}
