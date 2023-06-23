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

	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/ds/orderedmap"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/blockfactory"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection/poa"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

const MinIssuerAccountDeposit = uint64(84400)

type TestSuite struct {
	Testing     *testing.T
	fakeTesting *testing.T
	Network     *mock.Network

	Directory *utils.Directory
	nodes     *orderedmap.OrderedMap[string, *mock.Node]
	running   bool

	validators     []iotago.AccountID
	validatorsOnce sync.Once
	snapshotPath   string
	blocks         *shrinkingmap.ShrinkingMap[string, *blocks.Block]

	ProtocolParameters iotago.ProtocolParameters

	optsGenesisTimestampOffset uint32
	optsAccounts               []snapshotcreator.AccountDetails
	optsSnapshotOptions        []options.Option[snapshotcreator.Options]
	optsWaitFor                time.Duration
	optsTick                   time.Duration

	uniqueCounter        atomic.Int64
	mutex                sync.RWMutex
	TransactionFramework *TransactionFramework
	genesisSeed          [32]byte
}

func NewTestSuite(testingT *testing.T, opts ...options.Option[TestSuite]) *TestSuite {
	return options.Apply(&TestSuite{
		Testing:     testingT,
		fakeTesting: &testing.T{},
		genesisSeed: tpkg.RandEd25519Seed(),
		Network:     mock.NewNetwork(),
		Directory:   utils.NewDirectory(testingT.TempDir()),
		nodes:       orderedmap.New[string, *mock.Node](),
		blocks:      shrinkingmap.New[string, *blocks.Block](),

		optsWaitFor:                DurationFromEnvOrDefault(5*time.Second, "CI_UNIT_TESTS_WAIT_FOR"),
		optsTick:                   DurationFromEnvOrDefault(2*time.Millisecond, "CI_UNIT_TESTS_TICK"),
		optsGenesisTimestampOffset: 0,
	}, opts, func(t *TestSuite) {
		fmt.Println("Setup TestSuite -", testingT.Name())
		t.ProtocolParameters = iotago.ProtocolParameters{
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
			GenesisUnixTimestamp:  uint32(time.Now().Truncate(10*time.Second).Unix()) - t.optsGenesisTimestampOffset,
			SlotDurationInSeconds: 10,
			EpochDurationInSlots:  8192,
			MaxCommittableAge:     10,
			ProtocolVersions: []iotago.ProtocolVersion{
				{Version: 3, StartEpoch: 0},
				{Version: 4, StartEpoch: 1},
			},
		}

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

func (t *TestSuite) AccountOutput(alias string) *utxoledger.Output {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	output, exist := t.TransactionFramework.states[alias]
	if !exist {
		panic(fmt.Sprintf("account %s not registered", alias))
	}

	if _, ok := output.Output().(*iotago.AccountOutput); !ok {
		panic(fmt.Sprintf("output %s is not an account", alias))
	}

	return output
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

	slotTimeProvider := node.Protocol.MainEngineInstance().Storage.Settings().API().TimeProvider()
	issuingTime := slotTimeProvider.SlotStartTime(slot).Add(time.Duration(t.uniqueCounter.Add(1)))

	require.Truef(t.Testing, issuingTime.Before(time.Now()), "node: %s: issued block (%s, slot: %d) is in the current (%s, slot: %d) or future slot", node.Name, issuingTime, slot, time.Now(), slotTimeProvider.SlotIndexFromTime(time.Now()))

	block := node.IssueBlock(context.Background(), alias, blockfactory.WithIssuingTime(issuingTime), blockfactory.WithSlotCommitment(slotCommitment), blockfactory.WithStrongParents(parents...))

	t.blocks.Set(alias, block)
	block.ID().RegisterAlias(alias)

	return block
}

func (t *TestSuite) IssueBlockAtSlotWithOptions(alias string, slot iotago.SlotIndex, slotCommitment *iotago.Commitment, node *mock.Node, blockOpts ...options.Option[blockfactory.BlockParams]) *blocks.Block {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	slotTimeProvider := node.Protocol.MainEngineInstance().Storage.Settings().API().TimeProvider()
	issuingTime := slotTimeProvider.SlotStartTime(slot).Add(time.Duration(t.uniqueCounter.Add(1)))

	require.Truef(t.Testing, issuingTime.Before(time.Now()), "node: %s: issued block (%s, slot: %d) is in the current (%s, slot: %d) or future slot", node.Name, issuingTime, slot, time.Now(), slotTimeProvider.SlotIndexFromTime(time.Now()))

	block := node.IssueBlock(context.Background(), alias, append(blockOpts, blockfactory.WithIssuingTime(issuingTime), blockfactory.WithSlotCommitment(slotCommitment))...)

	t.registerBlock(alias, block)

	return block
}

func (t *TestSuite) IssueBlock(alias string, node *mock.Node, blockOpts ...options.Option[blockfactory.BlockParams]) *blocks.Block {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	block := node.IssueBlock(context.Background(), alias, blockOpts...)

	t.registerBlock(alias, block)

	return block
}

func (t *TestSuite) RegisterBlock(alias string, block *blocks.Block) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.registerBlock(alias, block)
}

func (t *TestSuite) registerBlock(alias string, block *blocks.Block) {
	t.blocks.Set(alias, block)
	block.ID().RegisterAlias(alias)
}

func (t *TestSuite) CreateBlock(alias string, node *mock.Node, blockOpts ...options.Option[blockfactory.BlockParams]) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	block := node.CreateBlock(context.Background(), alias, blockOpts...)

	t.registerBlock(alias, block)
}

func (t *TestSuite) Node(name string) *mock.Node {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	node, exist := t.nodes.Get(name)
	if !exist {
		panic(fmt.Sprintf("node %s does not exist", name))
	}

	return node
}

func (t *TestSuite) Nodes(names ...string) []*mock.Node {
	if len(names) == 0 {
		t.mutex.RLock()
		defer t.mutex.RUnlock()

		nodes := make([]*mock.Node, 0, t.nodes.Size())
		t.nodes.ForEach(func(_ string, node *mock.Node) bool {
			nodes = append(nodes, node)

			return true
		})

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

	t.nodes.ForEach(func(_ string, node *mock.Node) bool {
		node.Shutdown()
		return true
	})

	fmt.Println("======= ATTACHED BLOCKS =======")
	t.nodes.ForEach(func(_ string, node *mock.Node) bool {
		for _, block := range node.AttachedBlocks() {
			fmt.Println(node.Name, ">", block)
		}

		return true
	})
}

func (t *TestSuite) addNodeToPartition(name string, partition string, validator bool, optDeposit ...uint64) *mock.Node {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if validator && t.running {
		panic(fmt.Sprintf("cannot add validator node %s to partition %s: framework already running", name, partition))
	}

	node := mock.NewNode(t.Testing, t.Network, partition, name, validator)
	t.nodes.Set(name, node)

	deposit := MinIssuerAccountDeposit
	if len(optDeposit) > 0 {
		deposit = optDeposit[0]
	}
	if deposit > 0 {
		t.optsAccounts = append(t.optsAccounts, snapshotcreator.AccountDetails{
			Address:   iotago.Ed25519AddressFromPubKey(node.PubKey),
			Amount:    deposit,
			IssuerKey: node.PubKey,
		})
	}

	return node
}

func (t *TestSuite) AddValidatorNodeToPartition(name string, partition string, optDeposit ...uint64) *mock.Node {
	return t.addNodeToPartition(name, partition, true, optDeposit...)
}

func (t *TestSuite) AddValidatorNode(name string, optDeposit ...uint64) *mock.Node {
	return t.addNodeToPartition(name, mock.NetworkMainPartition, true, optDeposit...)
}

func (t *TestSuite) AddNodeToPartition(name string, partition string, optDeposit ...uint64) *mock.Node {
	return t.addNodeToPartition(name, partition, false, optDeposit...)
}

func (t *TestSuite) AddNode(name string, optDeposit ...uint64) *mock.Node {
	return t.addNodeToPartition(name, mock.NetworkMainPartition, false, optDeposit...)
}

func (t *TestSuite) RemoveNode(name string) {
	t.nodes.Delete(name)
}

func (t *TestSuite) Run(nodesOptions ...map[string][]options.Option[protocol.Protocol]) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Create accounts for any block issuer nodes added before starting the network.
	if t.optsAccounts != nil {
		wallet := mock.NewHDWallet("genesis", t.genesisSeed[:], 0)
		t.optsSnapshotOptions = append(t.optsSnapshotOptions, snapshotcreator.WithAccounts(lo.Map(t.optsAccounts, func(accountDetails snapshotcreator.AccountDetails) snapshotcreator.AccountDetails {
			if accountDetails.Address == nil {
				accountDetails.Address = wallet.Address()
			}

			return accountDetails
		})...))
	}
	// TODO: what if someone passes custom GenesisSeed? We set the random one anyway in the transaction framework.
	err := snapshotcreator.CreateSnapshot(append([]options.Option[snapshotcreator.Options]{snapshotcreator.WithGenesisSeed(t.genesisSeed[:])}, t.optsSnapshotOptions...)...)
	if err != nil {
		panic(fmt.Sprintf("failed to create snapshot: %s", err))
	}

	validators := t.Validators()
	t.nodes.ForEach(func(_ string, node *mock.Node) bool {
		baseOpts := []options.Option[protocol.Protocol]{
			protocol.WithSnapshotPath(t.snapshotPath),
			protocol.WithBaseDirectory(t.Directory.PathWithCreate(node.Name)),
			protocol.WithSybilProtectionProvider(
				poa.NewProvider(validators),
			),
		}
		if len(nodesOptions) == 1 {
			if opts, exists := nodesOptions[0][node.Name]; exists {
				baseOpts = append(baseOpts, opts...)
			}
		}

		node.Initialize(baseOpts...)

		if t.TransactionFramework == nil {
			t.TransactionFramework = NewTransactionFramework(node.Protocol, t.genesisSeed[:], t.optsAccounts...)
		}

		return true
	})

	t.running = true
}

func (t *TestSuite) Validators() []iotago.AccountID {
	t.validatorsOnce.Do(func() {
		if t.running {
			panic("cannot create validators from nodes: framework already running")
		}

		var validators = []iotago.AccountID{}
		var seat account.SeatIndex
		t.nodes.ForEach(func(_ string, node *mock.Node) bool {
			if node.Validator {
				node.ValidatorSeat = seat
				validators = append(validators, node.AccountID)
				seat++
			}

			return true
		})

		t.validators = validators
	})

	return t.validators
}

func (t *TestSuite) HookLogging() {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	t.nodes.ForEach(func(_ string, node *mock.Node) bool {
		node.HookLogging()
		return true
	})
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

func WithAccounts(accounts ...snapshotcreator.AccountDetails) options.Option[TestSuite] {
	return func(opts *TestSuite) {
		opts.optsAccounts = append(opts.optsAccounts, accounts...)
	}
}

func WithSnapshotOptions(snapshotOptions ...options.Option[snapshotcreator.Options]) options.Option[TestSuite] {
	return func(opts *TestSuite) {
		opts.optsSnapshotOptions = snapshotOptions
	}
}

func WithGenesisTimestampOffset(offset uint32) options.Option[TestSuite] {
	return func(opts *TestSuite) {
		opts.optsGenesisTimestampOffset = offset
	}
}

func DurationFromEnvOrDefault(defaultDuration time.Duration, envKey string) time.Duration {
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
