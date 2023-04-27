package testsuite

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

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

type Framework struct {
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

	mutex sync.RWMutex
}

func NewFramework(t *testing.T, snapshotOptions ...options.Option[snapshotcreator.Options]) *Framework {
	f := &Framework{
		Testing:   t,
		Network:   mock.NewNetwork(),
		Directory: utils.NewDirectory(t.TempDir()),
		nodes:     make(map[string]*mock.Node),
		blocks:    shrinkingmap.New[string, *model.Block](),
		ProtocolParameters: iotago.ProtocolParameters{
			Version:     3,
			NetworkName: t.Name(),
			Bech32HRP:   "rms",
			MinPoWScore: 0,
			RentStructure: iotago.RentStructure{
				VByteCost:    100,
				VBFactorData: 1,
				VBFactorKey:  10,
			},
			TokenSupply:           1_000_0000,
			GenesisUnixTimestamp:  uint32(time.Now().Unix() - 10*10),
			SlotDurationInSeconds: 10,
		},
	}

	f.snapshotPath = f.Directory.Path("genesis_snapshot.bin")
	var defaultSnapshotOptions = []options.Option[snapshotcreator.Options]{
		snapshotcreator.WithDatabaseVersion(protocol.DatabaseVersion),
		snapshotcreator.WithFilePath(f.snapshotPath),
		snapshotcreator.WithProtocolParameters(f.ProtocolParameters),
		snapshotcreator.WithRootBlocks(map[iotago.BlockID]iotago.CommitmentID{
			iotago.EmptyBlockID(): iotago.NewEmptyCommitment().MustID(),
		}),
	}
	f.optsSnapshotOptions = append(defaultSnapshotOptions, snapshotOptions...)

	return f
}

func (f *Framework) Block(alias string) *model.Block {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	block, exist := f.blocks.Get(alias)
	if !exist {
		panic(fmt.Sprintf("block %s not registered", alias))
	}

	return block
}

func (f *Framework) BlockID(alias string) iotago.BlockID {
	return f.Block(alias).ID()
}

func (f *Framework) BlockIDs(aliases ...string) []iotago.BlockID {
	return lo.Map(aliases, func(alias string) iotago.BlockID {
		return f.BlockID(alias)
	})
}

func (f *Framework) Blocks(aliases ...string) []*model.Block {
	return lo.Map(aliases, func(alias string) *model.Block {
		return f.Block(alias)
	})
}

func (f *Framework) BlocksByGroup(group string) []*model.Block {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	blocks := make([]*model.Block, 0)

	f.blocks.ForEach(func(alias string, block *model.Block) bool {
		if strings.HasPrefix(alias, group) {
			blocks = append(blocks, block)
		}
		return true
	})

	return blocks
}

func (f *Framework) IssueBlockAtSlot(alias string, slot iotago.SlotIndex, node *mock.Node, parents ...iotago.BlockID) *model.Block {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	block := node.IssueBlockAtSlot(alias, slot, parents...)

	f.blocks.Set(alias, block)
	return block
}

func (f *Framework) RegisterBlock(alias string, block *model.Block) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.blocks.Set(alias, block)
	block.ID().RegisterAlias(alias)
}

func (f *Framework) Node(name string) *mock.Node {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	node, exist := f.nodes[name]
	if !exist {
		panic(fmt.Sprintf("node %s does not exist", name))
	}

	return node
}

func (f *Framework) Nodes(names ...string) []*mock.Node {
	if len(names) == 0 {
		f.mutex.RLock()
		defer f.mutex.RUnlock()

		nodes := make([]*mock.Node, 0, len(f.nodes))
		for _, node := range f.nodes {
			nodes = append(nodes, node)
		}

		return nodes
	}

	nodes := make([]*mock.Node, len(names))
	for i, name := range names {
		nodes[i] = f.Node(name)
	}

	return nodes
}

func (f *Framework) Wait(nodes ...*mock.Node) {
	for _, node := range nodes {
		node.Wait()
	}
}

func (f *Framework) WaitWithDelay(delay time.Duration, nodes ...*mock.Node) {
	f.Wait(nodes...)
	time.Sleep(delay)
	f.Wait(nodes...)
}

func (f *Framework) Shutdown() {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	for _, node := range f.nodes {
		node.Shutdown()
	}
}

func (f *Framework) AddValidatorNodeToPartition(name string, weight int64, partition string, opts ...options.Option[protocol.Protocol]) *mock.Node {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if weight > 0 && f.running {
		panic(fmt.Sprintf("cannot add validator node %s to partition %s with weight %d: framework already running", name, partition, weight))
	}

	f.nodes[name] = mock.NewNode(f.Testing, f.Network, partition, name, weight, opts...)
	return f.nodes[name]
}

func (f *Framework) AddValidatorNode(name string, weight int64, opts ...options.Option[protocol.Protocol]) *mock.Node {
	return f.AddValidatorNodeToPartition(name, weight, mock.NetworkMainPartition, opts...)
}

func (f *Framework) AddNodeToPartition(name string, partition string, opts ...options.Option[protocol.Protocol]) *mock.Node {
	return f.AddValidatorNodeToPartition(name, 0, partition, opts...)
}

func (f *Framework) AddNode(name string, opts ...options.Option[protocol.Protocol]) *mock.Node {
	return f.AddValidatorNodeToPartition(name, 0, mock.NetworkMainPartition, opts...)
}

func (f *Framework) Run(nodesOptions ...map[string][]options.Option[protocol.Protocol]) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	err := snapshotcreator.CreateSnapshot(f.optsSnapshotOptions...)
	if err != nil {
		panic(fmt.Sprintf("failed to create snapshot: %s", err))
	}

	for _, node := range f.nodes {
		baseOpts := []options.Option[protocol.Protocol]{
			protocol.WithSnapshotPath(f.snapshotPath),
			protocol.WithBaseDirectory(f.Directory.PathWithCreate(node.Name)),
			protocol.WithSybilProtectionProvider(
				poa.NewProvider(f.Validators()),
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

	f.running = true
}

func (f *Framework) Validators() map[iotago.AccountID]int64 {
	f.validatorsOnce.Do(func() {
		if f.running {
			panic("cannot create validators from nodes: framework already running")
		}

		validators := make(map[iotago.AccountID]int64)
		for _, node := range f.nodes {
			if node.Weight == 0 {
				continue
			}
			validators[node.AccountID] = node.Weight
		}

		f.validators = validators
	})

	return f.validators
}

func (f *Framework) HookLogging() {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	for _, node := range f.nodes {
		node.HookLogging()
	}
}

func mustNodes(nodes []*mock.Node) {
	if len(nodes) == 0 {
		panic("no nodes provided")
	}
}
