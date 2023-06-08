package protocol

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
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

	rootsStorage := mainEngine.Storage.Roots(commitmentID.Index())
	if rootsStorage == nil {
		return
	}
	rootsBytes, err := rootsStorage.Get(kvstore.Key{prunable.RootsKey})
	if err != nil {
		return
	}
	var roots iotago.Roots
	lo.PanicOnErr(p.API().Decode(rootsBytes, &roots))

	p.networkProtocol.SendAttestations(commitment, attestations, roots.AttestationsProof(), src)
}

// func (p *Protocol) ProcessAttestations(commitment *model.Commitment, attestations []*iotago.Attestation, src network.PeerID) {
// 	fork, exists := p.ChainManager.ForkByForkingPoint(commitment.ID())
// 	if !exists {
// 		p.ErrorHandler()(errors.Errorf("failed to get forking point for commitment %s", commitment.ID()))
// 		return
// 	}
//
// 	mainEngine := p.MainEngineInstance()
//
// 	snapshotTargetIndex := fork.ForkingPoint.Index() - 1
//
// 	snapshotTargetCommitment, err := mainEngine.Storage.Commitments().Load(snapshotTargetIndex)
// 	if err != nil {
// 		p.ErrorHandler()(errors.Wrapf(err, "failed to get commitment at snapshotTargetIndex %s", snapshotTargetIndex))
// 		return
// 	}
//
// 	calculatedCumulativeWeight := snapshotTargetCommitment.CumulativeWeight()
//
// 	// TODO: check from here
//
// 	// Compare the calculated cumulative weight with ours to verify it is really higher
// 	weightAtForkedEventEnd := lo.PanicOnErr(mainEngine.Storage.Commitments().Load(fork.Commitment.Index())).CumulativeWeight()
// 	if calculatedCumulativeWeight <= weightAtForkedEventEnd {
// 		forkedEventClaimedWeight := fork.Commitment.CumulativeWeight()
// 		forkedEventMainWeight := lo.PanicOnErr(mainEngine.Storage.Commitments().Load(fork.Commitment.Index())).CumulativeWeight()
// 		p.ErrorHandler()(errors.Errorf("fork at point %d does not accumulate enough weight at slot %d calculated %d CW <= main chain %d CW. fork event detected at %d was %d CW > %d CW",
// 			fork.ForkingPoint.Index(),
// 			fork.Commitment.Index(),
// 			calculatedCumulativeWeight,
// 			weightAtForkedEventEnd,
// 			fork.Commitment.Index(),
// 			forkedEventClaimedWeight,
// 			forkedEventMainWeight))
// 		// TODO: ban source?
// 		return
// 	}
//
// 	candidateEngine, err := p.engineManager.ForkEngineAtSlot(snapshotTargetIndex)
// 	if err != nil {
// 		p.ErrorHandler()(errors.Wrap(err, "error creating new candidate engine"))
// 		return
// 	}
//
// 	// Set the chain to the correct forking point
// 	candidateEngine.SetChainID(fork.ForkingPoint.ID())
//
// 	// Attach the engine block requests to the protocol and detach as soon as we switch to that engine
// 	wp := candidateEngine.Workers.CreatePool("CandidateBlockRequester", 2)
// 	detachRequestBlocks := candidateEngine.Events.BlockRequester.Tick.Hook(func(blockID iotago.BlockID) {
// 		p.networkProtocol.RequestBlock(blockID)
// 	}, event.WithWorkerPool(wp)).Unhook
//
// 	// Attach slot commitments to the chain manager and detach as soon as we switch to that engine
// 	detachProcessCommitment := candidateEngine.Events.Notarization.SlotCommitted.Hook(func(details *notarization.SlotCommittedDetails) {
// 		p.ChainManager.ProcessCandidateCommitment(details.Commitment)
// 	}, event.WithWorkerPool(candidateEngine.Workers.CreatePool("ProcessCandidateCommitment", 2))).Unhook
//
// 	p.Events.MainEngineSwitched.Hook(func(_ *engine.Engine) {
// 		detachRequestBlocks()
// 		detachProcessCommitment()
// 	}, event.WithMaxTriggerCount(1))
//
// 	// Add all the blocks from the forking point attestations to the requester since those will not be passed to the engine by the protocol
// 	candidateEngine.BlockRequester.StartTickers(blockIDs)
//
// 	// Set the engine as the new candidate
// 	p.activeEngineMutex.Lock()
// 	oldCandidateEngine := p.candidateEngine
// 	p.candidateEngine = candidateEngine
// 	p.activeEngineMutex.Unlock()
//
// 	p.Events.CandidateEngineActivated.Trigger(candidateEngine)
//
// 	if oldCandidateEngine != nil {
// 		oldCandidateEngine.Shutdown()
// 		if err := oldCandidateEngine.RemoveFromFilesystem(); err != nil {
// 			p.ErrorHandler()(errors.Wrap(err, "error cleaning up replaced candidate engine from file system"))
// 		}
// 	}
// }

