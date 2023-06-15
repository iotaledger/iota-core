package mock

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/crypto/identity"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/blockissuer"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

type Node struct {
	Testing *testing.T

	Name   string
	Weight int64

	ctx       context.Context
	ctxCancel context.CancelFunc

	blockIssuer *blockissuer.BlockIssuer

	privateKey ed25519.PrivateKey
	pubKey     ed25519.PublicKey
	AccountID  iotago.AccountID
	PeerID     network.PeerID

	Endpoint *Endpoint
	Workers  *workerpool.Group

	Protocol *protocol.Protocol

	forkDetectedCount             atomic.Uint32
	candidateEngineActivatedCount atomic.Uint32
	mainEngineSwitchedCount       atomic.Uint32

	mutex          sync.RWMutex
	attachedBlocks []*blocks.Block
}

func NewNode(t *testing.T, net *Network, partition string, name string, weight int64) *Node {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}

	accountID := iotago.AccountID(*iotago.Ed25519AddressFromPubKey(pub))
	accountID.RegisterAlias(name)

	peerID := network.PeerID(pub)
	identity.RegisterIDAlias(peerID, name)

	return &Node{
		Testing: t,

		Name:       name,
		Weight:     weight,
		pubKey:     pub,
		privateKey: priv,
		AccountID:  accountID,
		PeerID:     peerID,

		Endpoint: net.Join(peerID, partition),
		Workers:  workerpool.NewGroup(name),

		attachedBlocks: make([]*blocks.Block, 0),
	}
}

func (n *Node) Initialize(opts ...options.Option[protocol.Protocol]) {
	n.Protocol = protocol.New(n.Workers.CreateGroup("Protocol"),
		n.Endpoint,
		opts...,
	)

	n.hookEvents()

	n.blockIssuer = blockissuer.New(n.Protocol, blockissuer.NewEd25519Account(n.AccountID, n.privateKey), blockissuer.WithTipSelectionTimeout(3*time.Second), blockissuer.WithTipSelectionRetryInterval(time.Millisecond*100))

	n.ctx, n.ctxCancel = context.WithCancel(context.Background())

	started := make(chan struct{}, 1)

	n.Protocol.Events.Started.Hook(func() {
		close(started)
	})

	go func() {
		defer n.ctxCancel()

		if err := n.Protocol.Run(n.ctx); err != nil {
			fmt.Printf("%s > Run finished with error: %s\n", n.Name, err.Error())
		}
	}()

	<-started
}

func (n *Node) hookEvents() {
	events := n.Protocol.Events

	events.ChainManager.ForkDetected.Hook(func(fork *chainmanager.Fork) { n.forkDetectedCount.Add(1) })

	events.CandidateEngineActivated.Hook(func(e *engine.Engine) { n.candidateEngineActivatedCount.Add(1) })

	events.MainEngineSwitched.Hook(func(e *engine.Engine) { n.mainEngineSwitchedCount.Add(1) })
}

