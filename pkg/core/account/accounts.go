package account

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Accounts represent a collection of accounts and their pools.
type Accounts struct {
	accountPools *shrinkingmap.ShrinkingMap[iotago.AccountID, *Pool]

	totalStake          iotago.BaseToken
	totalValidatorStake iotago.BaseToken
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
	_, has := a.accountPools.Get(id)
	return has
}

func (a *Accounts) Size() int {
	return a.accountPools.Size()
}

func (a *Accounts) IDs() []iotago.AccountID {
	ids := make([]iotago.AccountID, 0, a.accountPools.Size())
	a.ForEach(func(id iotago.AccountID, pool *Pool) bool {
		ids = append(ids, id)
		return true
	})

	return ids
}

// Get returns the weight of the given identity.
func (a *Accounts) Get(id iotago.AccountID) (pool *Pool, exists bool) {
	return a.accountPools.Get(id)
}

// Set sets the weight of the given identity.
func (a *Accounts) Set(id iotago.AccountID, pool *Pool) {
	a.accountPools.Set(id, pool)

	a.totalStake += pool.PoolStake
	a.totalValidatorStake += pool.ValidatorStake
}

func (a *Accounts) TotalStake() iotago.BaseToken {
	return a.totalStake
}

func (a *Accounts) TotalValidatorStake() iotago.BaseToken {
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

func (a *Accounts) FromBytes(b []byte) (n int, err error) {
	return a.readFromReadSeeker(bytes.NewReader(b))
}

func (a *Accounts) FromReader(readSeeker io.ReadSeeker) error {
	return lo.Return2(a.readFromReadSeeker(readSeeker))
}

func (a *Accounts) readFromReadSeeker(reader io.ReadSeeker) (n int, err error) {
	a.initialize()

	var accountCount uint32
	if err = binary.Read(reader, binary.LittleEndian, &accountCount); err != nil {
		return n, errors.Wrap(err, "unable to read accounts count")
	}
	n += 4

	for i := uint32(0); i < accountCount; i++ {
		var accountID iotago.AccountID

		if _, err = io.ReadFull(reader, accountID[:]); err != nil {
			return 0, errors.Wrap(err, "unable to read Account ID")
		}
		n += iotago.AccountIDLength

		poolBytes := make([]byte, poolBytesLength)
		if _, err = io.ReadFull(reader, poolBytes); err != nil {
			return 0, errors.Wrap(err, "unable to read pool bytes")
		}
		n += poolBytesLength

		pool := new(Pool)
		if _, err := pool.FromBytes(poolBytes); err != nil {
			return n, errors.Wrap(err, "failed to parse pool")
		}

		a.Set(accountID, pool)
	}

	return n, nil
}

func (a *Accounts) Bytes() (bytes []byte, err error) {
	m := marshalutil.New()

	m.WriteUint32(uint32(a.accountPools.Size()))
	var innerErr error
	a.ForEach(func(id iotago.AccountID, pool *Pool) bool {
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
