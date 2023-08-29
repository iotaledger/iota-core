package dashboard

import (
	"encoding/json"

	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	// MsgTypeNodeStatus is the type of the NodeStatus block.
	MsgTypeNodeStatus byte = iota
	// MsgTypeBPSMetric is the type of the block per second (BPS) metric block.
	MsgTypeBPSMetric
	// MsgTypeBlock is the type of the block.
	MsgTypeBlock
	// MsgTypeNeighborMetric is the type of the NeighborMetric block.
	MsgTypeNeighborMetric
	// MsgTypeComponentCounterMetric is the type of the component counter triggered per second.
	MsgTypeComponentCounterMetric
	// MsgTypeTipsMetric is the type of the TipsMetric block.
	MsgTypeTipsMetric
	// MsgTypeVertex defines a vertex block.
	MsgTypeVertex
	// MsgTypeTXAccepted defines a tx is accepted.
	MsgTypeTXAccepted
	// MsgTypeTipInfo defines a tip info block.
	MsgTypeTipInfo
	// MsgTypeManaValue defines a mana value block.
	MsgTypeManaValue
	// MsgTypeManaMapOverall defines a block containing overall mana map.
	MsgTypeManaMapOverall
	// MsgTypeManaMapOnline defines a block containing online mana map.
	MsgTypeManaMapOnline
	// MsgManaDashboardAddress is the socket address of the dashboard to stream mana from.
	MsgManaDashboardAddress
	// MsgTypeRateSetterMetric defines rate setter metrics.
	MsgTypeRateSetterMetric
	// MsgTypeConflictsConflictSet defines a websocket message that contains a conflictSet update for the "conflicts" tab.
	MsgTypeConflictsConflictSet
	// MsgTypeConflictsConflict defines a websocket message that contains a conflict update for the "conflicts" tab.
	MsgTypeConflictsConflict
	// MsgTypeSlotInfo defines a websocket message that contains a conflict update for the "conflicts" tab.
	MsgTypeSlotInfo
)

type wsblk struct {
	Type byte        `json:"type"`
	Data interface{} `json:"data"`
}

type blk struct {
	ID          string             `json:"id"`
	Value       int64              `json:"value"`
	PayloadType iotago.PayloadType `json:"payload_type"`
}

type nodestatus struct {
	ID         string          `json:"id"`
	Version    string          `json:"version"`
	Uptime     int64           `json:"uptime"`
	Mem        *memmetrics     `json:"mem"`
	TangleTime tangleTime      `json:"tangleTime"`
	Scheduler  schedulerMetric `json:"scheduler"`
}

type tangleTime struct {
	Synced       bool  `json:"synced"`
	Bootstrapped bool  `json:"bootstrapped"`
	ATT          int64 `json:"ATT"`
	RATT         int64 `json:"RATT"`
	CTT          int64 `json:"CTT"`
	RCTT         int64 `json:"RCTT"`

	AcceptedBlockSlot  int64 `json:"acceptedBlockSlot"`
	ConfirmedBlockSlot int64 `json:"confirmedBlockSlot"`
	CommittedSlot      int64 `json:"committedSlot"`
	ConfirmedSlot      int64 `json:"confirmedSlot"`
}

type memmetrics struct {
	HeapSys      uint64 `json:"heap_sys"`
	HeapAlloc    uint64 `json:"heap_alloc"`
	HeapIdle     uint64 `json:"heap_idle"`
	HeapReleased uint64 `json:"heap_released"`
	HeapObjects  uint64 `json:"heap_objects"`
	NumGC        uint32 `json:"num_gc"`
	LastPauseGC  uint64 `json:"last_pause_gc"`
}

type neighbormetric struct {
	ID             string `json:"id"`
	Addresses      string `json:"addresses"`
	PacketsRead    uint64 `json:"packets_read"`
	PacketsWritten uint64 `json:"packets_written"`
}

type tipsInfo struct {
	TotalTips int `json:"totaltips"`
}

type componentsmetric struct {
	Store      uint64 `json:"store"`
	Solidifier uint64 `json:"solidifier"`
	Scheduler  uint64 `json:"scheduler"`
	Booker     uint64 `json:"booker"`
}

type rateSetterMetric struct {
	Size     int     `json:"size"`
	Estimate string  `json:"estimate"`
	Rate     float64 `json:"rate"`
}

