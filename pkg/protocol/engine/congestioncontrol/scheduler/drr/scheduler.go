package drr

import (
	"math"
	"time"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/congestioncontrol/scheduler"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type Scheduler struct {
	events *scheduler.Events

	quantumFunc func(iotago.AccountID) (iotago.Mana, error)

	apiProvider api.Provider

	buffer      *BufferQueue
	bufferMutex syncutils.RWMutex

	deficits *shrinkingmap.ShrinkingMap[iotago.AccountID, iotago.WorkScore]

	tokenBucket      float64
	lastScheduleTime time.Time

	shutdownSignal chan struct{}

	blockChan chan *blocks.Block

	blockCache *blocks.Blocks

	module.Module
}

func NewProvider(opts ...options.Option[Scheduler]) module.Provider[*engine.Engine, scheduler.Scheduler] {
	return module.Provide(func(e *engine.Engine) scheduler.Scheduler {
		s := New(e, opts...)
		s.buffer = NewBufferQueue(int(s.apiProvider.CurrentAPI().ProtocolParameters().CongestionControlParameters().MaxBufferSize))
		e.HookConstructed(func() {
			s.blockCache = e.BlockCache
			e.Events.Scheduler.LinkTo(s.events)
			e.Ledger.HookInitialized(func() {
				// Mana retrieve function gets the account's Mana and returns the quantum for that account
				s.quantumFunc = func(accountID iotago.AccountID) (iotago.Mana, error) {
					manaSlot := e.Storage.Settings().LatestCommitment().Index()
					mana, err := e.Ledger.ManaManager().GetManaOnAccount(accountID, manaSlot)
					if err != nil {
						return 0, err
					}

					minMana := s.apiProvider.CurrentAPI().ProtocolParameters().CongestionControlParameters().MinMana
					if mana < minMana {
						return mana, ierrors.Errorf("account %s has insufficient Mana for block to be scheduled: account Mana %d, min Mana %d", accountID, mana, minMana)
					}

					return mana / minMana, nil
				}
			})
			s.TriggerConstructed()
			e.Events.Booker.BlockBooked.Hook(func(block *blocks.Block) {
				s.AddBlock(block)
				s.selectBlockToScheduleWithLocking()
			})

			e.HookInitialized(s.Start)
		})

		return s
	})
}

func New(apiProvider api.Provider, opts ...options.Option[Scheduler]) *Scheduler {
	return options.Apply(
		&Scheduler{
			events:           scheduler.NewEvents(),
			lastScheduleTime: time.Now(),
			deficits:         shrinkingmap.New[iotago.AccountID, iotago.WorkScore](),
			apiProvider:      apiProvider,
		}, opts,
	)
}

func (s *Scheduler) Shutdown() {
	close(s.shutdownSignal)
	s.TriggerStopped()
}

// Start starts the scheduler.
func (s *Scheduler) Start() {
	s.shutdownSignal = make(chan struct{}, 1)
	s.blockChan = make(chan *blocks.Block, 1)
	go s.mainLoop()

	s.TriggerInitialized()
}

// Rate gets the rate of the scheduler in units of work per second.
func (s *Scheduler) Rate() iotago.WorkScore {
	return s.apiProvider.CurrentAPI().ProtocolParameters().CongestionControlParameters().SchedulerRate
}

// IssuerQueueSizeCount returns the queue size of the given issuer as block count.
func (s *Scheduler) IssuerQueueBlockCount(issuerID iotago.AccountID) int {
	s.bufferMutex.RLock()
	defer s.bufferMutex.RUnlock()

	return s.buffer.IssuerQueue(issuerID).Size()
}

// IssuerQueueWork returns the queue size of the given issuer in work units.
func (s *Scheduler) IssuerQueueWork(issuerID iotago.AccountID) iotago.WorkScore {
	s.bufferMutex.RLock()
	defer s.bufferMutex.RUnlock()

	return s.buffer.IssuerQueue(issuerID).Work()
}

