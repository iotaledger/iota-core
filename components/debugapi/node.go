package debugapi

import (
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/account"
	iotago "github.com/iotaledger/iota.go/v4"
)

//nolint:unparam // we have no error case right now
func validatorsSummary() (*ValidatorsSummaryResponse, error) {
	seatManager := deps.Protocol.MainEngineInstance().SybilProtection.SeatManager()
	latestSlotIndex := deps.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Index()
	latestCommittee := seatManager.Committee(latestSlotIndex)
	validatorSeats := []*Validator{}
	latestCommittee.Accounts().ForEach(func(id iotago.AccountID, pool *account.Pool) bool {
		validatorSeats = append(validatorSeats, &Validator{
			AccountID:      id,
			SeatIndex:      uint8(lo.Return1(latestCommittee.GetSeat(id))),
			PoolStake:      pool.PoolStake,
			ValidatorStake: pool.ValidatorStake,
			FixedCost:      pool.FixedCost,
		})

		return true
	})

	return &ValidatorsSummaryResponse{
		ValidatorSeats: validatorSeats,
		ActiveSeats: lo.Map(seatManager.OnlineCommittee().ToSlice(), func(seatIndex account.SeatIndex) uint32 {
			return uint32(seatIndex)
		}),
	}, nil
}
