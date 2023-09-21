package chainmanager

import (
	"fmt"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds/walker"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

var (
	ErrCommitmentUnknown  = ierrors.New("unknown commitment")
	ErrCommitmentNotSolid = ierrors.New("commitment not solid")
)

type Manager struct {
	Events              *Events
	commitmentRequester *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]

	commitmentsByID *memstorage.IndexedStorage[iotago.SlotIndex, iotago.CommitmentID, *ChainCommitment]
	rootCommitment  *ChainCommitment

	forksByForkingPoint *memstorage.IndexedStorage[iotago.SlotIndex, iotago.CommitmentID, *Fork]

	evictionMutex syncutils.RWMutex

	optsCommitmentRequester []options.Option[eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]]

	commitmentEntityMutex *syncutils.DAGMutex[iotago.CommitmentID]
	lastEvictedSlot       *model.EvictionIndex[iotago.SlotIndex]
}

func NewManager(opts ...options.Option[Manager]) (manager *Manager) {
	return options.Apply(&Manager{
		Events: NewEvents(),

		commitmentsByID:       memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.CommitmentID, *ChainCommitment](),
		commitmentEntityMutex: syncutils.NewDAGMutex[iotago.CommitmentID](),
		forksByForkingPoint:   memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.CommitmentID, *Fork](),
		lastEvictedSlot:       model.NewEvictionIndex[iotago.SlotIndex](),
	}, opts, func(m *Manager) {
		m.commitmentRequester = eventticker.New(m.optsCommitmentRequester...)
		m.Events.CommitmentBelowRoot.Hook(m.commitmentRequester.StopTicker)

		m.Events.RequestCommitment.LinkTo(m.commitmentRequester.Events.Tick)
	})
}

func (m *Manager) Initialize(c *model.Commitment) {
	m.evictionMutex.Lock()
	defer m.evictionMutex.Unlock()

	m.rootCommitment, _ = m.getOrCreateCommitment(c.ID())
	m.rootCommitment.PublishCommitment(c)
	m.rootCommitment.SolidEvent().Trigger()
	m.rootCommitment.publishChain(NewChain(m.rootCommitment))
}

func (m *Manager) Shutdown() {
	m.commitmentRequester.Shutdown()
}

func (m *Manager) ProcessCommitmentFromSource(commitment *model.Commitment, source peer.ID) (isSolid bool, chain *Chain) {
	m.evictionMutex.RLock()
	defer m.evictionMutex.RUnlock()

	_, isSolid, chainCommitment := m.processCommitment(commitment)
	if chainCommitment == nil {
		return false, nil
	}

	m.detectForks(chainCommitment, source)

	return isSolid, chainCommitment.Chain()
}

func (m *Manager) ProcessCandidateCommitment(commitment *model.Commitment) (isSolid bool, chain *Chain) {
	m.evictionMutex.RLock()
	defer m.evictionMutex.RUnlock()

	_, isSolid, chainCommitment := m.processCommitment(commitment)
	if chainCommitment == nil {
		return false, nil
	}

	return isSolid, chainCommitment.Chain()
}

func (m *Manager) ProcessCommitment(commitment *model.Commitment) (isSolid bool, chain *Chain) {
	m.evictionMutex.RLock()
	defer m.evictionMutex.RUnlock()

	wasForked, isSolid, chainCommitment := m.processCommitment(commitment)

	if chainCommitment == nil {
		return false, nil
	}

	if wasForked {
		if err := m.switchMainChainToCommitment(chainCommitment); err != nil {
			panic(err)
		}
	}

	return isSolid, chainCommitment.Chain()
}

func (m *Manager) EvictUntil(index iotago.SlotIndex) {
	m.evictionMutex.Lock()
	defer m.evictionMutex.Unlock()

	for currentIndex := m.lastEvictedSlot.NextIndex(); currentIndex <= index; currentIndex++ {
		m.evict(currentIndex)
		m.lastEvictedSlot.MarkEvicted(currentIndex)
	}

	m.commitmentRequester.EvictUntil(index)
}

// RootCommitment returns the root commitment of the manager.
func (m *Manager) RootCommitment() (rootCommitment *ChainCommitment) {
	m.evictionMutex.RLock()
	defer m.evictionMutex.RUnlock()

	return m.rootCommitment
}

