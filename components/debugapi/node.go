package debugapi

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/account"
	iotago "github.com/iotaledger/iota.go/v4"
)

func validatorsSummary() (*ValidatorsSummaryResponse, error) {
	seatManager := deps.Protocol.Engines.Main.Get().SybilProtection.SeatManager()
	latestSlotIndex := deps.Protocol.Engines.Main.Get().SyncManager.LatestCommitment().Slot()
	latestCommittee, exists := seatManager.CommitteeInSlot(latestSlotIndex)
	if !exists {
		return nil, ierrors.Errorf("committee for slot %d was not selected", latestSlotIndex)
	}

	var validatorSeats []*Validator
	accounts, err := latestCommittee.Accounts()
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get accounts from committee for slot %d", latestSlotIndex)
	}

	accounts.ForEach(func(id iotago.AccountID, pool *account.Pool) bool {
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
