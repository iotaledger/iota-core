package ledger

import (
	"bytes"
	"time"

	iotago "github.com/iotaledger/iota.go/v4"
)

type BlockVoteRank struct {
	blockID iotago.BlockID
	time    time.Time
}

func NewBlockVoteRank(id iotago.BlockID, time time.Time) BlockVoteRank {
	return BlockVoteRank{
		blockID: id,
		time:    time,
	}
}

func (v BlockVoteRank) Compare(other BlockVoteRank) int {
	if v.time.Before(other.time) {
		return -1
	} else if v.time.After(other.time) {
		return 1
	}

	return bytes.Compare(v.blockID[:], other.blockID[:])
}