func (n *Node) HookLogging() {
	events := n.Protocol.Events

	n.attachEngineLogs(n.Protocol.MainEngineInstance())

	events.Network.BlockReceived.Hook(func(block *model.Block, source identity.ID) {
		fmt.Printf("%s > Network.BlockReceived: from %s %s - %d\n", n.Name, source, block.ID(), block.ID().Index())
	})

	events.Network.BlockRequestReceived.Hook(func(blockID iotago.BlockID, source identity.ID) {
		fmt.Printf("%s > Network.BlockRequestReceived: from %s %s\n", n.Name, source, blockID)
	})

	events.Network.SlotCommitmentReceived.Hook(func(commitment *model.Commitment, source identity.ID) {
		fmt.Printf("%s > Network.SlotCommitmentReceived: from %s %s\n", n.Name, source, commitment.ID())
	})

	events.Network.SlotCommitmentRequestReceived.Hook(func(commitmentID iotago.CommitmentID, source identity.ID) {
		fmt.Printf("%s > Network.SlotCommitmentRequestReceived: from %s %s\n", n.Name, source, commitmentID)
	})

	events.Network.AttestationsReceived.Hook(func(commitment *model.Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier], source network.PeerID) {
		fmt.Printf("%s > Network.AttestationsReceived: from %s %s number of attestations: %d with merkleProof: %s - %s\n", n.Name, source, commitment.ID(), len(attestations), lo.PanicOnErr(json.Marshal(merkleProof)), lo.Map(attestations, func(a *iotago.Attestation) iotago.BlockID {
			return lo.PanicOnErr(a.BlockID(n.Protocol.MainEngineInstance().API().SlotTimeProvider()))
		}))
	})

	events.Network.AttestationsRequestReceived.Hook(func(id iotago.CommitmentID, source network.PeerID) {
		fmt.Printf("%s > Network.AttestationsRequestReceived: from %s %s\n", n.Name, source, id)
	})

	events.ChainManager.RequestCommitment.Hook(func(commitmentID iotago.CommitmentID) {
		fmt.Printf("%s > ChainManager.RequestCommitment: %s\n", n.Name, commitmentID)
	})

	events.ChainManager.CommitmentMissing.Hook(func(commitmentID iotago.CommitmentID) {
		fmt.Printf("%s > ChainManager.CommitmentMissing: %s\n", n.Name, commitmentID)
	})

	events.ChainManager.MissingCommitmentReceived.Hook(func(commitmentID iotago.CommitmentID) {
		fmt.Printf("%s > ChainManager.MissingCommitmentReceived: %s\n", n.Name, commitmentID)
	})

	events.ChainManager.CommitmentBelowRoot.Hook(func(commitmentID iotago.CommitmentID) {
		fmt.Printf("%s > ChainManager.CommitmentBelowRoot: %s\n", n.Name, commitmentID)
	})

	events.ChainManager.ForkDetected.Hook(func(fork *chainmanager.Fork) {
		fmt.Printf("%s > ChainManager.ForkDetected: %s\n", n.Name, fork)
	})

	events.Engine.TipManager.BlockAdded.Hook(func(tipMetadata tipmanager.TipMetadata) {
		fmt.Printf("%s > TipManager.TipAdded: %s in pool %d\n", n.Name, tipMetadata.Block().ID(), tipMetadata.TipPool())
	})

	events.CandidateEngineActivated.Hook(func(e *engine.Engine) {
		fmt.Printf("%s > CandidateEngineActivated: %s, ChainID:%s Index:%s\n", n.Name, e.Name(), e.ChainID(), e.ChainID().Index())

		n.attachEngineLogs(e)
	})

	events.MainEngineSwitched.Hook(func(e *engine.Engine) {
		fmt.Printf("%s > MainEngineSwitched: %s, ChainID:%s Index:%s\n", n.Name, e.Name(), e.ChainID(), e.ChainID().Index())
	})

	events.Network.Error.Hook(func(err error, id identity.ID) {
		fmt.Printf("%s > Network.Error: from %s %s\n", n.Name, id, err)
	})

	events.Error.Hook(func(err error) {
		fmt.Printf("%s > Protocol.Error: %s\n", n.Name, err.Error())
	})
}

