package blockfactory

import (
	"time"

	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type BlockParams struct {
	parentsCount            int
	references              model.ParentReferences
	slotCommitment          *iotago.Commitment
	payload                 iotago.Payload
	latestFinalizedSlot     *iotago.SlotIndex
	issuingTime             *time.Time
	protocolVersion         *iotago.Version
	issuer                  Account
	highestSupportedVersion *iotago.Version
	protocolParametersHash  *iotago.Identifier
}

func WithParentsCount(parentsCount int) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.parentsCount = parentsCount
	}
}

func WithStrongParents(blockIDs ...iotago.BlockID) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		if builder.references == nil {
			builder.references = make(model.ParentReferences)
		}

		builder.references[iotago.StrongParentType] = blockIDs
	}
}
func WithWeakParents(blockIDs ...iotago.BlockID) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		if builder.references == nil {
			builder.references = make(model.ParentReferences)
		}

		builder.references[iotago.WeakParentType] = blockIDs
	}
}

func WithShallowLikeParents(blockIDs ...iotago.BlockID) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		if builder.references == nil {
			builder.references = make(model.ParentReferences)
		}

		builder.references[iotago.ShallowLikeParentType] = blockIDs
	}
}

func WithSlotCommitment(commitment *iotago.Commitment) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.slotCommitment = commitment
	}
}

func WithLatestFinalizedSlot(commitmentIndex iotago.SlotIndex) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.latestFinalizedSlot = &commitmentIndex
	}
}

func WithPayload(payload iotago.Payload) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.payload = payload
	}
}

func WithIssuingTime(issuingTime time.Time) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.issuingTime = &issuingTime
	}
}

func WithProtocolVersion(version iotago.Version) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.protocolVersion = &version
	}
}
func WithIssuer(issuer Account) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.issuer = issuer
	}
}

func WithHighestSupportedVersion(highestSupportedVersion iotago.Version) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.highestSupportedVersion = &highestSupportedVersion
	}
}

func WithProtocolParametersHash(protocolParametersHash iotago.Identifier) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.protocolParametersHash = &protocolParametersHash
	}
}
