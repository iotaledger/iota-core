package activity

import (
	"context"

	"github.com/iotaledger/iota-core/pkg/blockfactory"
	iotago "github.com/iotaledger/iota.go/v4"
)

func issueActivityBlock(ctx context.Context) {
	if !ParamsActivity.IgnoreBootstrapped && !deps.Protocol.MainEngineInstance().IsBootstrapped() {
		Component.LogDebug("Not issuing activity block because node is not bootstrapped yet.")
		return
	}

	modelBlock, err := deps.BlockIssuer.CreateBlock(ctx, blockfactory.WithPayload(&iotago.TaggedData{
		Tag: []byte("ACTIVITY"),
	}))
	if err != nil {
		Component.LogWarnf("error creating activity block: %s", err.Error())
		return
	}

	if err := deps.BlockIssuer.IssueBlock(modelBlock); err != nil {
		Component.LogWarnf("error issuing activity block: %s", err.Error())
		return
	}

	Component.LogInfof("Issued activity block: %s - commitment %s %d - latest finalized slot %d", modelBlock.ID(), modelBlock.ProtocolBlock().SlotCommitmentID, modelBlock.ProtocolBlock().SlotCommitmentID.Index(), modelBlock.ProtocolBlock().LatestFinalizedSlot)
}
