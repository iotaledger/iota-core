package tresholdblockgadget

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/iotaledger/goshimmer/packages/core/markers"
	"github.com/iotaledger/hive.go/core/causalorder"
	"github.com/iotaledger/hive.go/core/slot"
	"github.com/iotaledger/hive.go/ds/walker"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/eviction"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection"
	iotago "github.com/iotaledger/iota.go/v4"
)

// region Gadget ///////////////////////////////////////////////////////////////////////////////////////////////////////

type Gadget struct {
	events *blockgadget.Events

	workers *workerpool.Group

	sybilProtection sybilprotection.SybilProtection
	blockCache      *blocks.Blocks

	acceptanceOrder   *causalorder.CausalOrder[iotago.SlotIndex, iotago.BlockID, *blocks.Block]
	confirmationOrder *causalorder.CausalOrder[iotago.SlotIndex, iotago.BlockID, *blocks.Block]

	optsAcceptanceThreshold   float64
	optsConfirmationThreshold float64

	module.Module
}

func NewProvider(opts ...options.Option[Gadget]) module.Provider[*engine.Engine, blockgadget.Gadget] {
	return module.Provide(func(e *engine.Engine) blockgadget.Gadget {
		g := New(e.SybilProtection, opts...)

		e.HookConstructed(func() {
			e.SybilProtection.HookInitialized(func() {
				g.Initialize(e.Workers.CreateGroup("BlockGadget"), e.BlockCache, e.EvictionState, e.SlotTimeProvider(), e.SybilProtection.Validators(), e.SybilProtection.Weights().TotalWeightWithoutZeroIdentity)
			})
		})

		return g
	})
}

func New(sybilProtection sybilprotection.SybilProtection, opts ...options.Option[Gadget]) *Gadget {
	return options.Apply(&Gadget{
		events: blockgadget.NewEvents(),

		sybilProtection: sybilProtection,

		optsAcceptanceThreshold:   0.67,
		optsConfirmationThreshold: 0.67,
	}, opts)
}

func (g *Gadget) Initialize(workers *workerpool.Group, blockCache *blocks.Blocks, evictionState *eviction.State, slotTimeProvider *slot.TimeProvider, validators *sybilprotection.WeightedSet, totalWeightCallback func() int64) {
	g.workers = workers
	g.blockCache = blockCache
	g.validators = validators
	g.totalWeightCallback = totalWeightCallback

	g.acceptanceOrder = causalorder.New(g.workers.CreatePool("AcceptanceOrder", 2), blockCache.Block, (*blocks.Block).IsAccepted, g.markAsAccepted, g.acceptanceFailed, (*blocks.Block).StrongParents)
	g.confirmationOrder = causalorder.New(g.workers.CreatePool("ConfirmationOrder", 2), blockCache.Block, (*blocks.Block).IsConfirmed, g.markAsConfirmed, g.confirmationFailed, (*blocks.Block).StrongParents)

	// TODO: revisit whole eviction
	// g.evictionState.Events.SlotEvicted.Hook(g.EvictUntil, event.WithWorkerPool(g.workers.CreatePool("Eviction", 1)))

	g.TriggerConstructed()
	g.TriggerInitialized()
}

func (g *Gadget) Events() *blockgadget.Events {
	return g.events
}

// IsBlockAccepted returns whether the given block is accepted.
func (g *Gadget) IsBlockAccepted(blockID iotago.BlockID) (accepted bool) {
	block, exists := g.blockCache.Block(blockID)
	return exists && block.IsAccepted()
}

func (g *Gadget) IsBlockConfirmed(blockID iotago.BlockID) bool {
	block, exists := g.blockCache.Block(blockID)
	return exists && block.IsConfirmed()
}

func (g *Gadget) RefreshSequence(sequenceID markers.SequenceID, newMaxSupportedIndex, prevMaxSupportedIndex markers.Index) {
	g.evictionMutex.RLock()

	var acceptedBlocks, confirmedBlocks []*blockgadget.Block

	totalWeight := g.totalWeightCallback()

	if lastAcceptedIndex, exists := g.lastAcceptedMarker.Get(sequenceID); exists {
		prevMaxSupportedIndex = lo.Max(prevMaxSupportedIndex, lastAcceptedIndex)
	}

	for markerIndex := prevMaxSupportedIndex; markerIndex <= newMaxSupportedIndex; markerIndex++ {
		marker, markerExists := g.booker.BlockCeiling(markers.NewMarker(sequenceID, markerIndex))
		if !markerExists {
			break
		}

		blocksToAccept, blocksToConfirm := g.tryConfirmOrAccept(totalWeight, marker)
		acceptedBlocks = append(acceptedBlocks, blocksToAccept...)
		confirmedBlocks = append(confirmedBlocks, blocksToConfirm...)

		markerIndex = marker.Index()
	}

	g.evictionMutex.RUnlock()

	// EVICTION
	for _, block := range acceptedBlocks {
		g.acceptanceOrder.Queue(block)
	}
	for _, block := range confirmedBlocks {
		g.confirmationOrder.Queue(block)
	}
}

