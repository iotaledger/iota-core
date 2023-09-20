package validator

import (
	"context"
	"time"

	"github.com/iotaledger/iota-core/pkg/blockfactory"
)

func issueValidatorBlock(ctx context.Context) {
	// Get the main engine instance in case it changes mid-execution.
	engineInstance := deps.Protocol.MainEngineInstance()

	// Get the latest commitment from the engine before to avoid race conditions if something is committed after we fix block issuing time.
	latestCommitment := engineInstance.Storage.Settings().LatestCommitment()

	blockIssuingTime := time.Now()
	nextBroadcast := blockIssuingTime.Add(ParamsValidator.CommitteeBroadcastInterval)

	// Use 'defer' because nextBroadcast is updated during function execution, and the value at the end needs to be used.
	defer func() {
		executor.ExecuteAt(validatorAccount.ID(), func() { issueValidatorBlock(ctx) }, nextBroadcast)
	}()

	if !ParamsValidator.IgnoreBootstrapped && !engineInstance.SyncManager.IsBootstrapped() {
		Component.LogDebug("Not issuing validator block because node is not bootstrapped yet.")

		return
	}

	protocolParametersHash, err := deps.Protocol.CurrentAPI().ProtocolParameters().Hash()
	if err != nil {
		Component.LogWarnf("failed to get protocol parameters hash: %s", err.Error())

		return
	}

	modelBlock, err := deps.BlockIssuer.CreateValidationBlock(ctx,
		validatorAccount,
		blockfactory.WithValidationBlockHeaderOptions(
			blockfactory.WithIssuingTime(blockIssuingTime),
			blockfactory.WithSlotCommitment(latestCommitment.Commitment()),
		),
		blockfactory.WithProtocolParametersHash(protocolParametersHash),
		blockfactory.WithHighestSupportedVersion(deps.Protocol.LatestAPI().Version()),
	)
	if err != nil {
		Component.LogWarnf("error creating validator block: %s", err.Error())

		return
	}

	if !engineInstance.SybilProtection.SeatManager().Committee(deps.Protocol.CurrentAPI().TimeProvider().SlotFromTime(blockIssuingTime)).HasAccount(validatorAccount.ID()) {
		// update nextBroadcast value here, so that this updated value is used in the `defer`
		// callback to schedule issuing of the next block at a different interval than for committee members
		nextBroadcast = blockIssuingTime.Add(ParamsValidator.CandidateBroadcastInterval)
	}

	if err = deps.BlockIssuer.IssueBlock(modelBlock); err != nil {
		Component.LogWarnf("error issuing validator block: %s", err.Error())

		return
	}

	Component.LogDebugf("Issued validator block: %s - commitment %s %d - latest finalized slot %d", modelBlock.ID(), modelBlock.ProtocolBlock().SlotCommitmentID, modelBlock.ProtocolBlock().SlotCommitmentID.Index(), modelBlock.ProtocolBlock().LatestFinalizedSlot)

}
