package prunable

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/core/storable"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

// PerformanceFactors represent the performance factors of a validator for a given slot.
type PerformanceFactors struct {
	slot  iotago.SlotIndex
	store kvstore.KVStore
}

// NewPerformanceFactors is a constructor for the PerformanceFactors.
func NewPerformanceFactors(slot iotago.SlotIndex, store kvstore.KVStore) *PerformanceFactors {
	return &PerformanceFactors{
		slot:  slot,
		store: store,
	}
}

func (p *PerformanceFactors) Load(accountID iotago.AccountID) (pf uint64, err error) {
	performanceFactorsBytes, err := p.store.Get(accountID[:])
	if err != nil {
		if errors.Is(err, kvstore.ErrKeyNotFound) {
			return 0, nil
		}

		return 0, errors.Wrapf(err, "failed to get performance factor for account %s", accountID)
	}

	var serializablePf storable.SerializableInt64
	_, err = serializablePf.FromBytes(performanceFactorsBytes)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to deserialize performance factor for account %s", accountID)
	}

	return uint64(serializablePf), nil
}

func (p *PerformanceFactors) Store(accountID iotago.AccountID, pf uint64) error {
	return p.store.Set(lo.PanicOnErr(accountID.Bytes()), lo.PanicOnErr(storable.SerializableInt64(pf).Bytes()))
}

// ForEachPerformanceFactor iterates over all saved validators. Note that this stream function won't call the consumer function for inactive validators, so better to use the load over
// every member of the committee if needed.
func (p *PerformanceFactors) ForEachPerformanceFactor(consumer func(accountID iotago.AccountID, pf uint64) error) error {
	var innerErr error
	if err := p.store.Iterate(kvstore.EmptyPrefix, func(key kvstore.Key, value kvstore.Value) bool {
		accountID, _, err := iotago.IdentifierFromBytes(key)
		if err != nil {
			innerErr = err
			return false
		}

		serializablePf := new(storable.SerializableInt64)
		_, err = serializablePf.FromBytes(value)
		if err != nil {
			innerErr = err
			return false
		}

		return consumer(accountID, uint64(*serializablePf)) != nil
	}); err != nil {
		return errors.Wrapf(err, "failed to stream performance factors for slot %s", p.slot)
	}

	if innerErr != nil {
		return errors.Wrapf(innerErr, "failed to deserialize performance factor for slot %s", p.slot)
	}

	return nil
}
