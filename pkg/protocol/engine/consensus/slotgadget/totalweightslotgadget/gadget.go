package totalweightslotgadget

import (
	"sync"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/slotgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection"
	"github.com/iotaledger/iota-core/pkg/votes"
	"github.com/iotaledger/iota-core/pkg/votes/slottracker"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Gadget struct {
	events  *slotgadget.Events
	workers *workerpool.Group

	slotTracker     *slottracker.SlotTracker
	sybilProtection sybilprotection.SybilProtection

	lastFinalizedSlot          iotago.SlotIndex
	storeLastFinalizedSlotFunc func(index iotago.SlotIndex)

	mutex sync.RWMutex

	optsSlotFinalizationThreshold float64

	module.Module
}

func NewProvider(opts ...options.Option[Gadget]) module.Provider[*engine.Engine, slotgadget.Gadget] {
	return module.Provide(func(e *engine.Engine) slotgadget.Gadget {
		return options.Apply(&Gadget{
			events:                        slotgadget.NewEvents(),
			optsSlotFinalizationThreshold: 0.67,
		}, opts, func(g *Gadget) {
			g.sybilProtection = e.SybilProtection
			g.slotTracker = slottracker.NewSlotTracker(g.LatestFinalizedSlot)

			e.Events.SlotGadget.LinkTo(g.events)

			e.Events.BlockGadget.BlockRatifiedAccepted.Hook(g.trackVotes)

			g.storeLastFinalizedSlotFunc = func(index iotago.SlotIndex) {
				if err := e.Storage.Settings().SetLatestFinalizedSlot(index); err != nil {
					e.Events.Error.Trigger(errors.Wrap(err, "failed to set latest finalized slot"))
				}
			}

			e.HookConstructed(func() {
				g.workers = e.Workers.CreateGroup("SlotGadget")

				g.slotTracker.Events.VotersUpdated.Hook(func(evt *slottracker.VoterUpdatedEvent) {
					g.refreshSlotFinalization(evt.PrevLatestSlotIndex, evt.NewLatestSlotIndex)
				}, event.WithWorkerPool(g.workers.CreatePool("Refresh", 2)))

				e.HookInitialized(func() {
					g.lastFinalizedSlot = e.Storage.Permanent.Settings().LatestFinalizedSlot()
					g.TriggerInitialized()
				})
			})
		},
			(*Gadget).TriggerConstructed,
		)
	})
}

func (g *Gadget) Shutdown() {
	g.TriggerStopped()
	g.workers.Shutdown()
}

func (g *Gadget) LatestFinalizedSlot() iotago.SlotIndex {
	g.mutex.RLock()
	defer g.mutex.RUnlock()

	return g.lastFinalizedSlot
}

func (g *Gadget) setLastFinalizedSlot(i iotago.SlotIndex) {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	g.lastFinalizedSlot = i
	g.storeLastFinalizedSlotFunc(i)
}

func (g *Gadget) trackVotes(block *blocks.Block) {
	g.slotTracker.TrackVotes(block.Block().SlotCommitment.Index, block.Block().IssuerID)
}

func (g *Gadget) refreshSlotFinalization(previousLatestSlotIndex iotago.SlotIndex, newLatestSlotIndex iotago.SlotIndex) {
	committee := g.sybilProtection.Committee()
	committeeTotalWeight := committee.TotalWeight()

	for i := lo.Max(g.LatestFinalizedSlot(), previousLatestSlotIndex) + 1; i <= newLatestSlotIndex; i++ {
		attestorsTotalWeight := committee.SelectAccounts(g.slotTracker.Voters(i).Slice()...).TotalWeight()

		if !votes.IsThresholdReached(attestorsTotalWeight, committeeTotalWeight, g.optsSlotFinalizationThreshold) {
			break
		}

		// Lock here, so that SlotVotersTotalWeight is not inside the lock. Otherwise, it might cause a deadlock,
		// because one thread owns write-lock on VirtualVoting lock and needs read lock on SlotGadget lock,
		// while this method holds WriteLock on SlotGadget lock and is waiting for ReadLock on VirtualVoting.
		g.setLastFinalizedSlot(i)

		g.events.SlotFinalized.Trigger(i)

		g.slotTracker.EvictSlot(i)
	}
}

var _ slotgadget.Gadget = new(Gadget)
