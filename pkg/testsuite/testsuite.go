package testsuite

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/blockissuer"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection/poa"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type TestSuite struct {
	Testing     *testing.T
	fakeTesting *testing.T
	Network     *mock.Network

	Directory *utils.Directory
	nodes     map[string]*mock.Node
	running   bool

	validators     map[iotago.AccountID]int64
	validatorsOnce sync.Once
	snapshotPath   string
	blocks         *shrinkingmap.ShrinkingMap[string, *blocks.Block]

	ProtocolParameters iotago.ProtocolParameters

	optsSnapshotOptions []options.Option[snapshotcreator.Options]
	optsWaitFor         time.Duration
	optsTick            time.Duration

	uniqueCounter        atomic.Int64
	mutex                sync.RWMutex
	TransactionFramework *TransactionFramework
}

func NewTestSuite(testingT *testing.T, opts ...options.Option[TestSuite]) *TestSuite {
	return options.Apply(&TestSuite{
		Testing:     testingT,
		fakeTesting: &testing.T{},
		Network:     mock.NewNetwork(),
		Directory:   utils.NewDirectory(testingT.TempDir()),
		nodes:       make(map[string]*mock.Node),
		blocks:      shrinkingmap.New[string, *blocks.Block](),

		ProtocolParameters: iotago.ProtocolParameters{
			Version:     3,
			NetworkName: testingT.Name(),
			Bech32HRP:   "rms",
			MinPoWScore: 0,
			RentStructure: iotago.RentStructure{
				VByteCost:    100,
				VBFactorData: 1,
				VBFactorKey:  10,
			},
			TokenSupply:           1_000_0000,
			GenesisUnixTimestamp:  uint32(time.Now().Truncate(10*time.Second).Unix() - 10*100), // start 100 slots in the past at an even number.
			SlotDurationInSeconds: 10,
		},
		optsWaitFor: durationFromEnvOrDefault(5*time.Second, "CI_UNIT_TESTS_WAIT_FOR"),
		optsTick:    durationFromEnvOrDefault(2*time.Millisecond, "CI_UNIT_TESTS_TICK"),
	}, opts, func(t *TestSuite) {
		genesisBlock := blocks.NewRootBlock(iotago.EmptyBlockID(), iotago.NewEmptyCommitment().MustID(), time.Unix(int64(t.ProtocolParameters.GenesisUnixTimestamp), 0))
		t.RegisterBlock("Genesis", genesisBlock)

		t.snapshotPath = t.Directory.Path("genesis_snapshot.bin")
		defaultSnapshotOptions := []options.Option[snapshotcreator.Options]{
			snapshotcreator.WithDatabaseVersion(protocol.DatabaseVersion),
			snapshotcreator.WithFilePath(t.snapshotPath),
			snapshotcreator.WithProtocolParameters(t.ProtocolParameters),
			snapshotcreator.WithRootBlocks(map[iotago.BlockID]iotago.CommitmentID{
				iotago.EmptyBlockID(): iotago.NewEmptyCommitment().MustID(),
			}),
		}
		t.optsSnapshotOptions = append(defaultSnapshotOptions, t.optsSnapshotOptions...)
	})
}

// Block returns the block with the given alias. Important to note that this blocks.Block is a placeholder and is
// thus not the same as the blocks.Block that is created by a node.
func (t *TestSuite) Block(alias string) *blocks.Block {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	block, exist := t.blocks.Get(alias)
	if !exist {
		panic(fmt.Sprintf("block %s not registered", alias))
	}

	return block
}

func (t *TestSuite) BlockID(alias string) iotago.BlockID {
	return t.Block(alias).ID()
}

func (t *TestSuite) BlockIDs(aliases ...string) []iotago.BlockID {
	return lo.Map(aliases, func(alias string) iotago.BlockID {
		return t.BlockID(alias)
	})
}

func (t *TestSuite) Blocks(aliases ...string) []*blocks.Block {
	return lo.Map(aliases, func(alias string) *blocks.Block {
		return t.Block(alias)
	})
}