// SetRootCommitment sets the root commitment of the manager.
func (m *Manager) SetRootCommitment(commitment *model.Commitment) {
	m.evictionMutex.Lock()
	defer m.evictionMutex.Unlock()

	storage := m.commitmentsByID.Get(commitment.Index())
	if storage == nil {
		panic(fmt.Sprintf("we should always have commitment storage for confirmed index %s", commitment))
	}

	newRootCommitment, exists := storage.Get(commitment.ID())
	if !exists {
		panic(fmt.Sprint("we should always have the latest commitment ID we confirmed with", commitment))
	}

	if commitment.Index() <= m.rootCommitment.Commitment().Index() && commitment.ID() != m.rootCommitment.Commitment().ID() {
		panic(fmt.Sprintf("we should never set the root commitment to a commitment that is below the current root commitment %s - root: %s", commitment, m.rootCommitment.Commitment()))
	}

	m.rootCommitment = newRootCommitment
}

func (m *Manager) Chain(ec iotago.CommitmentID) (chain *Chain) {
	m.evictionMutex.RLock()
	defer m.evictionMutex.RUnlock()

	if commitment, exists := m.Commitment(ec); exists {
		return commitment.Chain()
	}

	return nil
}

func (m *Manager) Commitment(id iotago.CommitmentID) (commitment *ChainCommitment, exists bool) {
	storage := m.commitmentsByID.Get(id.Index())
	if storage == nil {
		return nil, false
	}

	return storage.Get(id)
}

func (m *Manager) LoadCommitmentOrRequestMissing(id iotago.CommitmentID) *ChainCommitment {
	m.evictionMutex.RLock()
	defer m.evictionMutex.RUnlock()

	chainCommitment, created := m.getOrCreateCommitment(id)
	if created {
		m.commitmentRequester.StartTicker(id)
	}

	return chainCommitment
}

func (m *Manager) Commitments(id iotago.CommitmentID, amount int) (commitments []*ChainCommitment, err error) {
	m.evictionMutex.RLock()
	defer m.evictionMutex.RUnlock()

	commitments = make([]*ChainCommitment, amount)

	for i := 0; i < amount; i++ {
		currentCommitment, _ := m.Commitment(id)
		if currentCommitment == nil {
			return nil, ierrors.Wrap(ErrCommitmentUnknown, "not all commitments in the given range are known")
		}

		commitments[i] = currentCommitment

		id = currentCommitment.Commitment().PreviousCommitmentID()
	}

	return
}

// ForkByForkingPoint returns the fork generated by a peer for the given forking point.
func (m *Manager) ForkByForkingPoint(forkingPoint iotago.CommitmentID) (fork *Fork, exists bool) {
	m.evictionMutex.RLock()
	defer m.evictionMutex.RUnlock()

	return m.forkByForkingPoint(forkingPoint)
}

func (m *Manager) forkByForkingPoint(forkingPoint iotago.CommitmentID) (fork *Fork, exists bool) {
	if indexStore := m.forksByForkingPoint.Get(forkingPoint.Index()); indexStore != nil {
		return indexStore.Get(forkingPoint)
	}

	return nil, false
}

func (m *Manager) SwitchMainChain(head iotago.CommitmentID) error {
	m.evictionMutex.RLock()
	defer m.evictionMutex.RUnlock()

	commitment, _ := m.Commitment(head)
	if commitment == nil {
		return ierrors.Wrapf(ErrCommitmentUnknown, "unknown commitment %s", head)
	}

	return m.switchMainChainToCommitment(commitment)
}

func (m *Manager) processCommitment(commitment *model.Commitment) (wasForked bool, isSolid bool, chainCommitment *ChainCommitment) {
	// Lock access to the parent commitment. We need to lock this first as we are trying to update children later within this function.
	// Failure to do so, leads to a deadlock, where a child is locked and tries to lock its parent, which is locked by the parent which tries to lock the child.
	m.commitmentEntityMutex.Lock(commitment.PreviousCommitmentID())
	defer m.commitmentEntityMutex.Unlock(commitment.PreviousCommitmentID())

	// Lock access to the chainCommitment so no children are added while we are propagating solidity
	m.commitmentEntityMutex.Lock(commitment.ID())
	defer m.commitmentEntityMutex.Unlock(commitment.ID())

	if isBelowRootCommitment, isRootCommitment := m.evaluateAgainstRootCommitment(commitment.Commitment()); isBelowRootCommitment || isRootCommitment {
		if isRootCommitment {
			chainCommitment = m.rootCommitment
		} else {
			m.Events.CommitmentBelowRoot.Trigger(commitment.ID())
		}

		return false, isRootCommitment, chainCommitment
	}

	isNew, isSolid, wasForked, chainCommitment := m.registerCommitment(commitment)
	if !isNew || chainCommitment.Chain() == nil {
		return wasForked, isSolid, chainCommitment
	}

	if mainChild := chainCommitment.mainChild(); mainChild != nil {
		for childWalker := walker.New[*ChainCommitment]().Push(chainCommitment.mainChild()); childWalker.HasNext(); {
			childWalker.PushAll(m.propagateChainToMainChild(childWalker.Next(), chainCommitment.Chain())...)
		}
	}

	if isSolid {
		if children := chainCommitment.Children(); len(children) != 0 {
			for childWalker := walker.New[*ChainCommitment]().PushAll(children...); childWalker.HasNext(); {
				childWalker.PushAll(m.propagateSolidity(childWalker.Next())...)
			}
		}
	}

	return wasForked, isSolid, chainCommitment
}

