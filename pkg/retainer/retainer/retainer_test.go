package retainer

import (
	"testing"

	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/tpkg"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func Test_ValidatorCache(t *testing.T) {
	validators := make([]*api.ValidatorResponse, 0)
	for i := 0; i < 10; i++ {
		validators = append(validators, &api.ValidatorResponse{
			AddressBech32:   tpkg.RandAccountID().ToAddress().Bech32(iotago.PrefixTestnet),
			FixedCost:       1,
			StakingEndEpoch: iotago.EpochIndex(i),
		})
	}

	r := New(workerpool.NewGroup("retainer"), func(si iotago.SlotIndex) (*slotstore.Retainer, error) {
		return nil, nil
	}, func() iotago.SlotIndex {
		return iotago.SlotIndex(5)
	}, func() iotago.SlotIndex {
		return iotago.SlotIndex(2)
	}, func(ei iotago.EpochIndex) ([]*api.ValidatorResponse, error) {
		if ei == 0 {
			return validators, nil
		}
		return nil, echo.ErrNotFound
	}, func(ei iotago.EpochIndex) iotago.API {
		return tpkg.ZeroCostTestAPI
	}, func(err error) {})

	// epoch 0, with 10 validators
	resp, err := r.RegisteredValidators(iotago.EpochIndex(0))
	require.NoError(t, err)
	require.ElementsMatch(t, validators, resp)

	// epoch 0, 1 validator added, should return 11 validators
	validators = append(validators, &api.ValidatorResponse{
		AddressBech32:   tpkg.RandAccountID().ToAddress().Bech32(iotago.PrefixTestnet),
		FixedCost:       1,
		StakingEndEpoch: iotago.EpochIndex(10),
	})
	resp, err = r.RegisteredValidators(iotago.EpochIndex(0))
	require.NoError(t, err)
	require.ElementsMatch(t, validators, resp)
	require.False(t, r.registeredValidatorsCache.Has(iotago.EpochIndex(0).MustBytes()))

	// epoch 2, we should have validators in epoch 0 added to cache
	r.latestCommittedSlotFunc = func() iotago.SlotIndex {
		return tpkg.ZeroCostTestAPI.TimeProvider().EpochStart(2)
	}
	resp, err = r.RegisteredValidators(iotago.EpochIndex(0))
	require.NoError(t, err)
	require.ElementsMatch(t, validators, resp)
	require.True(t, r.registeredValidatorsCache.Has(iotago.EpochIndex(0).MustBytes()))

	// epoch 3, we should have cache hit
	r.latestCommittedSlotFunc = func() iotago.SlotIndex {
		return tpkg.ZeroCostTestAPI.TimeProvider().EpochStart(3)
	}
	resp, err = r.RegisteredValidators(iotago.EpochIndex(0))
	require.NoError(t, err)
	require.ElementsMatch(t, validators, resp)
	require.True(t, r.registeredValidatorsCache.Has(iotago.EpochIndex(0).MustBytes()))

	// can not retrieve validators for epoch 1, should return not found
	resp, err = r.RegisteredValidators(iotago.EpochIndex(1))
	require.ErrorAs(t, echo.ErrNotFound, &err)

}
