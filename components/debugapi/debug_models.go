package debugapi

import (
	"fmt"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

type (
	BlockMetadataResponse struct {
		// BlockID The hex encoded block ID of the block.
		BlockID string `json:"blockId"`
		// StrongParents are the strong parents of the block.
		StrongParents []string `json:"strongParents"`
		// WeakParents are the weak parents of the block.
		WeakParents []string `json:"weakParents"`
		// ShallowLikeParents are the shallow like parents of the block.
		ShallowLikeParents []string `json:"shallowLikeParents"`

		Solid        bool `json:"solid"`
		Invalid      bool `json:"invalid"`
		Booked       bool `json:"booked"`
		PreAccepted  bool `json:"preAccepted"`
		Accepted     bool `json:"accepted"`
		PreConfirmed bool `json:"preConfirmed"`
		Confirmed    bool `json:"confirmed"`

		Witnesses []string `json:"witnesses"`
		// spenderIDs are the all spenderIDs of the block inherited from the parents + payloadSpenderIDs.
		SpenderIDs []iotago.TransactionID `json:"spenderIDs"`
		// payloadSpenderIDs are the spenderIDs of the block's payload (in case it is a transaction, otherwise empty).
		PayloadSpenderIDs []iotago.TransactionID `json:"payloadSpenderIDs"`
		String            string                 `json:"string"`
	}

	Validator struct {
		AccountID      iotago.AccountID `serix:""`
		SeatIndex      uint8            `serix:""`
		PoolStake      iotago.BaseToken `serix:""`
		ValidatorStake iotago.BaseToken `serix:""`
		FixedCost      iotago.Mana      `serix:""`
	}

	ValidatorsSummaryResponse struct {
		ValidatorSeats []*Validator `serix:"lenPrefix=uint8"`
		ActiveSeats    []uint32     `serix:"lenPrefix=uint8"`
	}

	BlockChangesResponse struct {
		// The index of the requested commitment.
		Index iotago.SlotIndex `json:"index"`
		// The blocks that got included in this slot.
		IncludedBlocks []string `json:"includedBlocks"`
		// The tangle root of the slot.
		TangleRoot string `json:"tangleRoot"`
	}

	TransactionsChangesResponse struct {
		// The index of the requested commitment.
		Index iotago.SlotIndex `json:"index"`
		// The transactions that got included in this slot.
		IncludedTransactions []string `json:"includedTransactions"`
		// The mutations root of the slot.
		MutationsRoot string `json:"mutationsRoot"`
	}
)

func BlockMetadataResponseFromBlock(block *blocks.Block) *BlockMetadataResponse {
	return &BlockMetadataResponse{
		BlockID:            block.ID().String(),
		StrongParents:      lo.Map(block.StrongParents(), func(blockID iotago.BlockID) string { return blockID.String() }),
		WeakParents:        lo.Map(block.ProtocolBlock().Body.WeakParentIDs(), func(blockID iotago.BlockID) string { return blockID.String() }),
		ShallowLikeParents: lo.Map(block.ProtocolBlock().Body.ShallowLikeParentIDs(), func(blockID iotago.BlockID) string { return blockID.String() }),
		Solid:              block.IsSolid(),
		Invalid:            block.IsInvalid(),
		Booked:             block.IsBooked(),
		PreAccepted:        block.IsPreAccepted(),
		Accepted:           block.IsAccepted(),
		PreConfirmed:       block.IsPreConfirmed(),
		Confirmed:          block.IsConfirmed(),
		Witnesses:          lo.Map(block.Witnesses().ToSlice(), func(seatIndex account.SeatIndex) string { return fmt.Sprintf("%d", seatIndex) }),
		SpenderIDs:         block.SpenderIDs().ToSlice(),
		PayloadSpenderIDs:  block.PayloadSpenderIDs().ToSlice(),
		String:             block.String(),
	}
}