// tryConfirmOrAccept checks if there is enough active weight to confirm blocks and then checks
// if the marker has accumulated enough witness weight to be both accepted and confirmed.
// Acceptance and Confirmation use the same threshold if confirmation is possible.
// If there is not enough online weight to achieve confirmation, then acceptance condition is evaluated based on total active weight.
func (g *Gadget) tryConfirmOrAccept(block *blocks.Block) {
	blockWeight := g.sybilProtection.Accounts().SelectAccounts(block.Witnesses()...).TotalWeight()
	committeeTotalWeight := g.sybilProtection.Committee().TotalWeight()
	onlineCommitteeTotalWeight := g.sybilProtection.OnlineCommittee().TotalWeight()

	// check if we reached the confirmation threshold based on the total commitee weight
	if IsThresholdReached(blockWeight, committeeTotalWeight, g.optsConfirmationThreshold) {
		// need to mark outside 'if' statement, otherwise only the first condition would be executed due to lazy evaluation

		// **************************************************
		// TODO: ************************************************** FIX ME this won't trigger an event
		// **************************************************
		accepted := block.SetAccepted()
		confirmed := block.SetConfirmed()
		if accepted || confirmed {
			g.propagateAcceptanceConfirmation(block, true)
			return
		}
	} else if IsThresholdReached(blockWeight, onlineCommitteeTotalWeight, g.optsAcceptanceThreshold) {
		if block.SetAccepted() {
			g.propagateAcceptanceConfirmation(block, false)
			return
		}
	}

	return
}

func (g *Gadget) EvictUntil(index slot.Index) {
	g.acceptanceOrder.EvictUntil(index)
	g.confirmationOrder.EvictUntil(index)
}

func (g *Gadget) propagateAcceptanceConfirmation(initialBlock *blocks.Block, confirmed bool) {
	pastConeWalker := walker.New[iotago.BlockID](false).Push(initialBlock.ID())
	for pastConeWalker.HasNext() {
		blockID := pastConeWalker.Next()
		walkerBlock, exists := g.blockCache.Block(blockID)
		if !exists {
			panic(fmt.Sprintf("parent %s does not exist", blockID))
		}

		shouldWalkPastCone := false
		if !walkerBlock.IsAccepted() {
			g.acceptanceOrder.Queue(walkerBlock)
			shouldWalkPastCone = true
		}

		if confirmed && !walkerBlock.IsConfirmed() {
			g.confirmationOrder.Queue(walkerBlock)
			shouldWalkPastCone = true
		}

		if !shouldWalkPastCone {
			continue
		}

		walkerBlock.ForEachParent(func(parent model.Parent) {
			switch parent.Type {
			case model.StrongParentType:
				pastConeWalker.Push(parent.ID)
			case model.ShallowLikeParentType, model.WeakParentType:
				if weakParent, exists := g.blockCache.Block(parent.ID); !exists {
					g.acceptanceOrder.Queue(weakParent)
					if confirmed {
						g.confirmationOrder.Queue(weakParent)
					}
				}
			}
		})
	}
}

func (g *Gadget) markAsAccepted(block *blocks.Block) (err error) {
	if block.SetAccepted() {
		g.events.BlockAccepted.Trigger(block)

		// TODO: set inclusion state of TX if block contains one.
		// set ConfirmationState of payload (applicable only to transactions)
		// if tx, ok := block.Transaction(); ok {
		// 	g.memPool.SetTransactionInclusionSlot(tx.ID(), g.slotTimeProvider.IndexFromTime(block.IssuingTime()))
		// }
	}

	return nil
}

func (g *Gadget) markAsConfirmed(block *blocks.Block) (err error) {
	if block.SetConfirmed() {
		g.events.BlockConfirmed.Trigger(block)
	}

	return nil
}

func (g *Gadget) acceptanceFailed(block *blocks.Block, err error) {
	g.events.Error.Trigger(errors.Wrapf(err, "could not mark block %s as accepted", block.ID()))
}

func (g *Gadget) confirmationFailed(block *blocks.Block, err error) {
	g.events.Error.Trigger(errors.Wrapf(err, "could not mark block %s as confirmed", block.ID()))
}

func (g *Gadget) evictSequence(sequenceID markers.SequenceID) {
	g.evictionMutex.Lock()
	defer g.evictionMutex.Unlock()

	g.lastAcceptedMarker.Delete(sequenceID)
	g.lastConfirmedMarker.Delete(sequenceID)
}

func IsThresholdReached(objectWeight, totalWeight int64, threshold float64) bool {
	return objectWeight > int64(float64(totalWeight)*threshold)
}

var _ blockgadget.Gadget = new(Gadget)
