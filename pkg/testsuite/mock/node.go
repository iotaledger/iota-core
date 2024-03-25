package mock

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/postsolidfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/presolidfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	"github.com/iotaledger/iota-core/pkg/requesthandler"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/wallet"
)

// idAliases contains a list of aliases registered for a set of IDs.
var idAliases = make(map[peer.ID]string)

// RegisterIDAlias registers an alias that will modify the String() output of the ID to show a human
// readable string instead of the base58 encoded version of itself.
func RegisterIDAlias(id peer.ID, alias string) {
	idAliases[id] = alias
}

// UnregisterIDAliases removes all aliases registered through the RegisterIDAlias function.
func UnregisterIDAliases() {
	idAliases = make(map[peer.ID]string)
}

type InvalidSignedTransactionEvent struct {
	Metadata mempool.SignedTransactionMetadata
	Error    error
}

// Nodes is a helper function that creates a slice of nodes.
func Nodes(nodes ...*Node) []*Node {
	return nodes
}

type Node struct {
	Testing *testing.T
	logger  log.Logger

	Name string

	Client         *TestSuiteClient
	isValidator    bool
	Validator      *BlockIssuer
	KeyManager     *wallet.KeyManager
	RequestHandler *requesthandler.RequestHandler

	ctx       context.Context
	ctxCancel context.CancelFunc

	PeerID                  peer.ID
	protocolParametersHash  iotago.Identifier
	highestSupportedVersion iotago.Version

	Partition string
	Endpoint  *Endpoint
	Workers   *workerpool.Group

	Protocol *protocol.Protocol

	forkDetectedCount             atomic.Uint32
	candidateEngineActivatedCount atomic.Uint32
	mainEngineSwitchedCount       atomic.Uint32

	mutex                    syncutils.RWMutex
	currentSlot              iotago.SlotIndex
	filteredBlockEvents      []*postsolidfilter.BlockFilteredEvent
	invalidTransactionEvents map[iotago.SignedTransactionID]InvalidSignedTransactionEvent
}

func NewNode(t *testing.T, parentLogger log.Logger, net *Network, partition string, name string, isValidator bool) *Node {
	t.Helper()

	keyManager := lo.PanicOnErr(wallet.NewKeyManagerFromRandom(wallet.DefaultIOTAPath))
	priv, pub := keyManager.KeyPair()

	accountID := iotago.AccountID(blake2b.Sum256(pub))
	accountID.RegisterAlias(name)

	peerID := lo.PanicOnErr(peer.IDFromPrivateKey(lo.PanicOnErr(p2pcrypto.UnmarshalEd25519PrivateKey(priv))))
	RegisterIDAlias(peerID, name)

	var validator *BlockIssuer
	if isValidator {
		validator = NewBlockIssuer(t, name, keyManager, nil, 0, accountID, isValidator)
	} else {
		validator = nil
	}

	return &Node{
		Testing: t,
		logger:  parentLogger.NewChildLogger(name),

		Name: name,

		isValidator: isValidator,
		Validator:   validator,
		KeyManager:  keyManager,

		PeerID: peerID,

		Partition: partition,
		Endpoint:  net.JoinWithEndpointID(peerID, partition),
		Workers:   workerpool.NewGroup(name),

		invalidTransactionEvents: make(map[iotago.SignedTransactionID]InvalidSignedTransactionEvent),
	}
}

func (n *Node) SetCurrentSlot(slot iotago.SlotIndex) {
	n.currentSlot = slot
}

func (n *Node) IsValidator() bool {
	return n.Validator != nil
}

