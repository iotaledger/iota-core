package debugapi

import iotago "github.com/iotaledger/iota.go/v4"

type (
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
