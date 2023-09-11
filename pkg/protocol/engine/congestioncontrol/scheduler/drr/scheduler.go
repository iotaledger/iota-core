package drr

import (
	"math"
	"time"

	"github.com/iotaledger/hive.go/core/safemath"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/congestioncontrol/scheduler"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type Deficit int64

type SubSlotIndex int

type Scheduler struct {
	events *scheduler.Events

	quantumFunc func(iotago.AccountID, iotago.SlotIndex) (Deficit, error)

	latestCommittedSlot func() iotago.SlotIndex

	apiProvider api.Provider

	seatManager seatmanager.SeatManager

	basicBuffer     *BufferQueue
	validatorBuffer *ValidatorBuffer
	bufferMutex     syncutils.RWMutex

	deficits *shrinkingmap.ShrinkingMap[iotago.AccountID, Deficit]

	shutdownSignal chan struct{}

	blockCache *blocks.Blocks

	errorHandler func(error)

	module.Module
}

func NewProvider(opts ...options.Option[Scheduler]) module.Provider[*engine.Engine, scheduler.Scheduler] {
	return module.Provide(func(e *engine.Engine) scheduler.Scheduler {
		s := New(e, opts...)
		s.errorHandler = e.ErrorHandler("scheduler")
		s.basicBuffer = NewBufferQueue()

		e.HookConstructed(func() {
			s.latestCommittedSlot = func() iotago.SlotIndex {
				return e.Storage.Settings().LatestCommitment().Index()
			}
			s.blockCache = e.BlockCache
			e.Events.Scheduler.LinkTo(s.events)
			e.SybilProtection.HookInitialized(func() {
				s.seatManager = e.SybilProtection.SeatManager()
			})
			e.Events.Notarization.LatestCommitmentUpdated.Hook(func(commitment *model.Commitment) {
				// when the last slot of an epoch is committed, remove the queues of validators that are no longer in the committee.
				if e.CurrentAPI().TimeProvider().SlotsBeforeNextEpoch(commitment.Index()) == 0 {
					s.bufferMutex.Lock()
					defer s.bufferMutex.Unlock()

					s.validatorBuffer.buffer.ForEach(func(accountID iotago.AccountID, validatorQueue *ValidatorQueue) bool {
						if !s.seatManager.Committee(commitment.Index() + 1).HasAccount(accountID) {
							s.shutdownValidatorQueue(validatorQueue)
							s.validatorBuffer.Delete(accountID)
						}

						return true
					})
				}
			})
			e.Ledger.HookInitialized(func() {
				// quantum retrieve function gets the account's Mana and returns the quantum for that account
				s.quantumFunc = func(accountID iotago.AccountID, manaSlot iotago.SlotIndex) (Deficit, error) {
					mana, err := e.Ledger.ManaManager().GetManaOnAccount(accountID, manaSlot)
					if err != nil {
						return 0, err
					}

					minMana := s.apiProvider.CurrentAPI().ProtocolParameters().CongestionControlParameters().MinMana
					if mana < minMana {
						return 0, ierrors.Errorf("account %s has insufficient Mana for block to be scheduled: account Mana %d, min Mana %d", accountID, mana, minMana)
					}

					return Deficit(mana / minMana), nil
				}
			})
			s.TriggerConstructed()
			e.Events.Booker.BlockBooked.Hook(func(block *blocks.Block) {
				s.AddBlock(block)
				s.selectBlockToScheduleWithLocking()

			})
			e.Events.Ledger.AccountCreated.Hook(func(accountID iotago.AccountID) {
				s.bufferMutex.Lock()
				defer s.bufferMutex.Unlock()

				s.createIssuer(accountID)
			})
			e.Events.Ledger.AccountDestroyed.Hook(func(accountID iotago.AccountID) {
				s.bufferMutex.Lock()
				defer s.bufferMutex.Unlock()

				s.removeIssuer(accountID, ierrors.New("account destroyed"))
			})

			e.HookInitialized(s.Start)
		})

		return s
	})
}

func New(apiProvider api.Provider, opts ...options.Option[Scheduler]) *Scheduler {
	return options.Apply(
		&Scheduler{
			events:          scheduler.NewEvents(),
			deficits:        shrinkingmap.New[iotago.AccountID, Deficit](),
			apiProvider:     apiProvider,
			validatorBuffer: NewValidatorBuffer(),
		}, opts,
	)
}

func (s *Scheduler) Shutdown() {
	s.validatorBuffer.buffer.ForEach(func(_ iotago.AccountID, validatorQueue *ValidatorQueue) bool {
		s.shutdownValidatorQueue(validatorQueue)

		return true
	})
	close(s.shutdownSignal)
	s.TriggerStopped()
}

// Start starts the scheduler.
func (s *Scheduler) Start() {
	s.shutdownSignal = make(chan struct{}, 1)
	go s.basicBlockLoop()

	s.TriggerInitialized()
}

// IssuerQueueSizeCount returns the number of blocks in the queue of the given issuer.
func (s *Scheduler) IssuerQueueBlockCount(issuerID iotago.AccountID) int {
	s.bufferMutex.RLock()
	defer s.bufferMutex.RUnlock()

	return s.basicBuffer.IssuerQueue(issuerID).Size()
}

// IssuerQueueWork returns the queue size of the given issuer in work units.
func (s *Scheduler) IssuerQueueWork(issuerID iotago.AccountID) iotago.WorkScore {
	s.bufferMutex.RLock()
	defer s.bufferMutex.RUnlock()

	return s.basicBuffer.IssuerQueue(issuerID).Work()
}

// ValidatorQueueBlockCount returns the number of validation blocks in the validator queue of the given issuer.
func (s *Scheduler) ValidatorQueueBlockCount(issuerID iotago.AccountID) int {
	s.bufferMutex.RLock()
	defer s.bufferMutex.RUnlock()

	validatorQueue, exists := s.validatorBuffer.Get(issuerID)
	if !exists {
		return 0
	}

	return validatorQueue.Size()
}

// BufferSize returns the current buffer size of the Scheduler as block count.
func (s *Scheduler) BasicBufferSize() int {
	s.bufferMutex.RLock()
	defer s.bufferMutex.RUnlock()

	return s.basicBuffer.Size()
}

func (s *Scheduler) ValidatorBufferSize() int {
	s.bufferMutex.RLock()
	defer s.bufferMutex.RUnlock()

	return s.validatorBuffer.Size()
}

// MaxBufferSize returns the max buffer size of the Scheduler as block count.
func (s *Scheduler) MaxBufferSize() int {
	return int(s.apiProvider.CurrentAPI().ProtocolParameters().CongestionControlParameters().MaxBufferSize)
}

// ReadyBlocksCount returns the number of ready blocks.
func (s *Scheduler) ReadyBlocksCount() int {
	s.bufferMutex.RLock()
	defer s.bufferMutex.RUnlock()

	return s.basicBuffer.ReadyBlocksCount()
}

func (s *Scheduler) IsBlockIssuerReady(accountID iotago.AccountID, blocks ...*blocks.Block) bool {
	s.bufferMutex.RLock()
	defer s.bufferMutex.RUnlock()

	// if the buffer is completely empty, any issuer can issue a block.
	if s.basicBuffer.Size() == 0 {
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
	deficit, exists := s.deficits.Get(accountID)
	if !exists {
		return false
	}

	return deficit >= s.deficitFromWork(work+s.basicBuffer.IssuerQueue(accountID).Work())
}

func (s *Scheduler) AddBlock(block *blocks.Block) {
	if _, isValidation := block.ValidationBlock(); isValidation {
		s.enqueueValidationBlock(block)
	} else if _, isBasic := block.BasicBlock(); isBasic {
		s.enqueueBasicBlock(block)
	}
}

func (s *Scheduler) enqueueBasicBlock(block *blocks.Block) {
	s.bufferMutex.Lock()
	defer s.bufferMutex.Unlock()

	slotIndex := s.latestCommittedSlot()

	issuerID := block.ProtocolBlock().IssuerID
	issuerQueue, err := s.basicBuffer.GetIssuerQueue(issuerID)
	if err != nil {
		// this should only ever happen if the issuer has been removed due to insufficient Mana.
		// if Mana is now sufficient again, we can add the issuer again.
		_, quantumErr := s.quantumFunc(issuerID, slotIndex)
		if quantumErr != nil {
			s.errorHandler(ierrors.Wrapf(quantumErr, "failed to retrieve quantum for issuerID %s in slot %d when adding a block", issuerID, slotIndex))
		}

		issuerQueue = s.createIssuer(issuerID)
	}

	droppedBlocks, submitted := s.basicBuffer.Submit(
		block,
		issuerQueue,
		func(issuerID iotago.AccountID) Deficit {
			quantum, quantumErr := s.quantumFunc(issuerID, slotIndex)
			if quantumErr != nil {
				s.errorHandler(ierrors.Wrapf(quantumErr, "failed to retrieve deficit for issuerID %d in slot %d when submitting a block", issuerID, slotIndex))

				return 0
			}

			return quantum
		},
		int(s.apiProvider.CurrentAPI().ProtocolParameters().CongestionControlParameters().MaxBufferSize),
	)
	// error submitting indicates that the block was already submitted so we do nothing else.
	if !submitted {
		return
	}
	for _, b := range droppedBlocks {
		b.SetDropped()
		s.events.BlockDropped.Trigger(b, ierrors.New("basic block dropped from buffer"))
	}
	if block.SetEnqueued() {
		s.events.BlockEnqueued.Trigger(block)
		s.tryReady(block)
	}
}

func (s *Scheduler) enqueueValidationBlock(block *blocks.Block) {
	s.bufferMutex.Lock()
	defer s.bufferMutex.Unlock()

	_, exists := s.validatorBuffer.Get(block.ProtocolBlock().IssuerID)
	if !exists {
		s.addValidator(block.ProtocolBlock().IssuerID)
	}
	droppedBlock, submitted := s.validatorBuffer.Submit(block, int(s.apiProvider.CurrentAPI().ProtocolParameters().CongestionControlParameters().MaxValidationBufferSize))
	if !submitted {
		return
	}
	if droppedBlock != nil {
		droppedBlock.SetDropped()
		s.events.BlockDropped.Trigger(droppedBlock, ierrors.New("validation block dropped from buffer"))
	}

	if block.SetEnqueued() {
		s.events.BlockEnqueued.Trigger(block)
		s.tryReadyValidationBlock(block)
	}
}

func (s *Scheduler) basicBlockLoop() {
	var blockToSchedule *blocks.Block
loop:
	for {
		select {
		// on close, exit the loop
		case <-s.shutdownSignal:
			break loop
		// when a block is pushed by the buffer
		case blockToSchedule = <-s.basicBuffer.blockChan:
			currentAPI := s.apiProvider.CurrentAPI()
			rate := currentAPI.ProtocolParameters().CongestionControlParameters().SchedulerRate
			if waitTime := s.basicBuffer.waitTime(float64(rate), blockToSchedule); waitTime > 0 {
				timer := time.NewTimer(waitTime)
				<-timer.C
			}
			s.basicBuffer.updateTokenBucket(float64(rate), float64(currentAPI.MaxBlockWork()))

			s.scheduleBasicBlock(blockToSchedule)
		}
	}
}

func (s *Scheduler) validatorLoop(validatorQueue *ValidatorQueue) {
	var blockToSchedule *blocks.Block
loop:
	for {
		select {
		// on close, exit the loop
		case <-validatorQueue.shutdownSignal:
			break loop
		// when a block is pushed by this validator queue.
		case blockToSchedule = <-validatorQueue.blockChan:
			currentAPI := s.apiProvider.CurrentAPI()
			validationBlocksPerSlot := float64(currentAPI.ProtocolParameters().ValidationBlocksPerSlot())
			rate := validationBlocksPerSlot / float64(currentAPI.TimeProvider().SlotDurationSeconds())
			if waitTime := validatorQueue.waitTime(rate); waitTime > 0 {
				timer := time.NewTimer(waitTime)
				<-timer.C
			}
			// allow a maximum burst of validationBlocksPerSlot by setting this as max token bucket size.
			validatorQueue.updateTokenBucket(rate, validationBlocksPerSlot)

			s.scheduleValidationBlock(blockToSchedule, validatorQueue)
		}
	}
}

func (s *Scheduler) scheduleBasicBlock(block *blocks.Block) {
	if block.SetScheduled() {
		// deduct tokens from the token bucket according to the scheduled block's work.
		s.basicBuffer.deductTokens(float64(block.WorkScore()))

		// check for another block ready to schedule
		s.updateChildrenWithLocking(block)
		s.selectBlockToScheduleWithLocking()

		s.events.BlockScheduled.Trigger(block)
	}
}

func (s *Scheduler) scheduleValidationBlock(block *blocks.Block, validatorQueue *ValidatorQueue) {
	if block.SetScheduled() {
		// deduct 1 token from the token bucket of this validator's queue.
		validatorQueue.deductTokens(1)

		// check for another block ready to schedule
		s.updateChildrenWithLocking(block)
		s.selectBlockToScheduleWithLocking()

		s.events.BlockScheduled.Trigger(block)
	}
}

func (s *Scheduler) selectBlockToScheduleWithLocking() {
	s.bufferMutex.Lock()
	defer s.bufferMutex.Unlock()

	s.validatorBuffer.buffer.ForEach(func(accountID iotago.AccountID, validatorQueue *ValidatorQueue) bool {
		if s.selectValidationBlockWithoutLocking(validatorQueue) {
			s.validatorBuffer.size--
		}

		return true
	})
	s.selectBasicBlockWithoutLocking()

}

func (s *Scheduler) selectValidationBlockWithoutLocking(validatorQueue *ValidatorQueue) bool {
	// already a block selected to be scheduled.
	if len(validatorQueue.blockChan) > 0 {
		return false
	}

	if blockToSchedule := validatorQueue.PopFront(); blockToSchedule != nil {
		validatorQueue.blockChan <- blockToSchedule

		return true
	}

	return false
}

func (s *Scheduler) selectBasicBlockWithoutLocking() {
	slotIndex := s.latestCommittedSlot()

	// already a block selected to be scheduled.
	if len(s.basicBuffer.blockChan) > 0 {
		return
	}
	start := s.basicBuffer.Current()
	// no blocks submitted
	if start == nil {
		return
	}

	rounds, schedulingIssuer := s.selectIssuer(start, slotIndex)

	// if there is no issuer with a ready block, we cannot schedule anything
	if schedulingIssuer == nil {
		return
	}

	if rounds > 0 {
		// increment every issuer's deficit for the required number of rounds
		for q := start; ; {
			issuerID := q.IssuerID()
			if err := s.incrementDeficit(issuerID, rounds, slotIndex); err != nil {
				s.errorHandler(ierrors.Wrapf(err, "failed to increment deficit for issuerID %s in slot %d", issuerID, slotIndex))
				s.removeIssuer(issuerID, err)

				q = s.basicBuffer.Current()
			} else {
				q = s.basicBuffer.Next()
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
	for q := start; q != schedulingIssuer; q = s.basicBuffer.Next() {
		issuerID := q.IssuerID()
		if err := s.incrementDeficit(issuerID, 1, slotIndex); err != nil {
			s.errorHandler(ierrors.Wrapf(err, "failed to increment deficit for issuerID %s in slot %d", issuerID, slotIndex))
			s.removeIssuer(issuerID, err)

			return
		}
	}

	// remove the block from the buffer and adjust issuer's deficit
	block := s.basicBuffer.PopFront()
	issuerID := block.ProtocolBlock().IssuerID
	err := s.updateDeficit(issuerID, -s.deficitFromWork(block.WorkScore()))

	if err != nil {
		// if something goes wrong with deficit update, drop the block instead of scheduling it.
		block.SetDropped()
		s.events.BlockDropped.Trigger(block, err)

		return
	}
	s.basicBuffer.blockChan <- block
}

func (s *Scheduler) selectIssuer(start *IssuerQueue, slotIndex iotago.SlotIndex) (Deficit, *IssuerQueue) {
	rounds := Deficit(math.MaxInt64)
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

				s.basicBuffer.PopFront()

				block = q.Front()

				continue
			}

			issuerID := block.ProtocolBlock().IssuerID

			// compute how often the deficit needs to be incremented until the block can be scheduled
			deficit, exists := s.deficits.Get(issuerID)
			if !exists {
				panic("deficit not found for issuer")
			}

			remainingDeficit := s.deficitFromWork(block.WorkScore()) - deficit
			// calculate how many rounds we need to skip to accumulate enough deficit.
			quantum, err := s.quantumFunc(issuerID, slotIndex)
			if err != nil {
				s.errorHandler(ierrors.Wrapf(err, "failed to retrieve quantum for issuerID %s in slot %d during issuer selection", issuerID, slotIndex))
				// if quantum, can't be retrieved, we need to remove this issuer.
				s.removeIssuer(issuerID, err)
				issuerRemoved = true

				break
			}

			numerator, err := safemath.SafeAdd(remainingDeficit, quantum-1)
			if err != nil {
				numerator = math.MaxInt64
			}

			r, err := safemath.SafeDiv(numerator, quantum)
			if err != nil {
				panic(err)
			}

			// find the first issuer that will be allowed to schedule a block
			if r < rounds {
				rounds = r
				schedulingIssuer = q
			}

			break
		}

		if issuerRemoved {
			q = s.basicBuffer.Current()
		} else {
			q = s.basicBuffer.Next()
		}
		if q == start || q == nil {
			break
		}
	}

	return rounds, schedulingIssuer
}

func (s *Scheduler) removeIssuer(issuerID iotago.AccountID, err error) {
	q := s.basicBuffer.IssuerQueue(issuerID)
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

	s.deficits.Delete(issuerID)

	s.basicBuffer.RemoveIssuer(issuerID)
}

func (s *Scheduler) createIssuer(accountID iotago.AccountID) *IssuerQueue {
	issuerQueue := s.basicBuffer.CreateIssuerQueue(accountID)
	s.deficits.Set(accountID, 0)

	return issuerQueue
}

func (s *Scheduler) updateDeficit(accountID iotago.AccountID, delta Deficit) error {
	var updateErr error
	s.deficits.Compute(accountID, func(currentValue Deficit, exists bool) Deficit {
		if !exists {
			updateErr = ierrors.Errorf("could not get deficit for issuer %s", accountID)
			return 0
		}
		newDeficit, err := safemath.SafeAdd(currentValue, delta)
		if err != nil {
			// It can only overflow. We never allow the value to go below 0, so underflow is impossible.
			return s.maxDeficit()
		}

		// If the new deficit is negative, it could only be a result of subtraction and an error should be returned.
		if newDeficit < 0 {
			updateErr = ierrors.Errorf("deficit for issuer %s decreased below zero", accountID)
			return 0
		}

		return lo.Min(newDeficit, s.maxDeficit())
	})

	if updateErr != nil {
		s.removeIssuer(accountID, updateErr)

		return updateErr
	}

	return nil
}

func (s *Scheduler) incrementDeficit(issuerID iotago.AccountID, rounds Deficit, slotIndex iotago.SlotIndex) error {
	quantum, err := s.quantumFunc(issuerID, slotIndex)
	if err != nil {
		return err
	}

	delta, err := safemath.SafeMul(quantum, rounds)
	if err != nil {
		// overflow, set to max deficit
		delta = s.maxDeficit()
	}

	return s.updateDeficit(issuerID, delta)
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

// tryReadyValidator tries to set the given validation block as ready.
func (s *Scheduler) tryReadyValidationBlock(block *blocks.Block) {
	if s.isReady(block) {
		s.readyValidationBlock(block)
	}
}

func (s *Scheduler) ready(block *blocks.Block) {
	s.basicBuffer.Ready(block)
}

func (s *Scheduler) readyValidationBlock(block *blocks.Block) {
	if validatorQueue, exists := s.validatorBuffer.Get(block.ProtocolBlock().IssuerID); exists {
		validatorQueue.Ready(block)
	}
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
			if _, isBasic := childBlock.BasicBlock(); isBasic {
				s.tryReady(childBlock)
			} else if _, isValidation := childBlock.ValidationBlock(); isValidation {
				s.tryReadyValidationBlock(childBlock)
			} else {
				panic("invalid block type")
			}
		}
	}
}

func (s *Scheduler) maxDeficit() Deficit {
	return Deficit(math.MaxInt64 / 2)
}

func (s *Scheduler) deficitFromWork(work iotago.WorkScore) Deficit {
	// max workscore block should occupy the full range of the deficit
	deficitScaleFactor := s.maxDeficit() / Deficit(s.apiProvider.CurrentAPI().MaxBlockWork())
	return Deficit(work) * deficitScaleFactor
}

func (s *Scheduler) addValidator(accountID iotago.AccountID) *ValidatorQueue {
	validatorQueue := NewValidatorQueue(accountID)
	s.validatorBuffer.Set(accountID, validatorQueue)
	go s.validatorLoop(validatorQueue)

	return validatorQueue
}

func (s *Scheduler) shutdownValidatorQueue(validatorQueue *ValidatorQueue) {
	validatorQueue.shutdownSignal <- struct{}{}
}
