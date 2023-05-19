package blockissuer

import (
	"time"

	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type BlockParams struct {
	parentsCount             int
	references               model.ParentReferences
	slotCommitment           *iotago.Commitment
	payload                  iotago.Payload
	latestFinalizedSlot      iotago.SlotIndex
	latestFinalizedSlotSet   bool
	issuingTime              time.Time
	issuingTimeSet           bool
	protocolVersion          byte
	protocolVersionSet       bool
	issuer                   Account
	issuerSet                bool
	proofOfWorkDifficulty    float64
	proofOfWorkDifficultySet bool
}

func WithParentsCount(parentsCount int) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.parentsCount = parentsCount
	}
}

func WithReferences(references model.ParentReferences) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.references = references
	}
}

func WithSlotCommitment(commitment *iotago.Commitment) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.slotCommitment = commitment
	}
}

func WithLatestFinalizedSlot(commitment iotago.SlotIndex) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.latestFinalizedSlot = commitment
		builder.latestFinalizedSlotSet = true
	}
}

func WithPayload(payload iotago.Payload) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.payload = payload
	}
}

func WithIssuingTime(issuingTime time.Time) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.issuingTime = issuingTime
		builder.issuingTimeSet = true
	}
}

func WithProtocolVersion(version byte) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.protocolVersion = version
		builder.protocolVersionSet = true
	}
}
func WithIssuer(issuer Account) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.issuer = issuer
	}
}
func WithProofOfWorkDifficulty(difficulty float64) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.proofOfWorkDifficulty = difficulty
		builder.proofOfWorkDifficultySet = true
	}
}
