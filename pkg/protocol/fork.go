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
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (p *Protocol) processAttestationsRequest(commitmentID iotago.CommitmentID, src network.PeerID) {
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

func (p *Protocol) onForkDetected(fork *chainmanager.Fork) {
	blockIDs, shouldSwitch, banSrc, err := p.processFork(fork)
	if err != nil {
		p.ErrorHandler()(errors.Wrapf(err, "failed to handle fork %s at forking point %s from source %s", fork.ForkedChain.LatestCommitment().ID(), fork.ForkingPoint.ID(), fork.Source))
	}

	if banSrc {
		fmt.Println("TODO: ban source")
		// TODO: ban/drop peer
	}

	if !shouldSwitch {
		// TODO: The chain might become heavier in the future, or the neighbor could just keep sending stuff from a less heavy chain.
		//  what do we do in this case?
	}

	fmt.Println("Should switch chain", blockIDs)

	snapshotTargetIndex := fork.ForkingPoint.Index() - 1
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

type commitmentVerificationResult struct {
	commitment *model.Commitment
	blockIDs   iotago.BlockIDs
	err        error
}

func (p *Protocol) processFork(fork *chainmanager.Fork) (anchorBlockIDs iotago.BlockIDs, shouldSwitch, banSource bool, err error) {
	// TODO: should this already be done before we even solidify a chain?
	{
		// TODO: check slot of forked chain commitment not in future and that both chains have a similar height

		forkedChainLatestCommitment := fork.ForkedChain.LatestCommitment().Commitment()
		mainChainLatestCommitment := fork.MainChain.LatestCommitment().Commitment()

		// Check whether the chain is claiming to be heavier than the current main chain.
		if forkedChainLatestCommitment.CumulativeWeight() <= mainChainLatestCommitment.CumulativeWeight() {
			// TODO: ban source?
			return nil, false, false, errors.Errorf("%s with %d CW <= than main chain %d CW", forkedChainLatestCommitment.ID(), forkedChainLatestCommitment.CumulativeWeight(), mainChainLatestCommitment.CumulativeWeight())
		}
	}

	// Flow:
	//  1. request attestations starting from forking point + AttestationCommitmentOffset
	//  2. request 1 by 1
	//  3. verify commitment and attestations: evaluate CW until this point
	//  4. evaluate heuristic to determine if we should switch to the fork
	ch := make(chan *commitmentVerificationResult)
	defer close(ch)

	commitmentVerifier := NewCommitmentVerifier(p.MainEngineInstance())
	verifyCommitmentFunc := func(commitment *model.Commitment, attestations []*iotago.Attestation, merkleProof iotago.Identifier, _ network.PeerID) {
		blockIDs, err := commitmentVerifier.verifyCommitment(fork.ForkedChain.Commitment(commitment.PrevID().Index()).Commitment(), commitment, attestations, merkleProof)

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

	var heavierCount int
	// We start from the forking point + AttestationCommitmentOffset as that is where the CW of the chains starts diverging.
	// Only at this slot can nodes start to commit to the different chains.
	start := fork.ForkingPoint.Index() + p.MainEngineInstance().Attestations.AttestationCommitmentOffset()
	end := fork.ForkedChain.LatestCommitment().ID().Index()
	for i := start; i <= end; i++ {
		mainChainChainCommitment := fork.MainChain.Commitment(i)
		if mainChainChainCommitment == nil {
			// TODO: What do we do? mainchain is shorter than forked chain.
		}
		mainChainCommitment := mainChainChainCommitment.Commitment()

		// TODO: should we retry?
		p.networkProtocol.RequestAttestations(fork.ForkedChain.Commitment(i).ID(), fork.Source)

		select {
		case result := <-ch:
			if result.err != nil {
				return nil, false, true, errors.Wrapf(result.err, "failed to verify commitment %s", result.commitment.ID())
			}

			anchorBlockIDs = append(anchorBlockIDs, result.blockIDs...)

			// Count how many consecutive slots are heavier/lighter than the main chain.
			switch {
			case result.commitment.CumulativeWeight() > mainChainCommitment.CumulativeWeight():
				heavierCount++
			case result.commitment.CumulativeWeight() < mainChainCommitment.CumulativeWeight():
				heavierCount--
			default:
				heavierCount = 0
			}

			// Fork-choice rule: switch if p.optsChainSwitchingThreshold slots in a row are heavier than main chain.
			switch heavierCount {
			case p.optsChainSwitchingThreshold:
				return anchorBlockIDs, true, false, nil
			case -p.optsChainSwitchingThreshold:
				return nil, false, false, nil
			}
		case <-ctx.Done():
			return nil, false, false, errors.Wrapf(ctx.Err(), "failed to verify commitment for slot %d", i)
		}
	}

	// If the condition is not met in either direction, we don't switch the chain.
	return nil, false, false, nil
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
