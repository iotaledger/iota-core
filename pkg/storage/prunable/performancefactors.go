package prunable

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	iotago "github.com/iotaledger/iota.go/v4"
)

// VlidatorSlotPerformance represent the performance factors of a validator for a given slot.
type VlidatorSlotPerformance struct {
	slot  iotago.SlotIndex
	store kvstore.KVStore
}

type ValidatorPerformance struct {
	// works if ValidatorBlocksPerSlot is less than 32 because we use it as bit vector
	SlotActivityVector uint32
	// can be uint8 because max count per slot is maximally ValidatorBlocksPerSlot + 1
	BlockIssuedCount        uint8
	HighestSupportedVersion iotago.Version
}

func (p *ValidatorPerformance) Bytes() []byte {
	m := marshalutil.MarshalUtil{}
	m.WriteUint32(p.SlotActivityVector)
	m.WriteUint8(p.BlockIssuedCount)
	m.WriteByte(byte(p.HighestSupportedVersion))

	return m.Bytes()
}

func (p *ValidatorPerformance) FromBytes(bytes []byte) (int, error) {
	m := marshalutil.New(bytes)
	av, err := m.ReadUint32()
	if err != nil {
		return 0, err
	}
	p.SlotActivityVector = av
	count, err := m.ReadUint8()
	if err != nil {
		return 0, err
	}
	p.BlockIssuedCount = count

	v, err := m.ReadByte()
	if err != nil {
		return 0, err
	}
	p.HighestSupportedVersion = iotago.Version(v)

	return m.ReadOffset(), nil
}

// NewPerformanceFactors is a constructor for the VlidatorSlotPerformance.
func NewPerformanceFactors(slot iotago.SlotIndex, store kvstore.KVStore) *VlidatorSlotPerformance {
	return &VlidatorSlotPerformance{
		slot:  slot,
		store: store,
	}
}

func (p *VlidatorSlotPerformance) Load(accountID iotago.AccountID) (pf *ValidatorPerformance, err error) {
	performanceFactorsBytes, err := p.store.Get(accountID[:])
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return nil, nil
		}

		return nil, ierrors.Wrapf(err, "failed to get performance factor for account %s", accountID)
	}
	pf = &ValidatorPerformance{}
	_, err = pf.FromBytes(performanceFactorsBytes)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to deserialize performance factor for account %s", accountID)
	}

	return pf, nil
}

func (p *VlidatorSlotPerformance) Store(accountID iotago.AccountID, pf *ValidatorPerformance) error {
	return p.store.Set(lo.PanicOnErr(accountID.Bytes()), pf.Bytes())
}

// ForEachPerformanceFactor iterates over all saved validators.
// Note that this stream function won't call the consumer function for inactive validators,
// so better to use the load over every member of the committee if needed.
func (p *VlidatorSlotPerformance) ForEachPerformanceFactor(consumer func(accountID iotago.AccountID, pf *ValidatorPerformance) error) error {
	var innerErr error
	if err := p.store.Iterate(kvstore.EmptyPrefix, func(key kvstore.Key, value kvstore.Value) bool {
		accountID, _, err := iotago.IdentifierFromBytes(key)
		if err != nil {
			innerErr = err
			return false
		}

		pf := &ValidatorPerformance{}
		if _, err = pf.FromBytes(value); err != nil {
			innerErr = err
			return false
		}

		return consumer(accountID, pf) == nil
	}); err != nil {
		return ierrors.Wrapf(err, "failed to stream performance factors for slot %s", p.slot)
	}

	if innerErr != nil {
		return ierrors.Wrapf(innerErr, "failed to deserialize performance factor for slot %s", p.slot)
	}

	return nil
}