func (m *Manager) evict(index iotago.SlotIndex) {
	m.forksByForkingPoint.Evict(index)
	m.commitmentsByID.Evict(index)
}

func (m *Manager) getOrCreateCommitment(id iotago.CommitmentID) (commitment *ChainCommitment, created bool) {
	return m.commitmentsByID.Get(id.Index(), true).GetOrCreate(id, func() *ChainCommitment {
		return NewChainCommitment(id)
	})
}

func (m *Manager) evaluateAgainstRootCommitment(commitment *iotago.Commitment) (isBelow, isRootCommitment bool) {
	isBelow = commitment.Index <= m.rootCommitment.Commitment().Index()
	isRootCommitment = commitment.Equals(m.rootCommitment.Commitment().Commitment())

	return
}

func (m *Manager) detectForks(commitment *ChainCommitment, source peer.ID) {
	forkingPoint, err := m.forkingPointAgainstMainChain(commitment)
	if err != nil {
		return
	}

	if forkingPoint == nil {
		return
	}

	// Note: we rely on the fact that the block filter will not let (not yet committable) commitments through.

	forkedChainLatestCommitment := forkingPoint.Chain().LatestCommitment().Commitment()
	mainChainLatestCommitment := m.rootCommitment.Chain().LatestCommitment().Commitment()

	// Check whether the chain is claiming to be heavier than the current main chain.
	if forkedChainLatestCommitment.CumulativeWeight() <= mainChainLatestCommitment.CumulativeWeight() {
		return
	}

	var doNotTrigger bool
	fork := m.forksByForkingPoint.Get(forkingPoint.ID().Index(), true).Compute(forkingPoint.ID(), func(currentValue *Fork, exists bool) *Fork {
		if exists {
			if forkedChainLatestCommitment.Index() <= currentValue.ForkLatestCommitment.Index() {
				// Do not trigger another event for the same forking point if the latest fork commitment did not change
				doNotTrigger = true
				return currentValue
			}

			return &Fork{
				Source:               currentValue.Source,
				MainChain:            currentValue.MainChain,
				ForkedChain:          currentValue.ForkedChain,
				ForkingPoint:         currentValue.ForkingPoint,
				ForkLatestCommitment: forkedChainLatestCommitment,
			}
		}

		return &Fork{
			Source:               source,
			MainChain:            m.rootCommitment.Chain(),
			ForkedChain:          forkingPoint.Chain(),
			ForkingPoint:         forkingPoint.Commitment(),
			ForkLatestCommitment: forkedChainLatestCommitment,
		}
	})

	if !doNotTrigger {
		m.Events.ForkDetected.Trigger(fork)
	}
}

func (m *Manager) forkingPointAgainstMainChain(commitment *ChainCommitment) (*ChainCommitment, error) {
	if !commitment.SolidEvent().WasTriggered() || commitment.Chain() == nil {
		return nil, ierrors.Wrapf(ErrCommitmentNotSolid, "commitment %s is not solid", commitment)
	}

	var forkingCommitment *ChainCommitment
	// Walk all possible forks until we reach our main chain by jumping over each forking point
	for chain := commitment.Chain(); chain != m.rootCommitment.Chain(); chain = commitment.Chain() {
		forkingCommitment = chain.ForkingPoint

		if commitment, _ = m.Commitment(forkingCommitment.Commitment().PreviousCommitmentID()); commitment == nil {
			return nil, ierrors.Wrapf(ErrCommitmentUnknown, "unknown parent of solid commitment %s", forkingCommitment.Commitment().ID())
		}
	}

	return forkingCommitment, nil
}

