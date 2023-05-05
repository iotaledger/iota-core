package testsuite

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection/poa"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TestSuite struct {
	Testing *testing.T
	Network *mock.Network

	Directory *utils.Directory
	nodes     map[string]*mock.Node
	running   bool

	validators     map[iotago.AccountID]int64
	validatorsOnce sync.Once
	snapshotPath   string
	blocks         *shrinkingmap.ShrinkingMap[string, *model.Block]

	ProtocolParameters iotago.ProtocolParameters

	optsSnapshotOptions []options.Option[snapshotcreator.Options]
	optsWaitFor         time.Duration
	optsTick            time.Duration

	mutex sync.RWMutex
}

func NewTestSuite(testingT *testing.T, opts ...options.Option[TestSuite]) *TestSuite {
	return options.Apply(&TestSuite{
		Testing:   testingT,
		Network:   mock.NewNetwork(),
		Directory: utils.NewDirectory(testingT.TempDir()),
		nodes:     make(map[string]*mock.Node),
		blocks:    shrinkingmap.New[string, *model.Block](),

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
			GenesisUnixTimestamp:  uint32(time.Now().Unix() - 10*100),
			SlotDurationInSeconds: 10,
		},
		optsWaitFor: 10 * time.Second,
		optsTick:    1 * time.Millisecond,
	}, opts, func(t *TestSuite) {
		t.snapshotPath = t.Directory.Path("genesis_snapshot.bin")
		var defaultSnapshotOptions = []options.Option[snapshotcreator.Options]{
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

func (t *TestSuite) Block(alias string) *model.Block {
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

func (t *TestSuite) Blocks(aliases ...string) []*model.Block {
	return lo.Map(aliases, func(alias string) *model.Block {
		return t.Block(alias)
	})
}

func (t *TestSuite) BlocksWithPrefix(prefix string) []*model.Block {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	blocks := make([]*model.Block, 0)

	t.blocks.ForEach(func(alias string, block *model.Block) bool {
		if strings.HasPrefix(alias, prefix) {
			blocks = append(blocks, block)
		}

		return true
	})

	return blocks
}

func (t *TestSuite) IssueBlockAtSlot(alias string, slot iotago.SlotIndex, node *mock.Node, parents ...iotago.BlockID) *model.Block {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	block := node.IssueBlockAtSlot(alias, slot, parents...)

	t.blocks.Set(alias, block)

	return block
}

func (t *TestSuite) RegisterBlock(alias string, block *model.Block) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.blocks.Set(alias, block)
	block.ID().RegisterAlias(alias)
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

func (t *TestSuite) AddValidatorNodeToPartition(name string, weight int64, partition string, opts ...options.Option[protocol.Protocol]) *mock.Node {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if weight > 0 && t.running {
		panic(fmt.Sprintf("cannot add validator node %s to partition %s with weight %d: framework already running", name, partition, weight))
	}

	t.nodes[name] = mock.NewNode(t.Testing, t.Network, partition, name, weight, opts...)

	return t.nodes[name]
}

func (t *TestSuite) AddValidatorNode(name string, weight int64, opts ...options.Option[protocol.Protocol]) *mock.Node {
	return t.AddValidatorNodeToPartition(name, weight, mock.NetworkMainPartition, opts...)
}

func (t *TestSuite) AddNodeToPartition(name string, partition string, opts ...options.Option[protocol.Protocol]) *mock.Node {
	return t.AddValidatorNodeToPartition(name, 0, partition, opts...)
}

func (t *TestSuite) AddNode(name string, opts ...options.Option[protocol.Protocol]) *mock.Node {
	return t.AddValidatorNodeToPartition(name, 0, mock.NetworkMainPartition, opts...)
}

func (t *TestSuite) Run(nodesOptions ...map[string][]options.Option[protocol.Protocol]) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	err := snapshotcreator.CreateSnapshot(t.optsSnapshotOptions...)
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
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(slotnotarization.WithMinCommittableSlotAge(1)),
			),
		}
		if len(nodesOptions) == 1 {
			if opts, exists := nodesOptions[0][node.Name]; exists {
				baseOpts = append(baseOpts, opts...)
			}
		}

		node.Initialize(baseOpts...)
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

func (t *TestSuite) Eventuallyf(condition func() bool, msg string, args ...any) {
	require.Eventuallyf(t.Testing, condition, t.optsWaitFor, t.optsTick, msg, args...)
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
