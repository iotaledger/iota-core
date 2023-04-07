package coreapi

import (
	"github.com/iotaledger/iota-core/pkg/protocol"
	iotago "github.com/iotaledger/iota.go/v4"
)

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
	// The semver version of the node software.
	Version string `json:"version"`
	// The current status of this node.
	Status nodeStatus `json:"status"`
	// The protocol versions this node supports.
	SupportedProtocolVersions protocol.Versions `json:"supportedProtocolVersions"`
	// The protocol parameters used by this node.
	ProtocolParameters *iotago.ProtocolParameters `json:"protocol"`
	// The base token of the network.
	BaseToken *protocfg.BaseToken `json:"baseToken"`
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
