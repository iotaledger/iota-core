package blockfactory

import (
	"time"

	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type BlockParams struct {
	ParentsCount            int
	References              model.ParentReferences
	SlotCommitment          *iotago.Commitment
	Payload                 iotago.Payload
	LatestFinalizedSlot     *iotago.SlotIndex
	IssuingTime             *time.Time
	ProtocolVersion         *iotago.Version
	Issuer                  Account
	HighestSupportedVersion *iotago.Version
	ProtocolParametersHash  *iotago.Identifier
}

func WithParentsCount(parentsCount int) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.ParentsCount = parentsCount
	}
}

func WithStrongParents(blockIDs ...iotago.BlockID) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		if builder.References == nil {
			builder.References = make(model.ParentReferences)
		}

		builder.References[iotago.StrongParentType] = blockIDs
	}
}
func WithWeakParents(blockIDs ...iotago.BlockID) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		if builder.References == nil {
			builder.References = make(model.ParentReferences)
		}

		builder.References[iotago.WeakParentType] = blockIDs
	}
}

func WithShallowLikeParents(blockIDs ...iotago.BlockID) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		if builder.References == nil {
			builder.References = make(model.ParentReferences)
		}

		builder.References[iotago.ShallowLikeParentType] = blockIDs
	}
}

func WithSlotCommitment(commitment *iotago.Commitment) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.SlotCommitment = commitment
	}
}

func WithLatestFinalizedSlot(commitmentIndex iotago.SlotIndex) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.LatestFinalizedSlot = &commitmentIndex
	}
}

func WithPayload(payload iotago.Payload) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.Payload = payload
	}
}

func WithIssuingTime(issuingTime time.Time) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.IssuingTime = &issuingTime
	}
}

func WithProtocolVersion(version iotago.Version) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.ProtocolVersion = &version
	}
}
func WithIssuer(issuer Account) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.Issuer = issuer
	}
}

func WithHighestSupportedVersion(highestSupportedVersion iotago.Version) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.HighestSupportedVersion = &highestSupportedVersion
	}
}

func WithProtocolParametersHash(protocolParametersHash iotago.Identifier) func(builder *BlockParams) {
	return func(builder *BlockParams) {
		builder.ProtocolParametersHash = &protocolParametersHash
	}
}
