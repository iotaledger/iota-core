package model

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	iotago "github.com/iotaledger/iota.go/v4"
)

// PoolsStats contains stats about the pools from all the validators for an epoch.
type PoolsStats struct {
	TotalStake          iotago.BaseToken
	TotalValidatorStake iotago.BaseToken
	ProfitMargin        uint64
}

func PoolsStatsFromBytes(bytes []byte) (*PoolsStats, int, error) {
	p := new(PoolsStats)

	var err error
	byteReader := stream.NewByteReader(bytes)

	if p.TotalStake, err = stream.Read[iotago.BaseToken](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read TotalStake")
	}
	if p.TotalValidatorStake, err = stream.Read[iotago.BaseToken](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read TotalValidatorStake")
	}
	if p.ProfitMargin, err = stream.Read[uint64](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read ProfitMargin")
	}

	return p, byteReader.BytesRead(), nil
}

func (p *PoolsStats) Bytes() ([]byte, error) {
	byteBuffer := stream.NewByteBuffer()

	if err := stream.Write(byteBuffer, p.TotalStake); err != nil {
		return nil, ierrors.Wrap(err, "failed to write TotalStake")
	}
	if err := stream.Write(byteBuffer, p.TotalValidatorStake); err != nil {
		return nil, ierrors.Wrap(err, "failed to write TotalValidatorStake")
	}
	if err := stream.Write(byteBuffer, p.ProfitMargin); err != nil {
		return nil, ierrors.Wrap(err, "failed to write ProfitMargin")
	}

	return byteBuffer.Bytes()
}

type PoolRewards struct {
	// Total stake of the validator including delegations
	PoolStake iotago.BaseToken
	// Rewards normalized by performance factor
	PoolRewards iotago.Mana
	// What the validator charges for its staking duties
	FixedCost iotago.Mana
}

func PoolRewardsFromBytes(bytes []byte) (*PoolRewards, int, error) {
	p := new(PoolRewards)

	var err error
	byteReader := stream.NewByteReader(bytes)

	if p.PoolStake, err = stream.Read[iotago.BaseToken](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read PoolStake")
	}
	if p.PoolRewards, err = stream.Read[iotago.Mana](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read PoolRewards")
	}
	if p.FixedCost, err = stream.Read[iotago.Mana](byteReader); err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to read FixedCost")
	}

	return p, byteReader.BytesRead(), nil
}

func (p *PoolRewards) Bytes() ([]byte, error) {
	byteBuffer := stream.NewByteBuffer()

	if err := stream.Write(byteBuffer, p.PoolStake); err != nil {
		return nil, ierrors.Wrap(err, "failed to write PoolStake")
	}
	if err := stream.Write(byteBuffer, p.PoolRewards); err != nil {
		return nil, ierrors.Wrap(err, "failed to write PoolRewards")
	}
	if err := stream.Write(byteBuffer, p.FixedCost); err != nil {
		return nil, ierrors.Wrap(err, "failed to write FixedCost")
	}

	return byteBuffer.Bytes()
}