func (m *Manager) registerCommitment(commitment *model.Commitment) (isNew bool, isSolid bool, wasForked bool, chainCommitment *ChainCommitment) {
	parentCommitment, commitmentCreated := m.getOrCreateCommitment(commitment.PreviousCommitmentID())
	if commitmentCreated {
		m.commitmentRequester.StartTicker(parentCommitment.ID())
	}

	chainCommitment, created := m.getOrCreateCommitment(commitment.ID())

	if !chainCommitment.PublishCommitment(commitment) {
		return false, chainCommitment.SolidEvent().WasTriggered(), false, chainCommitment
	}

	m.Events.CommitmentPublished.Trigger(chainCommitment)

	if !created {
		m.commitmentRequester.StopTicker(chainCommitment.ID())
	}

	isSolid, _, wasForked = m.registerChild(parentCommitment, chainCommitment)

	return true, isSolid, wasForked, chainCommitment
}

func (m *Manager) switchMainChainToCommitment(commitment *ChainCommitment) error {
	forkingPoint, err := m.forkingPointAgainstMainChain(commitment)
	if err != nil {
		return err
	}

	//  commitment is already part of the main chain
	if forkingPoint == nil {
		return nil
	}

	parentCommitment, _ := m.Commitment(forkingPoint.Commitment().PreviousCommitmentID())
	if parentCommitment == nil {
		return ierrors.Wrapf(ErrCommitmentUnknown, "unknown parent of solid commitment %s", forkingPoint.ID())
	}

	// Separate the main chain by remove it from the parent
	oldMainCommitment := parentCommitment.mainChild()

	// For each forking point coming out of the main chain we need to reorg the children
	for fp := commitment.Chain().ForkingPoint; ; {
		fpParent, _ := m.Commitment(fp.Commitment().PreviousCommitmentID())

		mainChild := fpParent.mainChild()
		newChildChain := NewChain(mainChild)

		if err := fpParent.setMainChild(fp); err != nil {
			return err
		}

		for childWalker := walker.New[*ChainCommitment]().Push(mainChild); childWalker.HasNext(); {
			childWalker.PushAll(m.propagateReplaceChainToMainChild(childWalker.Next(), newChildChain)...)
		}

		for childWalker := walker.New[*ChainCommitment]().Push(fp); childWalker.HasNext(); {
			childWalker.PushAll(m.propagateReplaceChainToMainChild(childWalker.Next(), parentCommitment.Chain())...)
		}

		if fp == forkingPoint {
			break
		}

		fp = fpParent.Chain().ForkingPoint
	}

	// Separate the old main chain by removing it from the parent
	parentCommitment.deleteChild(oldMainCommitment)

	return nil
}

func (m *Manager) registerChild(parent *ChainCommitment, child *ChainCommitment) (isSolid bool, chain *Chain, wasForked bool) {
	if isSolid, chain, wasForked = parent.registerChild(child); chain != nil {
		chain.addCommitment(child)
		child.publishChain(chain)

		if isSolid {
			child.SolidEvent().Trigger()
		}
	}

	return
}

func (m *Manager) propagateChainToMainChild(child *ChainCommitment, chain *Chain) (childrenToUpdate []*ChainCommitment) {
	m.commitmentEntityMutex.Lock(child.ID())
	defer m.commitmentEntityMutex.Unlock(child.ID())

	if !child.publishChain(chain) {
		return
	}

	chain.addCommitment(child)

	mainChild := child.mainChild()
	if mainChild == nil {
		return
	}

	return []*ChainCommitment{mainChild}
}

func (m *Manager) propagateReplaceChainToMainChild(child *ChainCommitment, chain *Chain) (childrenToUpdate []*ChainCommitment) {
	m.commitmentEntityMutex.Lock(child.ID())
	defer m.commitmentEntityMutex.Unlock(child.ID())

	child.replaceChain(chain)
	chain.addCommitment(child)

	mainChild := child.mainChild()
	if mainChild == nil {
		return
	}

	return []*ChainCommitment{mainChild}
}

func (m *Manager) propagateSolidity(child *ChainCommitment) (childrenToUpdate []*ChainCommitment) {
	m.commitmentEntityMutex.Lock(child.ID())
	defer m.commitmentEntityMutex.Unlock(child.ID())

	if child.SolidEvent().Trigger() {
		childrenToUpdate = child.Children()
	}

	return
}

func WithCommitmentRequesterOptions(opts ...options.Option[eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]]) options.Option[Manager] {
	return func(m *Manager) {
		m.optsCommitmentRequester = append(m.optsCommitmentRequester, opts...)
	}
}
