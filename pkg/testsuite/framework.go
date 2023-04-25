package testsuite

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection/poa"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

const genesisSnapshot = "genesis_snapshot.bin"

type Framework struct {
	Testing *testing.T
	Network *mock.Network

	Directory *utils.Directory
	nodes     map[string]*mock.Node
	running   bool

	validators map[iotago.AccountID]int64

	ProtocolParameters iotago.ProtocolParameters

	mutex sync.RWMutex
}

func NewFramework(t *testing.T) *Framework {
	return &Framework{
		Testing:   t,
		Network:   mock.NewNetwork(),
		Directory: utils.NewDirectory(t.TempDir()),
		nodes:     make(map[string]*mock.Node),
		ProtocolParameters: iotago.ProtocolParameters{
			Version:     3,
			NetworkName: t.Name(),
			Bech32HRP:   "rms",
			MinPoWScore: 10,
			RentStructure: iotago.RentStructure{
				VByteCost:    100,
				VBFactorData: 1,
				VBFactorKey:  10,
			},
			TokenSupply:           1_000_0000,
			GenesisUnixTimestamp:  uint32(time.Now().Unix()),
			SlotDurationInSeconds: 10,
		},
	}
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

func (f *Framework) Run() {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.validators = f.createValidatorsFromNodes()
	path := f.createSnapshot()
	f.running = true

	for _, node := range f.nodes {
		node.Initialize(
			protocol.WithSnapshotPath(path),
			protocol.WithSybilProtectionProvider(
				poa.NewProvider(f.validators),
			),
			protocol.WithBaseDirectory(f.Directory.PathWithCreate(node.Name)),
		)
	}
}

func (f *Framework) createValidatorsFromNodes() map[iotago.AccountID]int64 {
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

	return validators
}

func (f *Framework) createSnapshot() string {
	path := f.Directory.Path(genesisSnapshot)
	var base = []options.Option[snapshotcreator.Options]{
		snapshotcreator.WithDatabaseVersion(protocol.DatabaseVersion),
		snapshotcreator.WithFilePath(path),
		snapshotcreator.WithProtocolParameters(f.ProtocolParameters),
		snapshotcreator.WithRootBlocks(map[iotago.BlockID]iotago.CommitmentID{
			iotago.EmptyBlockID(): iotago.NewEmptyCommitment().MustID(),
		}),
	}

	err := snapshotcreator.CreateSnapshot(base...)
	if err != nil {
		panic(fmt.Sprintf("failed to create snapshot: %s", err))
	}

	return path
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
