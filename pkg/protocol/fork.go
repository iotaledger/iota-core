package protocol

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (p *Protocol) ProcessAttestationsRequest(commitmentID iotago.CommitmentID, src network.PeerID) {
	mainEngine := p.MainEngineInstance()

	if mainEngine.Storage.Settings().LatestCommitment().Index() < commitmentID.Index() {
		return
	}

	commitment, err := mainEngine.Storage.Commitments().Load(commitmentID.Index())
	if err != nil {
		return
	}

	if commitment.ID() != commitmentID {
		return
	}

	attestations, err := mainEngine.Attestations.Get(commitmentID.Index())
	if err != nil {
		return
	}

	p.networkProtocol.SendAttestations(commitment, attestations, src)
}

func (p *Protocol) ProcessAttestations(commitment *model.Commitment, attestations []*iotago.Attestation, src network.PeerID) {
	if len(attestations) == 0 {
		p.ErrorHandler()(errors.Errorf("received attestations from peer %s are empty", src.String()))
		return
	}

	fork, exists := p.ChainManager.ForkByForkingPoint(commitment.ID())
	if !exists {
		p.ErrorHandler()(errors.Errorf("failed to get forking point for commitment %s", commitment.ID()))
		return
	}

	mainEngine := p.MainEngineInstance()

	snapshotTargetIndex := fork.ForkingPoint.Index() - 1

	snapshotTargetCommitment, err := mainEngine.Storage.Commitments().Load(snapshotTargetIndex)
	if err != nil {
		p.ErrorHandler()(errors.Wrapf(err, "failed to get commitment at snapshotTargetIndex %s", snapshotTargetIndex))
		return
	}

	calculatedCumulativeWeight := snapshotTargetCommitment.CumulativeWeight()

	visitedIdentities := make(map[iotago.AccountID]types.Empty)
	var blockIDs iotago.BlockIDs

	// TODO:
	//  1. we need to verify whether the public key is valid for the given account
	//  2. do we need to verify whether the blocks are part of the chain? eg via the attestation root? or even tangle root?
	for _, att := range attestations {
		if valid, err := att.VerifySignature(); !valid {
			if err != nil {
				p.ErrorHandler()(errors.Wrapf(err, "error validating attestation signature provided by %s", src))
				return
			}

			p.ErrorHandler()(errors.Errorf("invalid attestation signature provided by %s", src))

			return
		}

		if _, alreadyVisited := visitedIdentities[att.IssuerID]; alreadyVisited {
			p.ErrorHandler()(errors.Errorf("invalid attestation from source %s, issuerID %s contains multiple attestations", src, att.IssuerID))
			// TODO: ban source!
			return
		}

		// TODO: this might differ if we have a Accounts with changing weights depending on the SlotIndex
		if weight, weightExists := mainEngine.SybilProtection.Accounts().Get(att.IssuerID); weightExists {
			calculatedCumulativeWeight += uint64(weight)
		}

		visitedIdentities[att.IssuerID] = types.Void

		blockID, err := att.BlockID(mainEngine.API().SlotTimeProvider())
		if err != nil {
			p.ErrorHandler()(errors.Wrapf(err, "error calculating blockID from attestation"))
			return
		}

		blockIDs = append(blockIDs, blockID)
	}

	// Compare the calculated cumulative weight with ours to verify it is really higher
	weightAtForkedEventEnd := lo.PanicOnErr(mainEngine.Storage.Commitments().Load(fork.Commitment.Index())).CumulativeWeight()
	if calculatedCumulativeWeight <= weightAtForkedEventEnd {
		forkedEventClaimedWeight := fork.Commitment.CumulativeWeight()
		forkedEventMainWeight := lo.PanicOnErr(mainEngine.Storage.Commitments().Load(fork.Commitment.Index())).CumulativeWeight()
		p.ErrorHandler()(errors.Errorf("fork at point %d does not accumulate enough weight at slot %d calculated %d CW <= main chain %d CW. fork event detected at %d was %d CW > %d CW",
			fork.ForkingPoint.Index(),
			fork.Commitment.Index(),
			calculatedCumulativeWeight,
			weightAtForkedEventEnd,
			fork.Commitment.Index(),
			forkedEventClaimedWeight,
			forkedEventMainWeight))
		// TODO: ban source?
		return
	}

	candidateEngine, err := p.engineManager.ForkEngineAtSlot(snapshotTargetIndex)
	if err != nil {
		p.ErrorHandler()(errors.Wrap(err, "error creating new candidate engine"))
		return
	}

	// Set the chain to the correct forking point
	candidateEngine.SetChainID(fork.ForkingPoint.ID())

	// Attach the engine block requests to the protocol and detach as soon as we switch to that engine
	wp := candidateEngine.Workers.CreatePool("CandidateBlockRequester", 2)
	detachRequestBlocks := candidateEngine.Events.BlockRequester.Tick.Hook(func(blockID iotago.BlockID) {
		p.networkProtocol.RequestBlock(blockID)
	}, event.WithWorkerPool(wp)).Unhook

	// Attach slot commitments to the chain manager and detach as soon as we switch to that engine
	detachProcessCommitment := candidateEngine.Events.Notarization.SlotCommitted.Hook(func(details *notarization.SlotCommittedDetails) {
		p.ChainManager.ProcessCandidateCommitment(details.Commitment)
	}, event.WithWorkerPool(candidateEngine.Workers.CreatePool("ProcessCandidateCommitment", 2))).Unhook

	p.Events.MainEngineSwitched.Hook(func(_ *engine.Engine) {
		detachRequestBlocks()
		detachProcessCommitment()
	}, event.WithMaxTriggerCount(1))

	// Add all the blocks from the forking point attestations to the requester since those will not be passed to the engine by the protocol
	candidateEngine.BlockRequester.StartTickers(blockIDs)

	// Set the engine as the new candidate
	p.activeEngineMutex.Lock()
	oldCandidateEngine := p.candidateEngine
	p.candidateEngine = candidateEngine
	p.activeEngineMutex.Unlock()

	p.Events.CandidateEngineActivated.Trigger(candidateEngine)

	if oldCandidateEngine != nil {
		oldCandidateEngine.Shutdown()
		if err := oldCandidateEngine.RemoveFromFilesystem(); err != nil {
			p.ErrorHandler()(errors.Wrap(err, "error cleaning up replaced candidate engine from file system"))
		}
	}
}