func (p *Protocol) onForkDetected(fork *chainmanager.Fork) {
	banSrc, err := p.handleFork(fork)
	if err != nil {
		p.ErrorHandler()(errors.Wrapf(err, "failed to handle fork %s at forking point %s from source %s", fork.Commitment.ID(), fork.ForkingPoint.ID(), fork.Source))
	}
	if banSrc {
		fmt.Println("TODO: ban source")
		// TODO: ban/drop peer
	}
}

type commitmentVerificationResult struct {
	commitment *model.Commitment
	blockIDs   iotago.BlockIDs
	err        error
}

func (p *Protocol) handleFork(fork *chainmanager.Fork) (banSource bool, err error) {
	// TODO: check slot of commitment not in future
	claimedWeight := fork.Commitment.CumulativeWeight()
	mainChainCommitment, err := p.MainEngineInstance().Storage.Commitments().Load(fork.Commitment.Index())
	if err != nil {
		return false, errors.Errorf("failed to load commitment for main chain tip at index %d", fork.Commitment.Index())
	}

	if claimedWeight <= mainChainCommitment.CumulativeWeight() {
		// TODO: ban source?
		return false, errors.Errorf("do not process fork %d with %d CW <= than main chain %d CW received from %s", fork.Commitment.ID(), claimedWeight, mainChainCommitment.CumulativeWeight(), fork.Source)
	}

	// Flow:
	//  1. request attestations starting from forking point + offsets
	//  2. request 1 by 1
	//  3. verify commitment and attestations: evaluate CW until this point
	//  4. evaluate heuristic to determine if we should switch to the fork
	ch := make(chan *commitmentVerificationResult)
	defer close(ch)

	commitmentVerifier := NewCommitmentVerifier(p.MainEngineInstance())
	verifyCommitmentFunc := func(commitment *model.Commitment, attestations []*iotago.Attestation, merkleProof iotago.Identifier, id network.PeerID) {
		fmt.Println("attestations received", commitment.Index(), len(attestations), id)

		blockIDs, err := commitmentVerifier.verifyCommitment(fork.Chain.Commitment(commitment.PrevID().Index()).Commitment(), commitment, attestations, merkleProof)

		result := &commitmentVerificationResult{
			commitment: commitment,
			blockIDs:   blockIDs,
			err:        err,
		}
		ch <- result
	}

	unhook := p.Events.Network.AttestationsReceived.Hook(
		verifyCommitmentFunc,
		event.WithWorkerPool(p.Workers.CreatePool("AttestationsVerifier", 1)),
	).Unhook
	defer unhook()

	ctx, cancel := context.WithTimeout(p.context, 2*time.Minute)
	defer cancel()

	start := fork.ForkingPoint.Index() + 1 // TODO: add offset here
	end := fork.Commitment.Index()
	for i := start; i <= end; i++ {
		// TODO: should we retry?
		p.networkProtocol.RequestAttestations(fork.Chain.Commitment(i).ID(), fork.Source)

		select {
		case result := <-ch:
			if result.err != nil {
				return true, errors.Wrapf(result.err, "failed to verify commitment %s", result.commitment.ID())
			}

			// TODO: check for whether heuristic is met: 3 slots in a row heavier than main chain
			//  1. create new engine and queue all blocks from attestations
			//  or exit if main chain is heavier

		case <-ctx.Done():
			return false, errors.Wrapf(ctx.Err(), "failed to verify commitment for slot %d", i)
		}
	}

	fmt.Println("fork detected: all attestations processed", fork.ForkingPoint.ID(), fork.Commitment.ID(), fork.Source)
	return false, nil
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