// BufferSize returns the current buffer size of the Scheduler as block count.
func (s *Scheduler) BufferSize() int {
	s.bufferMutex.RLock()
	defer s.bufferMutex.RUnlock()

	return s.buffer.Size()
}

// MaxBufferSize returns the max buffer size of the Scheduler as block count.
func (s *Scheduler) MaxBufferSize() int {
	return int(s.apiProvider.CurrentAPI().ProtocolParameters().CongestionControlParameters().MaxBufferSize)
}

// ReadyBlocksCount returns the number of ready blocks.
func (s *Scheduler) ReadyBlocksCount() int {
	s.bufferMutex.RLock()
	defer s.bufferMutex.RUnlock()

	return s.buffer.ReadyBlocksCount()
}

func (s *Scheduler) IsBlockIssuerReady(accountID iotago.AccountID, blocks ...*blocks.Block) bool {
	s.bufferMutex.RLock()
	defer s.bufferMutex.RUnlock()

	// if the buffer is completely empty, any issuer can issue a block.
	if s.buffer.Size() == 0 {
		return true
	}
	work := iotago.WorkScore(0)
	// if no specific block(s) is provided, assume max block size
	currentAPI := s.apiProvider.CurrentAPI()
	if len(blocks) == 0 {
		work = currentAPI.MaxBlockWork()
	}
	for _, block := range blocks {
		work += block.WorkScore()
	}
	deficit, err := s.getDeficit(accountID)
	if err != nil {
		return false
	}

	return deficit >= work+s.buffer.IssuerQueue(accountID).Work()
}

func (s *Scheduler) AddBlock(block *blocks.Block) {
	s.bufferMutex.Lock()
	defer s.bufferMutex.Unlock()

	droppedBlocks, err := s.buffer.Submit(block, s.quantumFunc)
	// error submitting indicates that the block was already submitted so we do nothing else.
	if err != nil {
		return
	}
	for _, b := range droppedBlocks {
		b.SetDropped()
		s.events.BlockDropped.Trigger(b, ierrors.New("block dropped from buffer"))
	}
	if block.SetEnqueued() {
		s.events.BlockEnqueued.Trigger(block)
		s.tryReady(block)
	}
}

func (s *Scheduler) mainLoop() {
	var blockToSchedule *blocks.Block
loop:
	for {
		select {
		// on close, exit the loop
		case <-s.shutdownSignal:
			break loop
		// when a block is pushed by the buffer
		case blockToSchedule = <-s.blockChan:
			currentAPI := s.apiProvider.CurrentAPI()
			rate := currentAPI.ProtocolParameters().CongestionControlParameters().SchedulerRate
			tokensRequired := float64(blockToSchedule.WorkScore()) - (s.tokenBucket + float64(rate)*time.Since(s.lastScheduleTime).Seconds())
			if tokensRequired > 0 {
				// wait until sufficient tokens in token bucket
				timer := time.NewTimer(time.Duration(tokensRequired/float64(rate)) * time.Second)
				<-timer.C
			}
			s.tokenBucket = lo.Min(
				float64(currentAPI.MaxBlockWork()),
				s.tokenBucket+float64(rate)*time.Since(s.lastScheduleTime).Seconds(),
			)
			s.lastScheduleTime = time.Now()
			s.scheduleBlock(blockToSchedule)
		}
	}
}

func (s *Scheduler) scheduleBlock(block *blocks.Block) {
	if block.SetScheduled() {
		// deduct tokens from the token bucket according to the scheduled block's work.
		s.tokenBucket -= float64(block.WorkScore())

		// check for another block ready to schedule
		s.updateChildrenWithLocking(block)
		s.selectBlockToScheduleWithLocking()

		s.events.BlockScheduled.Trigger(block)
	}
}

