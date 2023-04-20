package coreapi

import (
	"github.com/iotaledger/iota-core/pkg/protocol"
	iotago "github.com/iotaledger/iota.go/v4"
)

type txState int

const (
	txStatePending txState = iota
	txStateConfirmed
	txStateFinalized
	txStateRejected
	txStateConflicting
)

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

// slotInfoResponse defines the slot info response.
type slotInfoResponse struct {
	// The index of the latest commitment.
	Index iotago.SlotIndex `json:"index"`
	// The unix time of the milestone payload.
	// The timestamp can be omitted if the milestone is not available
	// (no milestone received yet after starting from snapshot).
	Timestamp uint32 `json:"timestamp,omitempty"`
	// The ID of the commitment.
	// The ID can be omitted if the commitment is not available.
	// (no commitment created yet after starting from snapshot).
	CommitmentID string `json:"commitmentId,omitempty"`
}

// infoResponse defines the response of a GET info REST API call.
type infoResponse struct {
	// The name of the node software.
	Name string `json:"name"`
	// The server version of the node software.
	Version string `json:"version"`
	// The current status of this node.
	Status nodeStatus `json:"status"`
	// The protocol versions this node supports.
	SupportedProtocolVersions protocol.Versions `json:"supportedProtocolVersions"`
	// The protocol parameters used by this node.
	ProtocolParameters *iotago.ProtocolParameters `json:"protocol"`
	// todo The base token of the network.
	//BaseToken *protocfg.BaseToken `json:"baseToken"`
	// The metrics of this node.
	Metrics nodeMetrics `json:"metrics"`
	// The features this node exposes.
	Features []string `json:"features"`
}

type nodeStatus struct {
	// Whether the node is healthy.
	IsHealthy bool `json:"isHealthy"`
	// The latest known committed slot info.
	LatestCommittedSlot slotInfoResponse `json:"latestCommittedSlot"`
	// The latest confirmed slot.
	ConfirmedSlot slotInfoResponse `json:"latestConfirmedSlot"`
	// The milestone index at which the last pruning commenced.
	PruningIndex iotago.SlotIndex `json:"pruningIndex"`
}

type nodeMetrics struct {
	// The current rate of new blocks per second.
	BlocksPerSecond float64 `json:"blocksPerSecond"`
	// The current rate of referenced blocks per second.
	ReferencedBlocksPerSecond float64 `json:"referencedBlocksPerSecond"`
	// The ratio of referenced blocks in relation to new blocks of the last confirmed milestone.
	ReferencedRate float64 `json:"referencedRate"`
}

// blockMetadataResponse defines the response of a GET block metadata REST API call.
type blockMetadataResponse struct {
	// The hex encoded block ID of the block.
	BlockID string `json:"blockId"`
	// BlockState might be pending, confirmed, finalized.
	BlockState string `json:"blockState"`
	// TxState might be pending, conflicting, confirmed, finalized, rejected.
	TxState string `json:"txState,omitempty"`
	// BlockError if applicable indicates the error that occurred during the block processing.
	BlockError string `json:"blockError,omitempty"`
	// TxError if applicable indicates the error that occurred during the transaction processing.
	TxError string `json:"txError,omitempty"`
	// Whether the block should be reissued.
	ShouldReissue *bool `json:"shouldReattach,omitempty"`
}

type blockIssuanceResponse struct {
	StrongParents       iotago.StrongParentsIDs     `json:"strongParents"`
	WeakParents         iotago.WeakParentsIDs       `json:"weakParents"`
	ShallowParents      iotago.ShallowLikeParentIDs `json:"shallowParents"`
	LatestConfirmedSlot iotago.SlotIndex            `json:"latestConfirmedSlot"`
	Commitment          iotago.Commitment           `json:"commitment"`
}

// blockCreatedResponse defines the response of a POST blocks REST API call.
type blockCreatedResponse struct {
	// The hex encoded block ID of the block.
	BlockID string `json:"blockId"`
}
