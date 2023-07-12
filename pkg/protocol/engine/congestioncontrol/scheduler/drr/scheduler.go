package drr

import (
	"math"
	"sync"
	"time"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/congestioncontrol/scheduler"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Scheduler struct {
	events *scheduler.Events

	optsRate                           int // rate is specified in units of work per second
	optsMaxBufferSize                  int
	optsAcceptedBlockScheduleThreshold time.Duration
	optsMaxDeficit                     uint64
	optsMinMana                        iotago.Mana
	optsTokenBucketSize                float64

	manaRetrieveFunc func(iotago.AccountID) (iotago.Mana, error)

	buffer      *BufferQueue
	bufferMutex sync.RWMutex

	deficits *shrinkingmap.ShrinkingMap[iotago.AccountID, uint64]

	tokenBucket      float64
	lastScheduleTime time.Time

	shutdownSignal chan struct{}

	blockChan chan *blocks.Block

	blockCache *blocks.Blocks

	module.Module
}

func NewProvider(opts ...options.Option[Scheduler]) module.Provider[*engine.Engine, scheduler.Scheduler] {
	return module.Provide(func(e *engine.Engine) scheduler.Scheduler {
		s := New(opts...)
		s.buffer = NewBufferQueue(s.optsMaxBufferSize)
		e.HookConstructed(func() {
			s.blockCache = e.BlockCache
			e.Events.Scheduler.LinkTo(s.events)
			e.Ledger.HookInitialized(func() {
				s.manaRetrieveFunc = func(accountID iotago.AccountID) (iotago.Mana, error) {
					manaSlot := e.Storage.Settings().LatestCommitment().Index()
					mana, err := e.Ledger.ManaManager().GetManaOnAccount(accountID, manaSlot)
					if mana < s.optsMinMana {
						return mana, ierrors.Errorf("account %s has insufficient Mana for block to be scheduled: account Mana %d, min Mana %d", accountID, mana, s.optsMinMana)
					}

					return mana, err
				}
			})
			s.TriggerConstructed()
			e.Events.Booker.BlockBooked.Hook(func(block *blocks.Block) {
				s.AddBlock(block)
				s.selectBlockToSchedule()
			})

			e.HookInitialized(s.Start)
		})

		return s
	})
}

