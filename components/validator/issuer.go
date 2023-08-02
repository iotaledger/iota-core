package validator

import (
	"context"
	"time"

	"github.com/iotaledger/iota-core/pkg/blockfactory"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

func issueValidatorBlock(ctx context.Context) {
	executor.ExecuteAfter(accountID, func() { issueValidatorBlock(ctx) }, ParamsValidator.BroadcastInterval)

	if !ParamsValidator.IgnoreBootstrapped && !deps.Protocol.MainEngineInstance().IsBootstrapped() {
		Component.LogDebug("Not issuing validator block because node is not bootstrapped yet.")
		return
	}
	blockIssuingTime := time.Now()
	var modelBlock *model.Block
	if deps.Protocol.MainEngineInstance().SybilProtection.SeatManager().Committee(deps.Protocol.CurrentAPI().TimeProvider().SlotFromTime(blockIssuingTime)).HasAccount(accountID) {
		var err error
		modelBlock, err = deps.BlockIssuer.CreateValidationBlock(ctx, blockfactory.WithPayload(&iotago.TaggedData{
			Tag: []byte("VALIDATOR BLOCK"),
		}))
		if err != nil {
			Component.LogWarnf("error creating validator block: %s", err.Error())
			return
		}
	} else {
		// TODO: once we know how candidates are evaluated, adjust this part.
		var err error
		modelBlock, err = deps.BlockIssuer.CreateBlock(ctx, blockfactory.WithPayload(&iotago.TaggedData{
			Tag: []byte("CANDIDATE BLOCK"),
		}))
		if err != nil {
			Component.LogWarnf("error creating validator candidate block: %s", err.Error())
			return
		}
	}

	if err := deps.BlockIssuer.IssueBlock(modelBlock); err != nil {
		Component.LogWarnf("error issuing validator block: %s", err.Error())
		return
	}

	Component.LogInfof("Issued validator block: %s - commitment %s %d - latest finalized slot %d", modelBlock.ID(), modelBlock.ProtocolBlock().SlotCommitmentID, modelBlock.ProtocolBlock().SlotCommitmentID.Index(), modelBlock.ProtocolBlock().LatestFinalizedSlot)

}
