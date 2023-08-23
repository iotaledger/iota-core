package prunable

import (
	"github.com/iotaledger/hive.go/kvstore"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

// TODO will be refactored anyway after merging pruning PR
// RegisteredValidatorSlotActivity represent the performance factors of a validator for a given epoch.
type RegisteredValidatorSlotActivity struct {
	epoch iotago.SlotIndex
	store kvstore.KVStore

	apiProvider api.Provider
}

type RegisteredValidatorActivity struct {
	// works if ValidatorBlocksPerSlot is less than 32 because we use it as bit vector
	Active                         bool
	HighestSupportedVersionAndHash iotago.VersionAndHash
}

// NewRegisteredValidatorActivity is a constructor for the ValidatorSlotPerformance.
func NewRegisteredValidatorActivity(epochStart iotago.SlotIndex, store kvstore.KVStore, apiProvider api.Provider) *RegisteredValidatorSlotActivity {
	return &RegisteredValidatorSlotActivity{
		epoch:       epochStart,
		store:       store,
		apiProvider: apiProvider,
	}
}
