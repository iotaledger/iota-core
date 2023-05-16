package coreapi

import (
	"encoding/json"
	"time"

	"github.com/iotaledger/iota-core/pkg/protocol"
	iotago "github.com/iotaledger/iota.go/v4"
)

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

// commitmentInfoResponse defines the commitment info response.
type commitmentInfoResponse struct {
	// The index of the requested commitment.
	Index iotago.SlotIndex `json:"index"`
	// The commitment ID of previous commitment.
	PrevID string `json:"prevId"`
	// The roots ID of merkle trees within the requested commitment.
	RootsID string `json:"rootsId"`
	// The cumulative weight of the requested slot.
	CumulativeWeight uint64 `json:"cumulativeWeight"`
	// TODO: decide what else to add here.
}

type slotUTXOResponse struct {
	// The index of the requested commitment.
	Index iotago.SlotIndex `json:"index"`
	// The outputs that are created in this slot.
	CreatedOutputs []string `json:"createdOutputs"`
	// The outputs that are consumed in this slot.
	ConsumedOutputs []string `json:"consumedOutputs"`
}

// infoResponse defines the response of a GET info REST API call.
type infoResponse struct {
	// The name of the node software.
	Name string `json:"name"`
	// The server version of the node software.
	Version string `json:"version"`
	// The ID of the node
	IssuerID string `json:"issuerId"`
	// The current status of this node.
	Status nodeStatus `json:"status"`
	// The metrics of this node.
	Metrics nodeMetrics `json:"metrics"`
	// The protocol versions this node supports.
	SupportedProtocolVersions protocol.Versions `json:"supportedProtocolVersions"`
	// The protocol parameters used by this node.
	ProtocolParameters json.RawMessage `json:"protocol"`
	// todo The base token of the network.
	//BaseToken *protocfg.BaseToken `json:"baseToken"`
	// The features this node exposes.
	Features []string `json:"features"`
}

//nolint:tagliatelle
type nodeStatus struct {
	// Whether the node is healthy.
	IsHealthy bool `json:"isHealthy"`
	// The blockID of last accepted block.
	LastAcceptedBlockID string `json:"lastAcceptedBlockId"`
	// The blockID of the last confirmed block
	LastConfirmedBlockID string `json:"lastConfirmedBlockId"`
	// The latest finalized slot.
	FinalizedSlot iotago.SlotIndex `json:"latestFinalizedSlot"`
	// The Accepted Tangle Time
	ATT time.Time `json:"ATT"`
	// The Relative Accepted Tangle Time
	RATT time.Time `json:"RATT"`
	// The Confirmed Tangle Time
	CTT time.Time `json:"CTT"`
	// The Relative Confirmed Tangle Time
	RCTT time.Time `json:"RCTT"`
	// The latest known committed slot info.
	LatestCommittedSlot iotago.SlotIndex `json:"latestCommittedSlot"`
	// The slot index at which the last pruning commenced.
	PruningSlot iotago.SlotIndex `json:"pruningSlot"`
}

type nodeMetrics struct {
	// The current rate of new blocks per second, it's updated when a commitment is committed.
	BlocksPerSecond float64 `json:"blocksPerSecond"`
	// The current rate of confirmed blocks per second, it's updated when a commitment is committed.
	ConfirmedBlocksPerSecond float64 `json:"confirmedBlocksPerSecond"`
	// The ratio of confirmed blocks in relation to new blocks up until the latest commitment is committed.
	ConfirmedRate float64 `json:"confirmedRate"`
}

// blockMetadataResponse defines the response of a GET block metadata REST API call.
type blockMetadataResponse struct {
	// BlockID The hex encoded block ID of the block.
	BlockID string `json:"blockId"`
	// StrongParents are the strong parents of the block.
	StrongParents []string `json:"strongParents"`
	// WeakParents are the weak parents of the block.
	WeakParents []string `json:"weakParents"`
	// ShallowLikeParents are the shallow like parents of the block.
	ShallowLikeParents []string `json:"shallowLikeParents"`
	// BlockState might be pending, confirmed, finalized.
	BlockState string `json:"blockState"`
	// TxState might be pending, conflicting, confirmed, finalized, rejected.
	TxState string `json:"txState,omitempty"`
	// BlockStateReason if applicable indicates the error that occurred during the block processing.
	BlockStateReason string `json:"blockStateReason,omitempty"`
	// TxStateReason if applicable indicates the error that occurred during the transaction processing.
	TxStateReason string `json:"txStateReason,omitempty"`
	// ReissuePayload whether the block should be reissued.
	ReissuePayload *bool `json:"reissuePayload,omitempty"`
}

type blockIssuanceResponse struct {
	// StrongParents are the strong parents of the block.
	StrongParents []string `json:"strongParents"`
	// WeakParents are the weak parents of the block.
	WeakParents []string `json:"weakParents"`
	// ShallowLikeParents are the shallow like parents of the block.
	ShallowLikeParents  []string         `json:"shallowLikeParents"`
	LatestFinalizedSlot iotago.SlotIndex `json:"latestFinalizedSlot"`
	Commitment          json.RawMessage  `json:"commitment"`
}

// blockCreatedResponse defines the response of a POST blocks REST API call.
type blockCreatedResponse struct {
	// The hex encoded block ID of the block.
	BlockID string `json:"blockId"`
}

type outputMetadataResponse struct {
	BlockID              string `json:"blockId"`
	TransactionID        string `json:"transactionId"`
	OutputIndex          uint16 `json:"outputIndex"`
	IsSpent              bool   `json:"isSpent"`
	CommitmentIDSpent    string `json:"commitmentIdSpent"`
	TransactionIDSpent   string `json:"transactionIdSpent"`
	IncludedCommitmentID string `json:"includedCommitmentId"`
	LatestCommitmentID   string `json:"latestCommitmentId"`
}
