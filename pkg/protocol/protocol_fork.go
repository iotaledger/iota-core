package protocol

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/timeutil"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

func (p *Protocol) processAttestationsRequest(commitmentID iotago.CommitmentID, src peer.ID) {
	mainEngine := p.MainEngineInstance()

	if mainEngine.Storage.Settings().LatestCommitment().Index() < commitmentID.Index() {
		return
	}

	commitment, err := mainEngine.Storage.Commitments().Load(commitmentID.Index())
	if err != nil {
		p.HandleError(ierrors.Wrapf(err, "failed to load commitment %s", commitmentID))
		return
	}

	if commitment.ID() != commitmentID {
		return
	}

	attestations, err := mainEngine.Attestations.Get(commitmentID.Index())
	if err != nil {
		p.HandleError(ierrors.Wrapf(err, "failed to load attestations for commitment %s", commitmentID))
		return
	}

	rootsStorage, err := mainEngine.Storage.Roots(commitmentID.Index())
	if err != nil {
		p.HandleError(ierrors.Errorf("failed to get roots storage for commitment %s", commitmentID))
		return
	}
	roots, err := rootsStorage.Load(commitmentID)
	if err != nil {
		p.HandleError(ierrors.Wrapf(err, "failed to load roots for commitment %s", commitmentID))
		return
	}

	p.networkProtocol.SendAttestations(commitment, attestations, roots.AttestationsProof(), src)
}