func (p *Protocol) onForkDetected(fork *chainmanager.Fork) {
	claimedWeight := fork.Commitment.CumulativeWeight()
	mainChainCommitment, err := p.MainEngineInstance().Storage.Commitments().Load(fork.Commitment.Index())
	if err != nil {
		p.ErrorHandler()(errors.Errorf("failed to load commitment for main chain tip at index %d", fork.Commitment.Index()))
		return
	}

	mainChainWeight := mainChainCommitment.CumulativeWeight()

	if claimedWeight <= mainChainWeight {
		// TODO: ban source?
		p.ErrorHandler()(errors.Errorf("do not process fork %d with %d CW <= than main chain %d CW received from %s", fork.Commitment.ID(), claimedWeight, mainChainWeight, fork.Source))
		return
	}

	p.networkProtocol.RequestAttestations(fork.ForkingPoint.ID(), fork.Source)
}

func (p *Protocol) switchEngines() {
	var oldEngine *engine.Engine
	success := func() bool {
		p.activeEngineMutex.Lock()
		defer p.activeEngineMutex.Unlock()

		if p.candidateEngine == nil {
			return false
		}

		// Try to re-org the chain manager
		if err := p.ChainManager.SwitchMainChain(p.candidateEngine.Storage.Settings().LatestCommitment().ID()); err != nil {
			p.ErrorHandler()(errors.Wrap(err, "switching main chain failed"))

			return false
		}

		if err := p.engineManager.SetActiveInstance(p.candidateEngine); err != nil {
			p.ErrorHandler()(errors.Wrap(err, "error switching engines"))

			return false
		}

		p.linkToEngine(p.candidateEngine)

		// Save a reference to the current main engine and storage so that we can shut it down and prune it after switching
		oldEngine = p.mainEngine
		oldEngine.Shutdown()

		p.mainEngine = p.candidateEngine
		p.candidateEngine = nil

		return true
	}()

	if success {
		p.Events.MainEngineSwitched.Trigger(p.MainEngineInstance())

		// TODO: copy over old slots from the old engine to the new one

		// Cleanup filesystem
		if err := oldEngine.RemoveFromFilesystem(); err != nil {
			p.ErrorHandler()(errors.Wrap(err, "error removing storage directory after switching engines"))
		}
	}
}
