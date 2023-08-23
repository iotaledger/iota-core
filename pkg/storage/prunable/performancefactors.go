package prunable

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

// ValidatorSlotPerformance represent the performance factors of a validator for a given slot.
type ValidatorSlotPerformance struct {
	slot  iotago.SlotIndex
	store kvstore.KVStore

	apiProvider api.Provider
}

type ValidatorPerformance struct {
	// works if ValidatorBlocksPerSlot is less than 32 because we use it as bit vector
	SlotActivityVector uint32
	// can be uint8 because max count per slot is maximally ValidatorBlocksPerSlot + 1
	BlockIssuedCount               uint8
	HighestSupportedVersionAndHash iotago.VersionAndHash
}

func ValidatorPerformanceFromBytes(bytes []byte, slotAPI iotago.API) (*ValidatorPerformance, int, error) {
	p := &ValidatorPerformance{}
	offset, err := slotAPI.Decode(bytes, p)
	if err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to decode validator performance")
	}
	return p, offset, nil
}

// NewPerformanceFactors is a constructor for the ValidatorSlotPerformance.
func NewPerformanceFactors(slot iotago.SlotIndex, store kvstore.KVStore, apiProvider api.Provider) *ValidatorSlotPerformance {
	return &ValidatorSlotPerformance{
		slot:        slot,
		store:       store,
		apiProvider: apiProvider,
	}
}

func (p *ValidatorSlotPerformance) Load(accountID iotago.AccountID) (pf *ValidatorPerformance, err error) {
	performanceFactorsBytes, err := p.store.Get(accountID[:])
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return nil, nil
		}

		return nil, ierrors.Wrapf(err, "failed to get performance factor for account %s", accountID)
	}

	validatorPerformance, _, err := ValidatorPerformanceFromBytes(performanceFactorsBytes, p.apiProvider.APIForSlot(p.slot))
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to deserialize performance factor for account %s", accountID)
	}
	return validatorPerformance, nil
}

func (p *ValidatorSlotPerformance) Store(accountID iotago.AccountID, pf *ValidatorPerformance) error {
	bytes, err := p.apiProvider.APIForSlot(p.slot).Encode(pf)
	if err != nil {
		return ierrors.Wrapf(err, "failed to serialize performance factor for account %s", accountID)
	}
	return p.store.Set(lo.PanicOnErr(accountID.Bytes()), bytes)
}

// ForEachPerformanceFactor iterates over all saved validators.
// Note that this stream function won't call the consumer function for inactive validators,
// so better to use the load over every member of the committee if needed.
func (p *ValidatorSlotPerformance) ForEachPerformanceFactor(consumer func(accountID iotago.AccountID, vp *ValidatorPerformance) error) error {
	var innerErr error
	if err := p.store.Iterate(kvstore.EmptyPrefix, func(key kvstore.Key, value kvstore.Value) bool {
		accountID, _, err := iotago.IdentifierFromBytes(key)
		if err != nil {
			innerErr = err
			return false
		}
		vp, _, err := ValidatorPerformanceFromBytes(value, p.apiProvider.APIForSlot(p.slot))
		if err != nil {
			innerErr = err
			return false
		}

		return consumer(accountID, vp) == nil
	}); err != nil {
		return ierrors.Wrapf(err, "failed to stream performance factors for slot %s", p.slot)
	}

	if innerErr != nil {
		return ierrors.Wrapf(innerErr, "failed to deserialize performance factor for slot %s", p.slot)
	}

	return nil
}