func (p *Protocol) onForkDetected(fork *chainmanager.Fork) {
	if candidateEngineInstance := p.CandidateEngineInstance(); candidateEngineInstance != nil && candidateEngineInstance.ChainID() == fork.ForkingPoint.ID() {
		p.HandleError(ierrors.Errorf("we are already processing the fork at forkingPoint %s", fork.ForkingPoint.ID()))
		return
	}

	if p.MainEngineInstance().ChainID() == fork.ForkingPoint.ID() {
		p.HandleError(ierrors.Errorf("we already switched our main engine to the fork at forkingPoint %s", fork.ForkingPoint.ID()))
		return
	}

	blockIDs, shouldSwitch, banSrc, err := p.processFork(fork)
	if err != nil {
		p.HandleError(ierrors.Wrapf(err, "failed to handle fork %s at forking point %s from source %s", fork.ForkedChain.LatestCommitment().ID(), fork.ForkingPoint.ID(), fork.Source))
		return
	}

	if banSrc {
		fmt.Println("TODO: ban source")
		// TODO: ban/drop peer
	}

	if !shouldSwitch {
		return
	}

	// When creating the candidate engine, there are 3 possible scenarios:
	//   1. The candidate engine becomes synced and its chain is heavier than the main chain -> switch to it.
	//   2. The candidate engine never becomes synced or its chain is not heavier than the main chain -> discard it after a timeout.
	//   3. The candidate engine is not creating the same commitments as the chain we decided to switch to -> discard it immediately.
	snapshotTargetIndex := fork.ForkingPoint.Index() - 1
	candidateEngineInstance, err := p.EngineManager.ForkEngineAtSlot(snapshotTargetIndex)
	if err != nil {
		p.HandleError(ierrors.Wrap(err, "error creating new candidate engine"))
		return
	}

	// Set the chain to the correct forking point
	candidateEngineInstance.SetChainID(fork.ForkingPoint.ID())

	// Attach the engine block requests to the protocol and detach as soon as we switch to that engine
	detachRequestBlocks := candidateEngineInstance.Events.BlockRequester.Tick.Hook(func(blockID iotago.BlockID) {
		p.networkProtocol.RequestBlock(blockID)
	}, event.WithWorkerPool(candidateEngineInstance.Workers.CreatePool("CandidateBlockRequester", 2))).Unhook

	var detachProcessCommitment, detachMainEngineSwitched func()
	candidateEngineTimeoutTimer := time.NewTimer(10 * time.Minute)

	cleanupFunc := func() {
		detachRequestBlocks()
		detachProcessCommitment()
		detachMainEngineSwitched()

		p.activeEngineMutex.Lock()
		p.candidateEngine = nil
		p.activeEngineMutex.Unlock()

		candidateEngineInstance.Shutdown()
		if err := candidateEngineInstance.RemoveFromFilesystem(); err != nil {
			p.HandleError(ierrors.Wrapf(err, "error cleaning up candidate engine %s from file system", candidateEngineInstance.Name()))
		}
	}

	// Attach slot commitments to the chain manager and detach as soon as we switch to that engine
	detachProcessCommitment = candidateEngineInstance.Events.Notarization.LatestCommitmentUpdated.Hook(func(commitment *model.Commitment) {
		// Check whether the commitment produced by syncing the candidate engine is actually part of the forked chain.
		if fork.ForkedChain.LatestCommitment().ID().Index() >= commitment.Index() {
			forkedChainCommitmentID := fork.ForkedChain.Commitment(commitment.Index()).ID()
			if forkedChainCommitmentID != commitment.ID() {
				p.HandleError(ierrors.Errorf("candidate engine %s produced a commitment %s that is not part of the forked chain %s", candidateEngineInstance.Name(), commitment.ID(), forkedChainCommitmentID))
				cleanupFunc()

				return
			}
		}

		p.ChainManager.ProcessCandidateCommitment(commitment)

		if candidateEngineInstance.SyncManager.IsBootstrapped() &&
			commitment.CumulativeWeight() > p.MainEngineInstance().Storage.Settings().LatestCommitment().CumulativeWeight() {
			p.switchEngines()
		}
	}, event.WithWorkerPool(candidateEngineInstance.Workers.CreatePool("ProcessCandidateCommitment", 1))).Unhook

	// Clean up events when we switch to the candidate engine.
	detachMainEngineSwitched = p.Events.MainEngineSwitched.Hook(func(_ *engine.Engine) {
		candidateEngineTimeoutTimer.Stop()
		detachRequestBlocks()
		detachProcessCommitment()
	}, event.WithMaxTriggerCount(1)).Unhook

	// Clean up candidate engine if we never switch to it.
	go func() {
		defer timeutil.CleanupTimer(candidateEngineTimeoutTimer)

		select {
		case <-candidateEngineTimeoutTimer.C:
			p.HandleError(ierrors.Errorf("timeout waiting for candidate engine %s to sync", candidateEngineInstance.Name()))
			cleanupFunc()
		case <-p.context.Done():
			// Nothing to do here. The candidate engine will be shutdown on protocol shutdown and cleaned up when starting the node again.
		}
	}()

	// Set the engine as the new candidate
	p.activeEngineMutex.Lock()
	oldCandidateEngine := p.candidateEngine
	p.candidateEngine = &candidateEngine{
		engine:      candidateEngineInstance,
		cleanupFunc: cleanupFunc,
	}
	p.activeEngineMutex.Unlock()

	// Add all the blocks from the forking point attestations to the requester since those will not be passed to the engine by the protocol
	candidateEngineInstance.BlockRequester.StartTickers(blockIDs)

	p.Events.CandidateEngineActivated.Trigger(candidateEngineInstance)

	if oldCandidateEngine != nil {
		oldCandidateEngine.cleanupFunc()
	}
}

type commitmentVerificationResult struct {
	commitment             *model.Commitment
	actualCumulativeWeight uint64
	blockIDs               iotago.BlockIDs
	err                    error
}

