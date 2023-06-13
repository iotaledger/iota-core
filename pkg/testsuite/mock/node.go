package mock

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"sync"
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
)

type Node struct {
	Testing *testing.T

	Name   string
	Weight int64

	blockIssuer *blockissuer.BlockIssuer

	privateKey ed25519.PrivateKey
	pubKey     ed25519.PublicKey
	AccountID  iotago.AccountID
	PeerID     network.PeerID

	Endpoint *Endpoint
	Workers  *workerpool.Group

	Protocol *protocol.Protocol
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
	}
}

func (n *Node) Initialize(opts ...options.Option[protocol.Protocol]) {
	n.Protocol = protocol.New(n.Workers.CreateGroup("Protocol"),
		n.Endpoint,
		opts...,
	)
	n.blockIssuer = blockissuer.New(n.Protocol, blockissuer.NewEd25519Account(n.AccountID, n.privateKey), blockissuer.WithTipSelectionTimeout(3*time.Second), blockissuer.WithTipSelectionRetryInterval(time.Millisecond*100))

	n.Protocol.Run()
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

	// events.Network.AttestationsReceived.Hook(func(event *network.AttestationsReceivedEvent) {
	// 	fmt.Printf("%s > Network.AttestationsReceived: from %s for %s\n", n.Name, event.Source, event.Commitment.ID())
	// })
	//
	// events.Network.AttestationsRequestReceived.Hook(func(event *network.AttestationsRequestReceivedEvent) {
	// 	fmt.Printf("%s > Network.AttestationsRequestReceived: from %s %s -> %d\n", n.Name, event.Source, event.Commitment.ID(), event.EndIndex)
	// })
	//

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
		fmt.Printf("%s > [%s] Clock.AcceptedTimeUpdated: %s\n", n.Name, engineName, newTime)
	})

	events.Clock.ConfirmedTimeUpdated.Hook(func(newTime time.Time) {
		fmt.Printf("%s > [%s] Clock.ConfirmedTimeUpdated: %s\n", n.Name, engineName, newTime)
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
		fmt.Printf("%s > [%s] NotarizationManager.SlotCommitted: %s %s\n", n.Name, engineName, details.Commitment.ID(), details.Commitment)
	})

	events.BlockGadget.BlockPreAccepted.Hook(func(block *blocks.Block) {
		fmt.Printf("%s > [%s] Consensus.BlockGadget.BlockPreAccepted: %s %s\n", n.Name, engineName, block.ID(), block.Block().SlotCommitment.MustID())
	})

	events.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
		fmt.Printf("%s > [%s] Consensus.BlockGadget.BlockAccepted: %s %s\n", n.Name, engineName, block.ID(), block.Block().SlotCommitment.MustID())
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

	instance.Events.ConflictDAG.ConflictCreated.Hook(func(conflictID iotago.TransactionID) {
		fmt.Printf("%s > [%s] ConflictDAG.ConflictCreated: %s\n", n.Name, engineName, conflictID)
	})

	instance.Events.ConflictDAG.ConflictEvicted.Hook(func(conflictID iotago.TransactionID) {
		fmt.Printf("%s > [%s] ConflictDAG.ConflictEvicted: %s\n", n.Name, engineName, conflictID)
	})
	instance.Events.ConflictDAG.ConflictRejected.Hook(func(conflictID iotago.TransactionID) {
		fmt.Printf("%s > [%s] ConflictDAG.ConflictRejected: %s\n", n.Name, engineName, conflictID)
	})

	instance.Events.ConflictDAG.ConflictAccepted.Hook(func(conflictID iotago.TransactionID) {
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
	n.Protocol.Shutdown()
	n.Workers.Shutdown()
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

func (n *Node) IssueActivity(ctx context.Context, duration time.Duration, wg *sync.WaitGroup) {
	go func() {
		defer wg.Done()

		start := time.Now()
		fmt.Println(n.Name, "> Starting activity")
		var counter int
		for {
			if ctx.Err() != nil {
				fmt.Println(n.Name, "> Stopped activity due to canceled context")
				return
			}

			n.IssueBlock(ctx, fmt.Sprintf("activity %s.%d", n.Name, counter), blockissuer.WithPayload(&iotago.TaggedData{
				Tag: []byte(fmt.Sprintf("activity %s.%d", n.Name, counter)),
			}))

			counter++
			time.Sleep(1 * time.Second)
			if duration > 0 && time.Since(start) > duration {
				fmt.Println(n.Name, "> Stopped activity after", time.Since(start))
				return
			}
		}

	}()
}