func (n *Node) Initialize(failOnBlockFiltered bool, opts ...options.Option[protocol.Protocol]) {
	n.Protocol = protocol.New(
		n.logger,
		n.Workers.CreateGroup("Protocol"),
		n.Endpoint,
		opts...,
	)
	n.RequestHandler = requesthandler.New(n.Protocol)

	_, pub := n.KeyManager.KeyPair()
	accountID := iotago.AccountID(blake2b.Sum256(pub))

	n.Client = NewTestSuiteClient(n)
	if n.isValidator {
		n.Validator = NewBlockIssuer(n.Testing, n.Name, n.KeyManager, n.Client, 0, accountID, n.isValidator)
	}
	n.hookEvents()
	n.hookEngineEvents(failOnBlockFiltered)

	n.ctx, n.ctxCancel = context.WithCancel(context.Background())

	started := make(chan struct{}, 1)

	n.Protocol.InitializedEvent().OnTrigger(func() {
		close(started)
	})

	go func() {
		if err := n.Protocol.Run(n.ctx); err != nil {
			fmt.Printf("%s > Run finished with error: %s\n", n.Name, err.Error())
		}
	}()

	<-started
}

func (n *Node) hookEvents() {
	n.Protocol.Chains.HeaviestAttestedCandidate.OnUpdate(func(_ *protocol.Chain, heaviestAttestedCandidate *protocol.Chain) {
		if heaviestAttestedCandidate != nil {
			n.forkDetectedCount.Add(1)

			//nolint:revive
			heaviestAttestedCandidate.Engine.OnUpdate(func(prevEngine *engine.Engine, newEngine *engine.Engine) {
				n.candidateEngineActivatedCount.Add(1)
			})
		}
	})

	//nolint:revive
	n.Protocol.Chains.Main.OnUpdate(func(prevChain *protocol.Chain, newChain *protocol.Chain) {
		if prevChain != nil {
			n.mainEngineSwitchedCount.Add(1)
		}
	})

	n.Protocol.Events.Engine.PostSolidFilter.BlockFiltered.Hook(func(event *postsolidfilter.BlockFilteredEvent) {
		n.mutex.Lock()
		defer n.mutex.Unlock()

		n.filteredBlockEvents = append(n.filteredBlockEvents, event)
	})

	n.Protocol.Engines.Main.Get().Ledger.MemPool().OnSignedTransactionAttached(
		func(signedTransactionMetadata mempool.SignedTransactionMetadata) {
			signedTxID := signedTransactionMetadata.ID()

			signedTransactionMetadata.OnSignaturesInvalid(func(err error) {
				n.mutex.Lock()
				defer n.mutex.Unlock()

				n.invalidTransactionEvents[signedTxID] = InvalidSignedTransactionEvent{
					Metadata: signedTransactionMetadata,
					Error:    err,
				}
			})

			transactionMetadata := signedTransactionMetadata.TransactionMetadata()

			transactionMetadata.OnInvalid(func(err error) {
				n.mutex.Lock()
				defer n.mutex.Unlock()

				n.invalidTransactionEvents[signedTxID] = InvalidSignedTransactionEvent{
					Metadata: signedTransactionMetadata,
					Error:    err,
				}
			})
		})
}

func (n *Node) hookEngineEvents(failOnBlockFiltered bool) {
	n.Protocol.Chains.WithElements(func(chain *protocol.Chain) (teardown func()) {
		return chain.Engine.OnUpdate(func(_ *engine.Engine, newEngine *engine.Engine) {
			if newEngine != nil {
				n.attachEngineEvents(failOnBlockFiltered, newEngine)
			}
		})
	})
}