func (n *Node) attachEngineLogs(instance *engine.Engine) {
	engineName := fmt.Sprintf("%s - %s", lo.Cond(n.Protocol.MainEngineInstance() != instance, "Candidate", "Main"), instance.Name()[:8])
	events := instance.Events

	events.BlockDAG.BlockAttached.Hook(func(block *blocks.Block) {
		fmt.Printf("%s > [%s] BlockDAG.BlockAttached: %s\n", n.Name, engineName, block.ID())

		n.mutex.Lock()
		defer n.mutex.Unlock()
		n.attachedBlocks = append(n.attachedBlocks, block)
	})

	events.BlockDAG.BlockSolid.Hook(func(block *blocks.Block) {
		fmt.Printf("%s > [%s] BlockDAG.BlockSolid: %s\n", n.Name, engineName, block.ID())
	})

	events.BlockDAG.BlockInvalid.Hook(func(block *blocks.Block, err error) {
		fmt.Printf("%s > [%s] BlockDAG.BlockInvalid: %s - %s\n", n.Name, engineName, block.ID(), err)
	})

	events.BlockDAG.BlockMissing.Hook(func(block *blocks.Block) {
		fmt.Printf("%s > [%s] BlockDAG.BlockMissing: %s\n", n.Name, engineName, block.ID())
	})

	events.BlockDAG.MissingBlockAttached.Hook(func(block *blocks.Block) {
		fmt.Printf("%s > [%s] BlockDAG.MissingBlockAttached: %s\n", n.Name, engineName, block.ID())
	})

	events.Booker.BlockBooked.Hook(func(block *blocks.Block) {
		fmt.Printf("%s > [%s] Booker.BlockBooked: %s\n", n.Name, engineName, block.ID())
	})

	events.Clock.AcceptedTimeUpdated.Hook(func(newTime time.Time) {
		fmt.Printf("%s > [%s] Clock.AcceptedTimeUpdated: %s [Slot %d]\n", n.Name, engineName, newTime, instance.API().SlotTimeProvider().IndexFromTime(newTime))
	})

	events.Clock.ConfirmedTimeUpdated.Hook(func(newTime time.Time) {
		fmt.Printf("%s > [%s] Clock.ConfirmedTimeUpdated: %s [Slot %d]\n", n.Name, engineName, newTime, instance.API().SlotTimeProvider().IndexFromTime(newTime))
	})

	events.Filter.BlockAllowed.Hook(func(block *model.Block) {
		fmt.Printf("%s > [%s] Filter.BlockAllowed: %s\n", n.Name, engineName, block.ID())
	})

	events.Filter.BlockFiltered.Hook(func(event *filter.BlockFilteredEvent) {
		fmt.Printf("%s > [%s] Filter.BlockFiltered: %s - %s\n", n.Name, engineName, event.Block.ID(), event.Reason.Error())
		n.Testing.Fatal("no blocks should be filtered")
	})

	events.BlockRequester.Tick.Hook(func(blockID iotago.BlockID) {
		fmt.Printf("%s > [%s] BlockRequester.Tick: %s\n", n.Name, engineName, blockID)
	})

	events.BlockProcessed.Hook(func(blockID iotago.BlockID) {
		fmt.Printf("%s > [%s] Engine.BlockProcessed: %s\n", n.Name, engineName, blockID)
	})

	events.Notarization.SlotCommitted.Hook(func(details *notarization.SlotCommittedDetails) {
		var acceptedBlocks []iotago.BlockID
		_ = details.AcceptedBlocks.Stream(func(key iotago.BlockID) bool {
			acceptedBlocks = append(acceptedBlocks, key)
			return true
		})
		fmt.Printf("%s > [%s] NotarizationManager.SlotCommitted: %s %s %s\n", n.Name, engineName, details.Commitment.ID(), details.Commitment, acceptedBlocks)
	})

	events.BlockGadget.BlockPreAccepted.Hook(func(block *blocks.Block) {
		fmt.Printf("%s > [%s] Consensus.BlockGadget.BlockPreAccepted: %s %s\n", n.Name, engineName, block.ID(), block.Block().SlotCommitment.MustID())
	})

	events.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
		fmt.Printf("%s > [%s] Consensus.BlockGadget.BlockAccepted: %s @ slot %s committing to %s\n", n.Name, engineName, block.ID(), block.ID().Index(), block.Block().SlotCommitment.MustID())
	})

	events.BlockGadget.BlockPreConfirmed.Hook(func(block *blocks.Block) {
		fmt.Printf("%s > [%s] Consensus.BlockGadget.BlockPreConfirmed: %s %s\n", n.Name, engineName, block.ID(), block.Block().SlotCommitment.MustID())
	})

	events.BlockGadget.BlockConfirmed.Hook(func(block *blocks.Block) {
		fmt.Printf("%s > [%s] Consensus.BlockGadget.BlockConfirmed: %s %s\n", n.Name, engineName, block.ID(), block.Block().SlotCommitment.MustID())
	})

	events.SlotGadget.SlotFinalized.Hook(func(slotIndex iotago.SlotIndex) {
		fmt.Printf("%s > [%s] Consensus.SlotGadget.SlotFinalized: %s\n", n.Name, engineName, slotIndex)
	})

	events.SybilProtection.OnlineCommitteeAccountAdded.Hook(func(accountID iotago.AccountID) {
		fmt.Printf("%s > [%s] SybilProtection.OnlineCommitteeAccountAdded: %s\n", n.Name, engineName, accountID)
	})

	events.SybilProtection.OnlineCommitteeAccountRemoved.Hook(func(accountID iotago.AccountID) {
		fmt.Printf("%s > [%s] SybilProtection.OnlineCommitteeAccountRemoved: %s\n", n.Name, engineName, accountID)
	})

	events.ConflictDAG.ConflictCreated.Hook(func(conflictID iotago.TransactionID) {
		fmt.Printf("%s > [%s] ConflictDAG.ConflictCreated: %s\n", n.Name, engineName, conflictID)
	})

	events.ConflictDAG.ConflictEvicted.Hook(func(conflictID iotago.TransactionID) {
		fmt.Printf("%s > [%s] ConflictDAG.ConflictEvicted: %s\n", n.Name, engineName, conflictID)
	})
	events.ConflictDAG.ConflictRejected.Hook(func(conflictID iotago.TransactionID) {
		fmt.Printf("%s > [%s] ConflictDAG.ConflictRejected: %s\n", n.Name, engineName, conflictID)
	})

	events.ConflictDAG.ConflictAccepted.Hook(func(conflictID iotago.TransactionID) {
		fmt.Printf("%s > [%s] ConflictDAG.ConflictAccepted: %s\n", n.Name, engineName, conflictID)
	})

	instance.Ledger.OnTransactionAttached(func(transactionMetadata mempool.TransactionMetadata) {
		fmt.Printf("%s > [%s] Ledger.TransactionAttached: %s\n", n.Name, engineName, transactionMetadata.ID())

		transactionMetadata.OnSolid(func() {
			fmt.Printf("%s > [%s] MemPool.TransactionSolid: %s\n", n.Name, engineName, transactionMetadata.ID())
		})

		transactionMetadata.OnExecuted(func() {
			fmt.Printf("%s > [%s] MemPool.TransactionExecuted: %s\n", n.Name, engineName, transactionMetadata.ID())
		})

		transactionMetadata.OnBooked(func() {
			fmt.Printf("%s > [%s] MemPool.TransactionBooked: %s\n", n.Name, engineName, transactionMetadata.ID())
		})

		transactionMetadata.OnConflicting(func() {
			fmt.Printf("%s > [%s] MemPool.TransactionConflicting: %s\n", n.Name, engineName, transactionMetadata.ID())
		})

		transactionMetadata.OnAccepted(func() {
			fmt.Printf("%s > [%s] MemPool.TransactionAccepted: %s\n", n.Name, engineName, transactionMetadata.ID())
		})

		transactionMetadata.OnRejected(func() {
			fmt.Printf("%s > [%s] MemPool.TransactionRejected: %s\n", n.Name, engineName, transactionMetadata.ID())
		})

		transactionMetadata.OnInvalid(func(err error) {
			fmt.Printf("%s > [%s] MemPool.TransactionInvalid(%s): %s\n", n.Name, engineName, err, transactionMetadata.ID())
		})

		transactionMetadata.OnOrphaned(func() {
			fmt.Printf("%s > [%s] MemPool.TransactionOrphaned: %s\n", n.Name, engineName, transactionMetadata.ID())
		})

		transactionMetadata.OnCommitted(func() {
			fmt.Printf("%s > [%s] MemPool.TransactionCommitted: %s\n", n.Name, engineName, transactionMetadata.ID())
		})

		transactionMetadata.OnPending(func() {
			fmt.Printf("%s > [%s] MemPool.TransactionPending: %s\n", n.Name, engineName, transactionMetadata.ID())
		})
	})
}