func (p *Protocol) processFork(fork *chainmanager.Fork) (anchorBlockIDs iotago.BlockIDs, shouldSwitch, banSource bool, err error) {
	// Flow:
	//  1. request attestations starting from forking point + AttestationCommitmentOffset
	//  2. request 1 by 1
	//  3. verify commitment and attestations: evaluate CW until this point
	//  4. evaluate heuristic to determine if we should switch to the fork
	ch := make(chan *commitmentVerificationResult)
	defer close(ch)

	commitmentVerifier := NewCommitmentVerifier(p.MainEngineInstance(), fork.MainChain.Commitment(fork.ForkingPoint.Index()-1).Commitment())
	verifyCommitmentFunc := func(commitment *model.Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier], _ peer.ID) {
		blockIDs, actualCumulativeWeight, err := commitmentVerifier.verifyCommitment(commitment, attestations, merkleProof)

		result := &commitmentVerificationResult{
			commitment:             commitment,
			actualCumulativeWeight: actualCumulativeWeight,
			blockIDs:               blockIDs,
			err:                    err,
		}
		ch <- result
	}

	wp := p.Workers.CreatePool("AttestationsVerifier", 1)
	unhook := p.Events.Network.AttestationsReceived.Hook(
		verifyCommitmentFunc,
		event.WithWorkerPool(wp),
	).Unhook
	defer unhook()
	defer wp.Shutdown()

	ctx, cancel := context.WithTimeout(p.context, 2*time.Minute)
	defer cancel()

	// Fork-choice rule: switch if p.optsChainSwitchingThreshold slots in a row are heavier than main chain.
	forkChoiceRule := func(heavierCount int) (decided bool, shouldSwitch bool) {
		switch heavierCount {
		case p.optsChainSwitchingThreshold:
			return true, true
		case -p.optsChainSwitchingThreshold:
			return true, false
		default:
			return false, false
		}
	}

	var heavierCount int
	// We start from the forking point to have all starting blocks for each slot. Even though the chain weight will only
	// start to diverge at forking point + AttestationCommitmentOffset.
	start := fork.ForkingPoint.Index()
	end := fork.ForkedChain.LatestCommitment().ID().Index()
	for i := start; i <= end; i++ {
		mainChainChainCommitment := fork.MainChain.Commitment(i)
		if mainChainChainCommitment == nil {
			// If the forked chain is longer than our main chain, we consider it to be heavier
			heavierCount++

			if decided, doSwitch := forkChoiceRule(heavierCount); decided {
				return anchorBlockIDs, doSwitch, false, nil
			}

			continue
		}
		mainChainCommitment := mainChainChainCommitment.Commitment()

		result, err := p.requestAttestation(ctx, fork.ForkedChain.Commitment(i).ID(), fork.Source, ch)
		if err != nil {
			return nil, false, true, ierrors.Wrapf(err, "failed to verify commitment %s", fork.ForkedChain.Commitment(i).ID())
		}

		// Count how many consecutive slots are heavier/lighter than the main chain.
		switch {
		case result.actualCumulativeWeight > mainChainCommitment.CumulativeWeight():
			heavierCount++
		case result.actualCumulativeWeight < mainChainCommitment.CumulativeWeight():
			heavierCount--
		default:
			heavierCount = 0
		}

		if decided, doSwitch := forkChoiceRule(heavierCount); decided {
			return anchorBlockIDs, doSwitch, false, nil
		}
	}

	// If the condition is not met in either direction, we don't switch the chain.
	return nil, false, false, nil
}

func (p *Protocol) requestAttestation(ctx context.Context, requestedID iotago.CommitmentID, src peer.ID, resultChan chan *commitmentVerificationResult) (*commitmentVerificationResult, error) {
	ticker := time.NewTicker(p.optsAttestationRequesterTryInterval)
	defer ticker.Stop()

	for i := 0; i < p.optsAttestationRequesterMaxRetries; i++ {
		p.networkProtocol.RequestAttestations(requestedID, src)

		select {
		case <-ticker.C:
			continue
		case result := <-resultChan:
			return result, result.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return nil, ierrors.Errorf("request attestation exceeds max retries from src: %s", src.String())
}

func (p *Protocol) switchEngines() {
	var oldEngine *engine.Engine
	success := func() bool {
		p.activeEngineMutex.Lock()
		defer p.activeEngineMutex.Unlock()

		if p.candidateEngine == nil {
			return false
		}

		candidateEngineInstance := p.candidateEngine.engine
		// Try to re-org the chain manager
		if err := p.ChainManager.SwitchMainChain(candidateEngineInstance.Storage.Settings().LatestCommitment().ID()); err != nil {
			p.HandleError(ierrors.Wrap(err, "switching main chain failed"))

			return false
		}

		if err := p.EngineManager.SetActiveInstance(candidateEngineInstance); err != nil {
			p.HandleError(ierrors.Wrap(err, "error switching engines"))

			return false
		}

		p.linkToEngine(candidateEngineInstance)

		// Save a reference to the current main engine and storage so that we can shut it down and prune it after switching
		oldEngine = p.mainEngine
		oldEngine.Shutdown()

		p.mainEngine = candidateEngineInstance
		p.candidateEngine = nil

		return true
	}()

	if success {
		p.Events.MainEngineSwitched.Trigger(p.MainEngineInstance())

		// Cleanup filesystem
		if err := oldEngine.RemoveFromFilesystem(); err != nil {
			p.HandleError(ierrors.Wrap(err, "error removing storage directory after switching engines"))
		}
	}
}
