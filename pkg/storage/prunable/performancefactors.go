package prunable

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/core/storable"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

// AccountDiff represent the storable changes for a single account within a slot.
type PerformanceFactors struct {
	slot  iotago.SlotIndex
	store kvstore.KVStore
}

// PerformanceFactors represent the storable changes for a single account within a slot.
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
	serializablePf.FromBytes(performanceFactorsBytes)

	return uint64(serializablePf), nil
}

func (p *PerformanceFactors) Store(accountID iotago.AccountID, pf uint64) error {
	p.store.Batched()
	return p.store.Set(lo.PanicOnErr(accountID.Bytes()), lo.PanicOnErr(storable.SerializableInt64(pf).Bytes()))
}

// TODO: this stream function won't call the consumer function for inactive validators, so better to use the load over
// every member of the committee

// func (b *PerformanceFactors) ForEachPerformanceFactor(consumer func(accountID iotago.AccountID, pf uint64) error) error {
// 	var innerErr error
// 	if err := b.store.Iterate(kvstore.EmptyPrefix, func(key kvstore.Key, value kvstore.Value) bool {
// 		var accountID iotago.AccountID
// 		if _, innerErr = accountID.FromBytes(key); innerErr != nil {
// 			return false
// 		}

// 		serializablePf := new(storable.SerializableInt64)
// 		serializablePf.FromBytes(value)

// 		return consumer(accountID, uint64(*serializablePf)) != nil
// 	}); err != nil {
// 		return errors.Wrapf(err, "failed to stream performance factors for slot %s", b.slot)
// 	}

// 	if innerErr != nil {
// 		return errors.Wrapf(innerErr, "failed to deserialize performance factor for slot %s", b.slot)
// 	}

// 	return nil
// }