func New(opts ...options.Option[Scheduler]) *Scheduler {
	return options.Apply(
		&Scheduler{
			events:                             scheduler.NewEvents(),
			lastScheduleTime:                   time.Now(),
			deficits:                           shrinkingmap.New[iotago.AccountID, uint64](),
			optsRate:                           100,
			optsMaxBufferSize:                  100,
			optsAcceptedBlockScheduleThreshold: 10 * time.Second,
			optsMaxDeficit:                     100,
			optsMinMana:                        1,
			optsTokenBucketSize:                1000,
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
func (s *Scheduler) Rate() int {
	return s.optsRate
}

func (s *Scheduler) IsBlockIssuerReady(accountID iotago.AccountID, blocks ...*blocks.Block) bool {
	// if the buffer is completely empty, any issuer can issue a block.
	if s.buffer.Size() == 0 {
		return true
	}
	work := 0
	// if no specific block(s) is provided, assume max block size
	// TODO: Define max block work instead of using size
	if len(blocks) == 0 {
		work = iotago.MaxBlockSize
	}
	for _, block := range blocks {
		work += block.Work()
	}
	deficit, err := s.getDeficit(accountID)
	if err != nil {
		return false
	}

	return deficit >= uint64(work+s.buffer.IssuerQueue(accountID).Work())
}

func (s *Scheduler) getDeficit(accountID iotago.AccountID) (uint64, error) {
	d, exists := s.deficits.Get(accountID)
	if !exists {
		_, err := s.manaRetrieveFunc(accountID)
		// manaRetrieveFunc return error if mana is less than MinMana or the Mana can not be retrieved for the account.
		if err != nil {
			return 0, ierrors.Wrapf(err, "could not get deficit for issuer %s", accountID)
		}
		// load with max deficit if the issuer has Mana but has been removed from the deficits map
		return s.optsMaxDeficit, nil
	}

	return d, nil
}

func (s *Scheduler) AddBlock(block *blocks.Block) {
	s.bufferMutex.Lock()
	defer s.bufferMutex.Unlock()

	droppedBlocks, err := s.buffer.Submit(block, s.manaRetrieveFunc)
	// error submitting indicates that the block was already submitted so we do nothing else.
	if err != nil {
		return
	}
	for _, b := range droppedBlocks {
		b.SetDropped()
		s.events.BlockDropped.Trigger(block)
	}
	block.SetEnqueued()
	s.tryReady(block)
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
			tokensRequired := float64(blockToSchedule.Work()) - (s.tokenBucket + float64(s.optsRate)*time.Since(s.lastScheduleTime).Seconds())
			if tokensRequired > 0 {
				// wait until sufficient tokens in token bucket
				timer := time.NewTimer(time.Duration(tokensRequired/float64(s.optsRate)) * time.Second)
				<-timer.C
			}
			s.tokenBucket = lo.Min(
				s.optsTokenBucketSize,
				s.tokenBucket+float64(s.optsRate)*time.Since(s.lastScheduleTime).Seconds(),
			)
			s.lastScheduleTime = time.Now()
			s.scheduleBlock(blockToSchedule)
		}
	}
}

func (s *Scheduler) scheduleBlock(block *blocks.Block) {
	if block.SetScheduled() {
		// deduct tokens from the token bucket according to the scheduled block's work.
		s.tokenBucket -= float64(block.Work())
		// check for another block ready to schedule
		s.selectBlockToSchedule()
		s.updateChildren(block)
		s.events.BlockScheduled.Trigger(block)
	}
}

func (s *Scheduler) skipBlock(block *blocks.Block) {
	if block.SetSkipped() && block.SetScheduled() {
		s.selectBlockToSchedule()
		s.updateChildren(block)
		s.events.BlockScheduled.Trigger(block)
		s.events.BlockScheduled.Trigger(block)
	}
}

func (s *Scheduler) quantum(accountID iotago.AccountID) (iotago.Mana, error) {
	mana, err := s.manaRetrieveFunc(accountID)

	return mana / s.optsMinMana, err
}

func (s *Scheduler) selectBlockToSchedule() {
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
				s.buffer.RemoveIssuer(q.IssuerID())
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
			s.buffer.RemoveIssuer(q.IssuerID())
			return
		}
	}

	// remove the block from the buffer and adjust issuer's deficit
	block := s.buffer.PopFront()
	issuerID := block.ProtocolBlock().IssuerID
	err := s.updateDeficit(issuerID, int64(-block.Work()))
	if err != nil {
		// if something goes wrong with deficit update, drop the block instead of scheduling it.
		block.SetDropped()
		s.events.BlockDropped.Trigger(block)
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
			if block.IsAccepted() && time.Since(block.IssuingTime()) > s.optsAcceptedBlockScheduleThreshold {
				s.skipBlock(block)
				s.buffer.PopFront()

				block = q.Front()

				continue
			}

			issuerID := block.ProtocolBlock().IssuerID

			// compute how often the deficit needs to be incremented until the block can be scheduled
			deficit, err := s.getDeficit(issuerID)
			if err != nil {
				// no deficit exists for this issuer queue, so remove it
				s.buffer.RemoveIssuer(issuerID)
				issuerRemoved = true

				break
			}
			remainingDeficit := int64(block.Work()) - int64(deficit)
			// calculate how many rounds we need to skip to accumulate enough deficit.
			quantum, err := s.quantum(issuerID)
			if err != nil {
				// if quantum, can't be retrieved, we need to remove this issuer.
				s.buffer.RemoveIssuer(issuerID)
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

func (s *Scheduler) updateDeficit(accountID iotago.AccountID, delta int64) error {
	deficit, err := s.getDeficit(accountID)
	if err != nil {
		return ierrors.Errorf("could not get deficit for issuer %s", accountID)
	}
	if int64(deficit)+delta < 0 {
		return ierrors.Errorf("tried to decrease deficit to a negative value %d for issuer %s", int64(deficit)+delta, accountID)
	}
	s.deficits.Set(accountID, lo.Max(uint64(int64(deficit)+delta), s.optsMaxDeficit))

	return nil
}

func (s *Scheduler) incrementDeficit(issuerID iotago.AccountID, rounds int64) error {
	quantum, err := s.quantum(issuerID)
	if err != nil {
		return err
	}

	return s.updateDeficit(issuerID, int64(quantum)*rounds)
}

func (s *Scheduler) isEligible(block *blocks.Block) (eligible bool) {
	return block.IsScheduled() || block.IsAccepted()
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

// updateChildren iterates over the direct children of the given blockID and
// tries to mark them as ready.
func (s *Scheduler) updateChildren(block *blocks.Block) {
	for _, childBlock := range block.Children() {
		if childBlock, childBlockExists := s.blockCache.Block(childBlock.ID()); childBlockExists && childBlock.IsEnqueued() {
			s.tryReady(childBlock)
		}
	}
}

// region Options ////////////////////////////////////////////////////////////////////////////////////////////////////

func WithAcceptedBlockScheduleThreshold(acceptedBlockScheduleThreshold time.Duration) options.Option[Scheduler] {
	return func(s *Scheduler) {
		s.optsAcceptedBlockScheduleThreshold = acceptedBlockScheduleThreshold
	}
}

func WithMaxBufferSize(maxBufferSize int) options.Option[Scheduler] {
	return func(s *Scheduler) {
		s.optsMaxBufferSize = maxBufferSize
	}
}

func WithRate(rate int) options.Option[Scheduler] {
	return func(s *Scheduler) {
		s.optsRate = rate
	}
}

func WithMaxDeficit(maxDef int) options.Option[Scheduler] {
	return func(s *Scheduler) {
		s.optsMaxDeficit = uint64(maxDef)
	}
}

func WithMinMana(minMana int) options.Option[Scheduler] {
	return func(s *Scheduler) {
		s.optsMaxDeficit = uint64(minMana)
	}
}

func WithTokenBucketSize(tokenBucketSize float64) options.Option[Scheduler] {
	return func(s *Scheduler) {
		s.optsTokenBucketSize = tokenBucketSize
	}
}