func (s *Scheduler) selectBlockToScheduleWithLocking() {
	s.bufferMutex.Lock()
	defer s.bufferMutex.Unlock()

	// already a block selected to be scheduled.
	if len(s.blockChan) > 0 {
		return
	}
	start := s.buffer.Current()
	// no blocks submitted
	if start == nil {
		return
	}

	rounds, schedulingIssuer := s.selectIssuer(start)

	// if there is no issuer with a ready block, we cannot schedule anything
	if schedulingIssuer == nil {
		return
	}

	if rounds > 0 {
		// increment every issuer's deficit for the required number of rounds
		for q := start; ; {
			if err := s.incrementDeficit(q.IssuerID(), rounds); err != nil {
				s.removeIssuer(q, err)
				q = s.buffer.Current()
			} else {
				q = s.buffer.Next()
			}
			if q == nil {
				return
			}
			if q == start {
				break
			}
		}
	}
	// increment the deficit for all issuers before schedulingIssuer one more time
	for q := start; q != schedulingIssuer; q = s.buffer.Next() {
		if err := s.incrementDeficit(q.IssuerID(), 1); err != nil {
			s.removeIssuer(q, err)

			return
		}
	}

	// remove the block from the buffer and adjust issuer's deficit
	block := s.buffer.PopFront()
	issuerID := block.ProtocolBlock().IssuerID
	err := s.updateDeficit(issuerID, int64(-block.WorkScore()))
	if err != nil {
		// if something goes wrong with deficit update, drop the block instead of scheduling it.
		block.SetDropped()
		s.events.BlockDropped.Trigger(block, ierrors.New("error updating deficit"))
	}
	s.blockChan <- block
}

func (s *Scheduler) selectIssuer(start *IssuerQueue) (int64, *IssuerQueue) {
	rounds := int64(math.MaxInt64)
	var schedulingIssuer *IssuerQueue

	for q := start; ; {
		block := q.Front()
		var issuerRemoved bool

		for block != nil && time.Now().After(block.IssuingTime()) {
			currentAPI := s.apiProvider.CurrentAPI()
			if block.IsAccepted() && time.Since(block.IssuingTime()) > time.Duration(currentAPI.TimeProvider().SlotDurationSeconds()*int64(currentAPI.ProtocolParameters().MaxCommittableAge())) {
				if block.SetSkipped() {
					s.updateChildrenWithoutLocking(block)
					s.events.BlockSkipped.Trigger(block)
				}

				s.buffer.PopFront()

				block = q.Front()

				continue
			}

			issuerID := block.ProtocolBlock().IssuerID

			// compute how often the deficit needs to be incremented until the block can be scheduled
			deficit, err := s.getDeficit(issuerID)
			if err != nil {
				// no deficit exists for this issuer queue, so remove it
				s.removeIssuer(q, err)
				issuerRemoved = true

				break
			}

			remainingDeficit := int64(block.WorkScore()) - int64(deficit)
			// calculate how many rounds we need to skip to accumulate enough deficit.
			quantum, err := s.quantumFunc(issuerID)
			if err != nil {
				// if quantum, can't be retrieved, we need to remove this issuer.
				s.removeIssuer(q, err)
				issuerRemoved = true

				break
			}

			r := (remainingDeficit + int64(quantum) - 1) / int64(quantum) // round up division result
			// find the first issuer that will be allowed to schedule a block
			if r < rounds {
				rounds = r
				schedulingIssuer = q
			}

			break
		}

		if issuerRemoved {
			q = s.buffer.Current()
		} else {
			q = s.buffer.Next()
		}
		if q == start || q == nil {
			break
		}
	}

	return rounds, schedulingIssuer
}

func (s *Scheduler) removeIssuer(q *IssuerQueue, err error) {
	q.submitted.ForEach(func(id iotago.BlockID, block *blocks.Block) bool {
		block.SetDropped()
		s.events.BlockDropped.Trigger(block, err)

		return true
	})

	for q.inbox.Len() > 0 {
		block := q.PopFront()
		block.SetDropped()
		s.events.BlockDropped.Trigger(block, err)
	}

	s.buffer.RemoveIssuer(q.IssuerID())
}