func (t *TestSuite) BlocksWithPrefix(prefix string) []*blocks.Block {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	b := make([]*blocks.Block, 0)

	t.blocks.ForEach(func(alias string, block *blocks.Block) bool {
		if strings.HasPrefix(alias, prefix) {
			b = append(b, block)
		}

		return true
	})

	return b
}

func (t *TestSuite) IssueBlockAtSlot(alias string, slot iotago.SlotIndex, slotCommitment *iotago.Commitment, node *mock.Node, parents ...iotago.BlockID) *blocks.Block {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	slotTimeProvider := node.Protocol.MainEngineInstance().Storage.Settings().API().SlotTimeProvider()
	issuingTime := slotTimeProvider.StartTime(slot).Add(time.Duration(t.uniqueCounter.Add(1)))

	require.Truef(t.Testing, issuingTime.Before(time.Now()), "node: %s: issued block (%s, slot: %d) is in the current (%s, slot: %d) or future slot", node.Name, issuingTime, slot, time.Now(), slotTimeProvider.IndexFromTime(time.Now()))

	block := node.IssueBlock(context.Background(), alias, blockissuer.WithIssuingTime(issuingTime), blockissuer.WithSlotCommitment(slotCommitment), blockissuer.WithStrongParents(parents...))

	t.blocks.Set(alias, block)
	block.ID().RegisterAlias(alias)

	return block
}

func (t *TestSuite) IssueBlock(alias string, node *mock.Node, blockOpts ...options.Option[blockissuer.BlockParams]) *blocks.Block {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	block := node.IssueBlock(context.Background(), alias, blockOpts...)

	t.blocks.Set(alias, block)
	block.ID().RegisterAlias(alias)

	return block
}

func (t *TestSuite) RegisterBlock(alias string, block *blocks.Block) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.blocks.Set(alias, block)
	block.ID().RegisterAlias(alias)
}

func (t *TestSuite) CreateTransactionWithInputsAndOutputs(consumedInputs ledgerstate.Outputs, outputs iotago.Outputs[iotago.Output], signingWallets []*mock.HDWallet) *iotago.Transaction {
	if t.TransactionFramework == nil {
		panic("cannot create a transaction without running the network first")
	}

	return lo.PanicOnErr(t.TransactionFramework.CreateTransactionWithInputsAndOutputs(consumedInputs, outputs, signingWallets))
}

func (t *TestSuite) CreateTransaction(alias string, outputCount int, inputAliases ...string) *iotago.Transaction {
	if t.TransactionFramework == nil {
		panic("cannot create a transaction without running the network first")
	}

	return lo.PanicOnErr(t.TransactionFramework.CreateTransaction(alias, outputCount, inputAliases...))
}

func (t *TestSuite) Node(name string) *mock.Node {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	node, exist := t.nodes[name]
	if !exist {
		panic(fmt.Sprintf("node %s does not exist", name))
	}

	return node
}

func (t *TestSuite) Nodes(names ...string) []*mock.Node {
	if len(names) == 0 {
		t.mutex.RLock()
		defer t.mutex.RUnlock()

		nodes := make([]*mock.Node, 0, len(t.nodes))
		for _, node := range t.nodes {
			nodes = append(nodes, node)
		}

		return nodes
	}

	nodes := make([]*mock.Node, len(names))
	for i, name := range names {
		nodes[i] = t.Node(name)
	}

	return nodes
}

func (t *TestSuite) Wait(nodes ...*mock.Node) {
	for _, node := range nodes {
		node.Wait()
	}
}

func (t *TestSuite) WaitWithDelay(delay time.Duration, nodes ...*mock.Node) {
	t.Wait(nodes...)
	time.Sleep(delay)
	t.Wait(nodes...)
}

func (t *TestSuite) Shutdown() {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	for _, node := range t.nodes {
		node.Shutdown()
	}
}

func (t *TestSuite) AddValidatorNodeToPartition(name string, weight int64, partition string) *mock.Node {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if weight > 0 && t.running {
		panic(fmt.Sprintf("cannot add validator node %s to partition %s with weight %d: framework already running", name, partition, weight))
	}

	t.nodes[name] = mock.NewNode(t.Testing, t.Network, partition, name, weight)

	return t.nodes[name]
}

