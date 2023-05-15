package booker

import (
	"bytes"
	"time"

	iotago "github.com/iotaledger/iota.go/v4"
)

type BlockVotePower struct {
	blockID iotago.BlockID
	time    time.Time
}

func NewBlockVotePower(id iotago.BlockID, time time.Time) BlockVotePower {
	return BlockVotePower{
		blockID: id,
		time:    time,
	}
}

func (v BlockVotePower) Compare(other BlockVotePower) int {
	if v.time.Before(other.time) {
		return -1
	} else if v.time.After(other.time) {
		return 1
	} else {
		return bytes.Compare(v.blockID[:], other.blockID[:])
	}
}
