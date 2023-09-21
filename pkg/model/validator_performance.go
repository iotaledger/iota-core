package model

import (
	iotago "github.com/iotaledger/iota.go/v4"
)

type ValidatorPerformance struct {
	// works if ValidatorBlocksPerSlot is less than 32 because we use it as bit vector
	SlotActivityVector uint32 `serix:"0"`
	// can be uint8 because max count per slot is maximally ValidatorBlocksPerSlot + 1
	BlockIssuedCount               uint8          `serix:"1"`
	HighestSupportedVersionAndHash VersionAndHash `serix:"2"`
}

func NewValidatorPerformance() *ValidatorPerformance {
	return &ValidatorPerformance{
		SlotActivityVector:             0,
		BlockIssuedCount:               0,
		HighestSupportedVersionAndHash: VersionAndHash{},
	}
}

func ValidatorPerformanceFromBytes(decodeAPI iotago.API) func([]byte) (*ValidatorPerformance, int, error) {
	return func(bytes []byte) (*ValidatorPerformance, int, error) {
		validatorPerformance := new(ValidatorPerformance)
		consumedBytes, err := decodeAPI.Decode(bytes, validatorPerformance)
		if err != nil {
			return nil, 0, err
		}

		return validatorPerformance, consumedBytes, nil
	}
}

func (p *ValidatorPerformance) Bytes(api iotago.API) ([]byte, error) {
	return api.Encode(p)
}
