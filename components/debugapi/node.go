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
	validatorSeats := make(map[int]string)
	latestCommittee.Accounts().ForEach(func(id iotago.AccountID, pool *account.Pool) bool {
		// TODO: check if seat exists, and include Pool info in the response
		validatorSeats[int(lo.Return1(latestCommittee.GetSeat(id)))] = id.String()

		return true
	})

	return &ValidatorsSummaryResponse{
		ValidatorSeats: validatorSeats,
		ActiveSeats: lo.Map(seatManager.OnlineCommittee().ToSlice(), func(seatIndex account.SeatIndex) int {
			return int(seatIndex)
		}),
	}, nil
}