func (n *Node) attachEngineEvents(failOnBlockFiltered bool, instance *engine.Engine) {
	events := instance.Events

	events.PreSolidFilter.BlockPreFiltered.Hook(func(event *presolidfilter.BlockPreFilteredEvent) {
		if failOnBlockFiltered {
			n.Testing.Fatal("no blocks should be prefiltered", "block", event.Block.ID(), "err", event.Reason)
		}
	})

	events.PostSolidFilter.BlockFiltered.Hook(func(event *postsolidfilter.BlockFilteredEvent) {
		if failOnBlockFiltered {
			n.Testing.Fatal("no blocks should be filtered")
		}

		n.mutex.Lock()
		defer n.mutex.Unlock()
		n.filteredBlockEvents = append(n.filteredBlockEvents, event)
	})

	events.Notarization.SlotCommitted.Hook(func(details *notarization.SlotCommittedDetails) {
		var acceptedBlocks iotago.BlockIDs
		err := details.AcceptedBlocks.Stream(func(id iotago.BlockID) error {
			acceptedBlocks = append(acceptedBlocks, id)
			return nil
		})
		require.NoError(n.Testing, err)

		rootsStorage, err := instance.Storage.Roots(details.Commitment.ID().Slot())
		require.NoError(n.Testing, err, "roots storage for slot %d not found", details.Commitment.Slot())
		roots, exists, err := rootsStorage.Load(details.Commitment.ID())
		require.NoError(n.Testing, err)
		require.True(n.Testing, exists)

		attestationBlockIDs := make([]iotago.BlockID, 0)
		tree, err := instance.Attestations.GetMap(details.Commitment.Slot())
		if err == nil {
			//nolint:revive
			err = tree.Stream(func(key iotago.AccountID, value *iotago.Attestation) error {
				attestationBlockIDs = append(attestationBlockIDs, lo.PanicOnErr(value.BlockID()))
				return nil
			})
			require.NoError(n.Testing, err)
		}

		instance.LogTrace("NotarizationManager.SlotCommitted", "commitment", details.Commitment.ID(), "acceptedBlocks", acceptedBlocks, "roots", roots, "attestations", attestationBlockIDs)
	})
}

func (n *Node) Wait() {
	n.Workers.WaitChildren()
}

func (n *Node) Shutdown() {
	stopped := make(chan struct{}, 1)

	if n.Protocol != nil {
		n.Protocol.StoppedEvent().OnTrigger(func() {
			close(stopped)
		})
	} else {
		close(stopped)
	}

	if n.ctxCancel != nil {
		n.ctxCancel()
	}

	<-stopped
	n.logger.Shutdown()
}

func (n *Node) ProtocolParametersHash() iotago.Identifier {
	if n.protocolParametersHash == iotago.EmptyIdentifier {
		return lo.PanicOnErr(n.Protocol.LatestAPI().ProtocolParameters().Hash())
	}

	return n.protocolParametersHash
}

func (n *Node) SetProtocolParametersHash(hash iotago.Identifier) {
	n.protocolParametersHash = hash
}

func (n *Node) HighestSupportedVersion() iotago.Version {
	if n.highestSupportedVersion == 0 {
		return n.Protocol.LatestAPI().Version()
	}

	return n.highestSupportedVersion
}

func (n *Node) SetHighestSupportedVersion(version iotago.Version) {
	n.highestSupportedVersion = version
}

func (n *Node) ForkDetectedCount() int {
	return int(n.forkDetectedCount.Load())
}

func (n *Node) CandidateEngineActivatedCount() int {
	return int(n.candidateEngineActivatedCount.Load())
}

func (n *Node) FilteredBlocks() []*postsolidfilter.BlockFilteredEvent {
	n.mutex.RLock()
	defer n.mutex.RUnlock()

	return n.filteredBlockEvents
}

func (n *Node) TransactionFailure(txID iotago.SignedTransactionID) (InvalidSignedTransactionEvent, bool) {
	n.mutex.RLock()
	defer n.mutex.RUnlock()
	event, exists := n.invalidTransactionEvents[txID]

	return event, exists
}

func (n *Node) MainEngineSwitchedCount() int {
	return int(n.mainEngineSwitchedCount.Load())
}

func (n *Node) IssueValidationBlock(ctx context.Context, alias string, opts ...options.Option[ValidationBlockParams]) (*blocks.Block, error) {
	if n.Validator == nil {
		panic("node is not a validator")
	}

	return n.Validator.IssueValidationBlock(ctx, alias, n, opts...)
}
