package validator

import (
	"context"
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
)

var ErrBlockTooRecent = ierrors.New("block is too recent compared to latest commitment")

func issueValidatorBlock(ctx context.Context) {
	// Get the main engine instance in case it changes mid-execution.
	engineInstance := deps.Protocol.MainEngineInstance()

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

	protocolParametersHash, err := deps.Protocol.CommittedAPI().ProtocolParameters().Hash()
	if err != nil {
		Component.LogWarnf("failed to get protocol parameters hash: %s", err.Error())

		return
	}

	parents := engineInstance.TipSelection.SelectTips(iotago.BlockTypeValidationMaxParents)

	addressableCommitment, err := getAddressableCommitment(deps.Protocol.CommittedAPI().TimeProvider().SlotFromTime(blockIssuingTime))
	if err != nil && ierrors.Is(err, ErrBlockTooRecent) {
		commitment, parentID, reviveChainErr := reviveChain(blockIssuingTime)
		if reviveChainErr != nil {
			Component.LogError("error reviving chain: %s", reviveChainErr.Error())
			return
		}

		addressableCommitment = commitment
		parents = make(model.ParentReferences)
		parents[iotago.StrongParentType] = []iotago.BlockID{parentID}
	} else if err != nil {
		Component.LogWarnf("error getting commitment: %s", err.Error())

		return
	}

	// create the validation block here using the validation block builder from iota.go
	validationBlock, err := builder.NewValidationBlockBuilder(deps.Protocol.CommittedAPI()).
		IssuingTime(blockIssuingTime).
		ProtocolParametersHash(protocolParametersHash).
		SlotCommitmentID(addressableCommitment.MustID()).
		HighestSupportedVersion(deps.Protocol.LatestAPI().Version()).
		LatestFinalizedSlot(engineInstance.SyncManager.LatestFinalizedSlot()).
		StrongParents(parents[iotago.StrongParentType]).
		WeakParents(parents[iotago.WeakParentType]).
		ShallowLikeParents(parents[iotago.ShallowLikeParentType]).
		Sign(validatorAccount.ID(), validatorAccount.PrivateKey()).
		Build()
	if err != nil {
		Component.LogWarnf("error creating validation block: %s", err.Error())

		return
	}

	modelBlock, err := model.BlockFromBlock(validationBlock)
	if err != nil {
		Component.LogWarnf("error creating model block from validation block: %s", err.Error())

		return
	}

	blockSlot := deps.Protocol.CommittedAPI().TimeProvider().SlotFromTime(blockIssuingTime)
	committee, exists := engineInstance.SybilProtection.SeatManager().CommitteeInSlot(blockSlot)
	if !exists {
		Component.LogWarnf("committee for slot %d not selected: %s", blockSlot, err.Error())

		return
	}

	if !committee.HasAccount(validatorAccount.ID()) {
		// update nextBroadcast value here, so that this updated value is used in the `defer`
		// callback to schedule issuing of the next block at a different interval than for committee members
		nextBroadcast = blockIssuingTime.Add(ParamsValidator.CandidateBroadcastInterval)
	}

	if err = deps.BlockHandler.SubmitBlock(modelBlock); err != nil {
		Component.LogWarnf("error issuing validator block: %s", err.Error())

		return
	}

	Component.LogDebugf("Issued validator block: %s - commitment %s %d - latest finalized slot %d", modelBlock.ID(), modelBlock.ProtocolBlock().SlotCommitmentID, modelBlock.ProtocolBlock().SlotCommitmentID.Slot(), modelBlock.ProtocolBlock().LatestFinalizedSlot)
}

func reviveChain(issuingTime time.Time) (*iotago.Commitment, iotago.BlockID, error) {
	lastCommittedSlot := deps.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Slot()
	apiForSlot := deps.Protocol.APIForSlot(lastCommittedSlot)

	// Get a rootblock as recent as possible for the parent.
	parentBlockID := iotago.EmptyBlockID
	for rootBlock := range deps.Protocol.MainEngineInstance().EvictionState.ActiveRootBlocks() {
		if rootBlock.Slot() > parentBlockID.Slot() {
			parentBlockID = rootBlock
		}

		// Exit the loop if we found a rootblock in the last committed slot (which is the highest we can get).
		if parentBlockID.Slot() == lastCommittedSlot {
			break
		}
	}

	issuingSlot := apiForSlot.TimeProvider().SlotFromTime(issuingTime)

	// Force commitments until minCommittableAge relative to the block's issuing time. We basically "pretend" that
	// this block was already accepted at the time of issuing so that we have a commitment to reference.
	if issuingSlot < apiForSlot.ProtocolParameters().MinCommittableAge() { // Should never happen as we're beyond maxCommittableAge which is > minCommittableAge.
		return nil, iotago.EmptyBlockID, ierrors.Errorf("issuing slot %d is smaller than min committable age %d", issuingSlot, apiForSlot.ProtocolParameters().MinCommittableAge())
	}
	commitUntilSlot := issuingSlot - apiForSlot.ProtocolParameters().MinCommittableAge()

	if err := deps.Protocol.MainEngineInstance().Notarization.ForceCommitUntil(commitUntilSlot); err != nil {
		return nil, iotago.EmptyBlockID, ierrors.Wrapf(err, "failed to force commit until slot %d", commitUntilSlot)
	}

	commitment, err := deps.Protocol.MainEngineInstance().Storage.Commitments().Load(commitUntilSlot)
	if err != nil {
		return nil, iotago.EmptyBlockID, ierrors.Wrapf(err, "failed to commit until slot %d to revive chain", commitUntilSlot)
	}

	return commitment.Commitment(), parentBlockID, nil
}

func getAddressableCommitment(blockSlot iotago.SlotIndex) (*iotago.Commitment, error) {
	protoParams := deps.Protocol.CommittedAPI().ProtocolParameters()
	commitment := deps.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()

	if blockSlot > commitment.Slot+protoParams.MaxCommittableAge() {
		return nil, ierrors.Wrapf(ErrBlockTooRecent, "can't issue block: block slot %d is too far in the future, latest commitment is %d", blockSlot, commitment.Slot)
	}

	if blockSlot < commitment.Slot+protoParams.MinCommittableAge() {
		if blockSlot < protoParams.MinCommittableAge() || commitment.Slot < protoParams.MinCommittableAge() {
			return commitment, nil
		}

		commitmentSlot := commitment.Slot - protoParams.MinCommittableAge()
		loadedCommitment, err := deps.Protocol.MainEngineInstance().Storage.Commitments().Load(commitmentSlot)
		if err != nil {
			return nil, ierrors.Wrapf(err, "error loading valid commitment of slot %d according to minCommittableAge from storage", commitmentSlot)
		}

		return loadedCommitment.Commitment(), nil
	}

	return commitment, nil
}
