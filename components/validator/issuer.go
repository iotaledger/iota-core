package validator

import (
	"context"
	"time"

	"github.com/iotaledger/iota-core/pkg/blockfactory"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

func issueValidatorBlock(ctx context.Context) {
	blockIssuingTime := time.Now()
	nextBroadcast := blockIssuingTime.Add(ParamsValidator.CommitteeBroadcastInterval)

	// Use 'defer' because nextBroadcast is updated during function execution, and the value at the end needs to be used.
	defer func() {
		executor.ExecuteAt(accountID, func() { issueValidatorBlock(ctx) }, nextBroadcast)
	}()

	if !ParamsValidator.IgnoreBootstrapped && !deps.Protocol.MainEngineInstance().IsBootstrapped() {
		Component.LogDebug("Not issuing validator block because node is not bootstrapped yet.")

		return
	}

	var modelBlock *model.Block
	if deps.Protocol.MainEngineInstance().SybilProtection.SeatManager().Committee(deps.Protocol.CurrentAPI().TimeProvider().SlotFromTime(blockIssuingTime)).HasAccount(accountID) {
		var err error
		modelBlock, err = deps.BlockIssuer.CreateValidationBlock(ctx,
			blockfactory.WithIssuingTime(blockIssuingTime),
			blockfactory.WithPayload(&iotago.TaggedData{
				Tag: []byte("VALIDATOR BLOCK"),
			}),
		)
		if err != nil {
			Component.LogWarnf("error creating committee validator block: %s", err.Error())

			return
		}
	} else {
		var err error
		modelBlock, err = deps.BlockIssuer.CreateBlock(ctx,
			blockfactory.WithIssuingTime(blockIssuingTime),
			blockfactory.WithPayload(&iotago.TaggedData{
				Tag: []byte("CANDIDATE BLOCK"),
			}),
		)
		if err != nil {
			Component.LogWarnf("error creating candidate validator block: %s", err.Error())

			return
		}

		nextBroadcast = blockIssuingTime.Add(ParamsValidator.CommitteeBroadcastInterval)
	}

	if err := deps.BlockIssuer.IssueBlock(modelBlock); err != nil {
		Component.LogWarnf("error issuing validator block: %s", err.Error())

		return
	}

	Component.LogInfof("Issued validator block: %s - commitment %s %d - latest finalized slot %d", modelBlock.ID(), modelBlock.ProtocolBlock().SlotCommitmentID, modelBlock.ProtocolBlock().SlotCommitmentID.Index(), modelBlock.ProtocolBlock().LatestFinalizedSlot)

}
