package coreapi

//nolint:unused // transactions are currently unused
type txState int

//nolint:unused // transactions are currently unused
const (
	txStatePending txState = iota
	txStateConfirmed
	txStateFinalized
	txStateRejected
	txStateConflicting
)

//nolint:unused // transactions are currently unused
func (t txState) String() string {
	switch t {
	case txStatePending:
		return "pending"
	case txStateConfirmed:
		return "confirmed"
	case txStateFinalized:
		return "finalized"
	case txStateRejected:
		return "rejected"
	case txStateConflicting:
		return "conflicting"
	default:
		return "unknown"
	}
}

type blockState int

const (
	blockStatePending blockState = iota
	blockStateConfirmed
	blockStateFinalized
)

func (b blockState) String() string {
	switch b {
	case blockStatePending:
		return "pending"
	case blockStateConfirmed:
		return "confirmed"
	case blockStateFinalized:
		return "finalized"
	default:
		return "unknown"
	}
}

// submitBlockResponse defines the response of a POST blocks REST API call.
type submitBlockResponse struct {
	// The hex encoded block ID of the block.
	BlockID string `json:"blockId"`
}