func (t *TestSuite) AddValidatorNode(name string, weight int64) *mock.Node {
	return t.AddValidatorNodeToPartition(name, weight, mock.NetworkMainPartition)
}

func (t *TestSuite) AddNodeToPartition(name string, partition string) *mock.Node {
	return t.AddValidatorNodeToPartition(name, 0, partition)
}

func (t *TestSuite) AddNode(name string) *mock.Node {
	return t.AddValidatorNodeToPartition(name, 0, mock.NetworkMainPartition)
}

func (t *TestSuite) RemoveNode(name string) {
	delete(t.nodes, name)
}

func (t *TestSuite) Run(nodesOptions ...map[string][]options.Option[protocol.Protocol]) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	genesisSeed := tpkg.RandEd25519Seed()

	err := snapshotcreator.CreateSnapshot(append([]options.Option[snapshotcreator.Options]{snapshotcreator.WithGenesisSeed(genesisSeed[:])}, t.optsSnapshotOptions...)...)
	if err != nil {
		panic(fmt.Sprintf("failed to create snapshot: %s", err))
	}

	for _, node := range t.nodes {
		baseOpts := []options.Option[protocol.Protocol]{
			protocol.WithSnapshotPath(t.snapshotPath),
			protocol.WithBaseDirectory(t.Directory.PathWithCreate(node.Name)),
			protocol.WithSybilProtectionProvider(
				poa.NewProvider(t.Validators()),
			),
		}
		if len(nodesOptions) == 1 {
			if opts, exists := nodesOptions[0][node.Name]; exists {
				baseOpts = append(baseOpts, opts...)
			}
		}

		node.Initialize(baseOpts...)

		if t.TransactionFramework == nil {
			t.TransactionFramework = NewTransactionFramework(node.Protocol, genesisSeed[:])
		}
	}

	t.running = true
}

func (t *TestSuite) Validators() map[iotago.AccountID]int64 {
	t.validatorsOnce.Do(func() {
		if t.running {
			panic("cannot create validators from nodes: framework already running")
		}

		validators := make(map[iotago.AccountID]int64)
		for _, node := range t.nodes {
			if node.Weight == 0 {
				continue
			}
			validators[node.AccountID] = node.Weight
		}

		t.validators = validators
	})

	return t.validators
}

func (t *TestSuite) HookLogging() {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	for _, node := range t.nodes {
		node.HookLogging()
	}
}

// Eventually asserts that given condition will be met in opts.waitFor time,
// periodically checking target function each opts.tick.
//
//	assert.Eventually(t, func() bool { return true; }, time.Second, 10*time.Millisecond)
func (t *TestSuite) Eventually(condition func() error) {
	ch := make(chan error, 1)

	timer := time.NewTimer(t.optsWaitFor)
	defer timer.Stop()

	ticker := time.NewTicker(t.optsTick)
	defer ticker.Stop()

	var lastErr error
	for tick := ticker.C; ; {
		select {
		case <-timer.C:
			require.FailNow(t.Testing, "condition never satisfied", lastErr)
		case <-tick:
			tick = nil
			go func() { ch <- condition() }()
		case lastErr = <-ch:
			// The condition is satisfied, we can exit.
			if lastErr == nil {
				return
			}
			tick = ticker.C
		}
	}
}

func mustNodes(nodes []*mock.Node) {
	if len(nodes) == 0 {
		panic("no nodes provided")
	}
}

func WithWaitFor(waitFor time.Duration) options.Option[TestSuite] {
	return func(opts *TestSuite) {
		opts.optsWaitFor = waitFor
	}
}

func WithTick(tick time.Duration) options.Option[TestSuite] {
	return func(opts *TestSuite) {
		opts.optsTick = tick
	}
}

func WithSnapshotOptions(snapshotOptions ...options.Option[snapshotcreator.Options]) options.Option[TestSuite] {
	return func(opts *TestSuite) {
		opts.optsSnapshotOptions = snapshotOptions
	}
}

func durationFromEnvOrDefault(defaultDuration time.Duration, envKey string) time.Duration {
	waitFor := os.Getenv(envKey)
	if waitFor == "" {
		return defaultDuration
	}

	d, err := time.ParseDuration(waitFor)
	if err != nil {
		panic(err)
	}

	return d
}
