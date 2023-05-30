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

	mutex        sync.RWMutex
	errorHandler func(error)

	optsSlotFinalizationThreshold float64

	module.Module
}

func NewProvider(opts ...options.Option[Gadget]) module.Provider[*engine.Engine, slotgadget.Gadget] {
	return module.Provide(func(e *engine.Engine) slotgadget.Gadget {
		return options.Apply(&Gadget{
			events:                        slotgadget.NewEvents(),
			optsSlotFinalizationThreshold: 0.67,
			errorHandler:                  e.ErrorHandler("slotgadget"),
		}, opts, func(g *Gadget) {
			g.sybilProtection = e.SybilProtection
			g.slotTracker = slottracker.NewSlotTracker(g.latestFinalizedSlot)

			e.Events.SlotGadget.LinkTo(g.events)

			e.Events.BlockGadget.BlockRatifiedAccepted.Hook(g.trackVotes)

			g.storeLastFinalizedSlotFunc = func(index iotago.SlotIndex) {
				if err := e.Storage.Settings().SetLatestFinalizedSlot(index); err != nil {
					g.errorHandler(errors.Wrap(err, "failed to set latest finalized slot"))
				}
			}

			e.HookConstructed(func() {
				g.workers = e.Workers.CreateGroup("SlotGadget")

				g.slotTracker.Events.VotersUpdated.Hook(func(evt *slottracker.VoterUpdatedEvent) {
					g.refreshSlotFinalization(evt.PrevLatestSlotIndex, evt.NewLatestSlotIndex)
				}, event.WithWorkerPool(g.workers.CreatePool("Refresh", 1)))

				e.HookInitialized(func() {
					// Can't use setter here as it has a side effect.
					func() {
						g.mutex.Lock()
						defer g.mutex.Unlock()
						g.lastFinalizedSlot = e.Storage.Permanent.Settings().LatestFinalizedSlot()
					}()

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

func (g *Gadget) latestFinalizedSlot() iotago.SlotIndex {
	g.mutex.RLock()
	defer g.mutex.RUnlock()

	return g.lastFinalizedSlot
}

func (g *Gadget) setLastFinalizedSlot(i iotago.SlotIndex) {
	g.lastFinalizedSlot = i
	g.storeLastFinalizedSlotFunc(i)
}

func (g *Gadget) trackVotes(block *blocks.Block) {
	g.slotTracker.TrackVotes(block.Block().SlotCommitment.Index, block.Block().IssuerID)
}

func (g *Gadget) refreshSlotFinalization(previousLatestSlotIndex iotago.SlotIndex, newLatestSlotIndex iotago.SlotIndex) {
	committee := g.sybilProtection.Committee()
	committeeTotalWeight := committee.TotalWeight()

	finalizedSlots := make([]iotago.SlotIndex, 0)
	g.mutex.Lock()
	for i := lo.Max(g.lastFinalizedSlot, previousLatestSlotIndex) + 1; i <= newLatestSlotIndex; i++ {
		attestorsTotalWeight := committee.SelectAccounts(g.slotTracker.Voters(i).Slice()...).TotalWeight()

		if !votes.IsThresholdReached(attestorsTotalWeight, committeeTotalWeight, g.optsSlotFinalizationThreshold) {
			break
		}

		g.setLastFinalizedSlot(i)

		finalizedSlots = append(finalizedSlots, i)
	}
	g.mutex.Unlock()

	for _, i := range finalizedSlots {
		g.events.SlotFinalized.Trigger(i)
		g.slotTracker.EvictSlot(i)
	}
}

var _ slotgadget.Gadget = new(Gadget)
