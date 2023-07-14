package debugapi

import (
	"github.com/iotaledger/iota-core/pkg/core/account"
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
		Future       bool `json:"future"`
		PreAccepted  bool `json:"preAccepted"`
		Accepted     bool `json:"accepted"`
		PreConfirmed bool `json:"preConfirmed"`
		Confirmed    bool `json:"confirmed"`

		Witnesses []account.SeatIndex `json:"witnesses"`
		// conflictIDs are the all conflictIDs of the block inherited from the parents + payloadConflictIDs.
		ConflictIDs []iotago.TransactionID `json:"conflictIDs"`
		// payloadConflictIDs are the conflictIDs of the block's payload (in case it is a transaction, otherwise empty).
		PayloadConflictIDs []iotago.TransactionID `json:"payloadConflictIDs"`
		String             string                 `json:"string"`
	}

	ValidatorsSummaryResponse struct {
		ValidatorSeats map[int]string `json:"validatorSeats"`
		ActiveSeats    []int          `json:"activeSeats"`
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