func (s *Scheduler) getDeficit(accountID iotago.AccountID) (iotago.WorkScore, error) {
	d, exists := s.deficits.Get(accountID)
	if !exists {
		_, err := s.quantumFunc(accountID)
		// quantumFunc returns error if mana is less than MinMana or the Mana can not be retrieved for the account.
		if err != nil {
			return 0, ierrors.Wrapf(err, "could not get deficit for issuer %s", accountID)
		}
		// load with max deficit if the issuer has Mana but has been removed from the deficits map
		return s.apiProvider.CurrentAPI().MaxBlockWork(), nil
	}

	return d, nil
}

func (s *Scheduler) updateDeficit(accountID iotago.AccountID, delta int64) error {
	var err error
	s.deficits.Compute(accountID, func(currentValue iotago.WorkScore, exists bool) iotago.WorkScore {
		if !exists {
			_, err = s.quantumFunc(accountID)
			// quantumFunc returns error if mana is less than MinMana or the Mana can not be retrieved for the account.
			if err != nil {
				err = ierrors.Wrapf(err, "could not get deficit for issuer %s", accountID)
				return 0
			}

			// load with max deficit if the issuer has Mana but has been removed from the deficits map
			return s.apiProvider.CurrentAPI().MaxBlockWork()
		}

		// TODO: use safemath package to prevent underflow
		if int64(currentValue)+delta < 0 {
			err = ierrors.Errorf("tried to decrease deficit to a negative value %d for issuer %s", int64(currentValue)+delta, accountID)
			return 0
		}

		return lo.Min(iotago.WorkScore(int64(currentValue)+delta), s.apiProvider.CurrentAPI().MaxBlockWork())
	})

	if err != nil {
		s.deficits.Delete(accountID)

		return err
	}

	return nil
}

func (s *Scheduler) incrementDeficit(issuerID iotago.AccountID, rounds int64) error {
	quantum, err := s.quantumFunc(issuerID)
	if err != nil {
		return err
	}

	return s.updateDeficit(issuerID, int64(quantum)*rounds)
}

func (s *Scheduler) isEligible(block *blocks.Block) (eligible bool) {
	return block.IsSkipped() || block.IsScheduled() || block.IsAccepted()
}

// isReady returns true if the given blockID's parents are eligible.
func (s *Scheduler) isReady(block *blocks.Block) bool {
	ready := true
	block.ForEachParent(func(parent iotago.Parent) {
		if parentBlock, parentExists := s.blockCache.Block(parent.ID); !parentExists || !s.isEligible(parentBlock) {
			// if parents are evicted and orphaned (not root blocks), or have not been received yet they will not exist.
			// if parents are evicted, they will be returned as root blocks with scheduled==true here.
			ready = false
			return
		}
	})

	return ready
}

// tryReady tries to set the given block as ready.
func (s *Scheduler) tryReady(block *blocks.Block) {
	if s.isReady(block) {
		s.ready(block)
	}
}

func (s *Scheduler) ready(block *blocks.Block) {
	s.buffer.Ready(block)
}

// updateChildrenWithLocking locks the buffer mutex and iterates over the direct children of the given blockID and
// tries to mark them as ready.
func (s *Scheduler) updateChildrenWithLocking(block *blocks.Block) {
	s.bufferMutex.Lock()
	defer s.bufferMutex.Unlock()

	s.updateChildrenWithoutLocking(block)
}

// updateChildrenWithoutLocking iterates over the direct children of the given blockID and
// tries to mark them as ready.
func (s *Scheduler) updateChildrenWithoutLocking(block *blocks.Block) {
	for _, childBlock := range block.Children() {
		if _, childBlockExists := s.blockCache.Block(childBlock.ID()); childBlockExists && childBlock.IsEnqueued() {
			s.tryReady(childBlock)
		}
	}
}