type schedulerMetric struct {
	Running           bool    `json:"running"`
	Rate              string  `json:"rate"`
	MaxBufferSize     int     `json:"maxBufferSize"`
	CurrentBufferSize int     `json:"currentBufferSize"`
	Deficit           float64 `json:"deficit"`
}

// ExplorerBlock defines the struct of the ExplorerBlock.
type ExplorerBlock struct {
	// ID is the block ID.
	ID string `json:"id"`
	// NetworkID is the network ID of the block that attaches to.
	NetworkID iotago.NetworkID `json:"networkID"`
	// ProtocolVersion is the protocol that proccess the block.
	ProtocolVersion iotago.Version `json:"protocolVersion"`
	// SolidificationTimestamp is the timestamp of the block.
	SolidificationTimestamp int64 `json:"solidificationTimestamp"`
	// The time when this block was issued
	IssuanceTimestamp int64 `json:"issuanceTimestamp"`
	// The issuer's sequence number of this block.
	SequenceNumber uint64 `json:"sequenceNumber"`
	// The public key of the issuer who issued this block.
	IssuerID string `json:"issuerID"`
	// The signature of the block.
	Signature string `json:"signature"`
	// StrongParents are the strong parents of the block.
	StrongParents []string `json:"strongParents"`
	// WeakParents are the strong parents of the block.
	WeakParents []string `json:"weakParents"`
	// ShallowLikedParents are the strong parents of the block.
	ShallowLikedParents []string `json:"shallowLikedParents"`
	// StrongChildren are the strong children of the block.
	StrongChildren []string `json:"strongChildren"`
	// WeakChildren are the weak children of the block.
	WeakChildren []string `json:"weakChildren"`
	// LikedInsteadChildren are the shallow like children of the block.
	LikedInsteadChildren []string `json:"shallowLikeChildren"`
	// Solid defines the solid status of the block.
	Solid                  bool     `json:"solid"`
	ConflictIDs            []string `json:"conflictIDs"`
	AddedConflictIDs       []string `json:"addedConflictIDs"`
	SubtractedConflictIDs  []string `json:"subtractedConflictIDs"`
	Scheduled              bool     `json:"scheduled"`
	Booked                 bool     `json:"booked"`
	Orphaned               bool     `json:"orphaned"`
	ObjectivelyInvalid     bool     `json:"objectivelyInvalid"`
	SubjectivelyInvalid    bool     `json:"subjectivelyInvalid"`
	Acceptance             bool     `json:"acceptance"`
	AcceptanceTime         int64    `json:"acceptanceTime"`
	Confirmation           bool     `json:"confirmation"`
	ConfirmationTime       int64    `json:"confirmationTime"`
	ConfirmationBySlot     bool     `json:"confirmationBySlot"`
	ConfirmationBySlotTime int64    `json:"confirmationBySlotTime"`
	// PayloadType defines the type of the payload.
	PayloadType iotago.PayloadType `json:"payloadType"`
	// Payload is the content of the payload.
	Payload       json.RawMessage `json:"payload"`
	TransactionID string          `json:"txId,omitempty"`

	// Structure details
	Rank          uint64 `json:"rank"`
	PastMarkerGap uint64 `json:"pastMarkerGap"`
	IsPastMarker  bool   `json:"isPastMarker"`
	PastMarkers   string `json:"pastMarkers"`

	// Slot commitment
	CommitmentID        string             `json:"commitmentID"`
	Commitment          CommitmentResponse `json:"commitment"`
	LatestConfirmedSlot uint64             `json:"latestConfirmedSlot"`
}

// ExplorerAddress defines the struct of the ExplorerAddress.
type ExplorerAddress struct {
	Address         string           `json:"address"`
	ExplorerOutputs []ExplorerOutput `json:"explorerOutputs"`
}

// ExplorerOutput defines the struct of the ExplorerOutput.
type ExplorerOutput struct {
	ID     iotago.OutputIDHex `json:"id"`
	Output iotago.Output      `json:"output"`
	// Metadata          *jsonmodels.OutputMetadata `json:"metadata"`
	TxTimestamp int `json:"txTimestamp"`
	// ConfirmationState coreapi.txState `json:"confirmationState"`
}

type CommitmentResponse struct {
	Index            uint64 `json:"index"`
	PrevID           string `json:"prevID"`
	RootsID          string `json:"rootsID"`
	CumulativeWeight uint64 `json:"cumulativeWeight"`
}