func (n *Node) Wait() {
	n.Workers.WaitChildren()
}

func (n *Node) Shutdown() {
	stopped := make(chan struct{}, 1)

	n.Protocol.Events.Stopped.Hook(func() {
		close(stopped)
	})

	n.ctxCancel()
	n.Workers.Shutdown()

	<-stopped
}

func (n *Node) CopyIdentityFromNode(otherNode *Node) {
	n.AccountID = otherNode.AccountID
	n.pubKey = otherNode.pubKey
	n.privateKey = otherNode.privateKey
	n.AccountID.RegisterAlias(n.Name)
}

func (n *Node) IssueBlock(ctx context.Context, alias string, opts ...options.Option[blockissuer.BlockParams]) *blocks.Block {
	modelBlock, err := n.blockIssuer.CreateBlock(ctx, opts...)
	require.NoError(n.Testing, err)

	modelBlock.ID().RegisterAlias(alias)

	require.NoError(n.Testing, n.blockIssuer.IssueBlock(modelBlock))

	fmt.Printf("Issued block: %s - slot %d - commitment %s %d - latest finalized slot %d\n", modelBlock.ID(), modelBlock.ID().Index(), modelBlock.Block().SlotCommitment.MustID(), modelBlock.Block().SlotCommitment.Index, modelBlock.Block().LatestFinalizedSlot)

	return blocks.NewBlock(modelBlock)
}

func (n *Node) IssueActivity(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()

		fmt.Println(n.Name, "> Starting activity")
		var counter int
		for {
			if ctx.Err() != nil {
				fmt.Println(n.Name, "> Stopped activity due to canceled context:", ctx.Err())
				return
			}

			n.IssueBlock(ctx, fmt.Sprintf("activity %s.%d", n.Name, counter), blockissuer.WithPayload(&iotago.TaggedData{
				Tag: []byte(fmt.Sprintf("activity %s.%d", n.Name, counter)),
			}))

			counter++
			time.Sleep(1 * time.Second)
		}

	}()
}

func (n *Node) ForkDetectedCount() int {
	return int(n.forkDetectedCount.Load())
}

func (n *Node) CandidateEngineActivatedCount() int {
	return int(n.candidateEngineActivatedCount.Load())
}

func (n *Node) MainEngineSwitchedCount() int {
	return int(n.mainEngineSwitchedCount.Load())
}

func (n *Node) AttachedBlocks() []*blocks.Block {
	n.mutex.RLock()
	defer n.mutex.RUnlock()

	return n.attachedBlocks
}
