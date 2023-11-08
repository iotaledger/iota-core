package testsuite

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/orderedmap"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/pkg/testsuite/snapshotcreator"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type TestSuite struct {
	Testing     *testing.T
	fakeTesting *testing.T
	network     *mock.Network

	Directory *utils.Directory
	nodes     *orderedmap.OrderedMap[string, *mock.Node]
	wallets   *orderedmap.OrderedMap[string, *mock.Wallet]
	running   bool

	snapshotPath string
	blocks       *shrinkingmap.ShrinkingMap[string, *blocks.Block]

	API                      iotago.API
	ProtocolParameterOptions []options.Option[iotago.V3ProtocolParameters]

	optsAccounts        []snapshotcreator.AccountDetails
	optsSnapshotOptions []options.Option[snapshotcreator.Options]
	optsWaitFor         time.Duration
	optsTick            time.Duration

	uniqueBlockTimeCounter              atomic.Int64
	automaticTransactionIssuingCounters shrinkingmap.ShrinkingMap[string, int]
	mutex                               syncutils.RWMutex
	genesisKeyManager                   *mock.KeyManager
}

func NewTestSuite(testingT *testing.T, opts ...options.Option[TestSuite]) *TestSuite {
	genesisSeed := tpkg.RandEd25519Seed()
	return options.Apply(&TestSuite{
		Testing:                             testingT,
		fakeTesting:                         &testing.T{},
		genesisKeyManager:                   mock.NewKeyManager(genesisSeed[:], 0),
		network:                             mock.NewNetwork(),
		Directory:                           utils.NewDirectory(testingT.TempDir()),
		nodes:                               orderedmap.New[string, *mock.Node](),
		wallets:                             orderedmap.New[string, *mock.Wallet](),
		blocks:                              shrinkingmap.New[string, *blocks.Block](),
		automaticTransactionIssuingCounters: *shrinkingmap.New[string, int](),

		optsWaitFor: durationFromEnvOrDefault(5*time.Second, "CI_UNIT_TESTS_WAIT_FOR"),
		optsTick:    durationFromEnvOrDefault(2*time.Millisecond, "CI_UNIT_TESTS_TICK"),
	}, opts, func(t *TestSuite) {
		fmt.Println("Setup TestSuite -", testingT.Name(), " @ ", time.Now())

		defaultProtocolParameters := []options.Option[iotago.V3ProtocolParameters]{
			iotago.WithNetworkOptions(
				testingT.Name(),
				"rms",
			),
			iotago.WithSupplyOptions(
				1_000_0000,
				100,
				1,
				10,
				100,
				100,
				100,
			),
			iotago.WithRewardsOptions(8, 8, 31, 1154, 2, 1),
			iotago.WithStakingOptions(1, 100, 1),

			iotago.WithTimeProviderOptions(
				0,
				GenesisTimeWithOffsetBySlots(0, DefaultSlotDurationInSeconds),
				DefaultSlotDurationInSeconds,
				DefaultSlotsPerEpochExponent,
			),
			iotago.WithLivenessOptions(
				DefaultLivenessThresholdLowerBoundInSeconds,
				DefaultLivenessThresholdUpperBoundInSeconds,
				DefaultMinCommittableAge,
				DefaultMaxCommittableAge,
				DefaultEpochNearingThreshold,
			),
			iotago.WithCongestionControlOptions(
				DefaultMinReferenceManaCost,
				DefaultRMCIncrease,
				DefaultRMCDecrease,
				DefaultRMCIncreaseThreshold,
				DefaultRMCDecreaseThreshold,
				DefaultSchedulerRate,
				DefaultMaxBufferSize,
				DefaultMaxValBufferSize,
			),
		}

		t.ProtocolParameterOptions = append(defaultProtocolParameters, t.ProtocolParameterOptions...)
		t.API = iotago.V3API(iotago.NewV3ProtocolParameters(t.ProtocolParameterOptions...))

		genesisBlock := blocks.NewRootBlock(t.API.ProtocolParameters().GenesisBlockID(), iotago.NewEmptyCommitment(t.API).MustID(), time.Unix(t.API.ProtocolParameters().GenesisUnixTimestamp(), 0))
		t.RegisterBlock("Genesis", genesisBlock)

		t.snapshotPath = t.Directory.Path("genesis_snapshot.bin")
		defaultSnapshotOptions := []options.Option[snapshotcreator.Options]{
			snapshotcreator.WithDatabaseVersion(protocol.DatabaseVersion),
			snapshotcreator.WithFilePath(t.snapshotPath),
			snapshotcreator.WithProtocolParameters(t.API.ProtocolParameters()),
			snapshotcreator.WithRootBlocks(map[iotago.BlockID]iotago.CommitmentID{
				t.API.ProtocolParameters().GenesisBlockID(): iotago.NewEmptyCommitment(t.API).MustID(),
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

	output := t.DefaultWallet().Output(alias)

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

func (t *TestSuite) BlocksWithPrefixes(prefixes ...string) []*blocks.Block {
	b := make([]*blocks.Block, 0)

	for _, prefix := range prefixes {
		b = append(b, t.BlocksWithPrefix(prefix)...)
	}

	return b
}

func (t *TestSuite) BlocksWithSuffix(suffix string) []*blocks.Block {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	b := make([]*blocks.Block, 0)

	t.blocks.ForEach(func(alias string, block *blocks.Block) bool {
		if strings.HasSuffix(alias, suffix) {
			b = append(b, block)
		}

		return true
	})

	return b
}

func (t *TestSuite) BlocksWithSuffixes(suffixes ...string) []*blocks.Block {
	b := make([]*blocks.Block, 0)

	for _, prefix := range suffixes {
		b = append(b, t.BlocksWithSuffix(prefix)...)
	}

	return b
}

func (t *TestSuite) BlockIDsWithPrefix(prefix string) []iotago.BlockID {
	blocksWithPrefix := t.BlocksWithPrefix(prefix)

	return lo.Map(blocksWithPrefix, func(block *blocks.Block) iotago.BlockID {
		return block.ID()
	})
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

func (t *TestSuite) Wallet(name string) *mock.Wallet {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	wallet, exist := t.wallets.Get(name)
	if !exist {
		panic(fmt.Sprintf("wallet %s does not exist", name))
	}

	return wallet
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

	// fmt.Println("======= ATTACHED BLOCKS =======")
	// t.nodes.ForEach(func(_ string, node *mock.Node) bool {
	// 	for _, block := range node.AttachedBlocks() {
	// 		fmt.Println(node.Name, ">", block)
	// 	}
	//
	// 	return true
	// })
}

func (t *TestSuite) addNodeToPartition(name string, partition string, validator bool, optAmount ...iotago.BaseToken) *mock.Node {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if validator && t.running {
		panic(fmt.Sprintf("cannot add validator node %s to partition %s: framework already running", name, partition))
	}

	node := mock.NewNode(t.Testing, t.network, partition, name, validator)
	t.nodes.Set(name, node)

	amount := mock.MinValidatorAccountAmount
	if len(optAmount) > 0 {
		amount = optAmount[0]
	}
	if amount > 0 && validator {
		accountDetails := snapshotcreator.AccountDetails{
			Address:              iotago.Ed25519AddressFromPubKey(node.Validator.PublicKey),
			Amount:               amount,
			Mana:                 iotago.Mana(amount),
			IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(node.Validator.PublicKey)),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 2,
			StakedAmount:         amount,
			StakingEpochEnd:      iotago.MaxEpochIndex,
			FixedCost:            iotago.Mana(0),
			AccountID:            node.Validator.AccountID,
		}

		t.optsAccounts = append(t.optsAccounts, accountDetails)
	}

	return node
}

func (t *TestSuite) AddValidatorNodeToPartition(name string, partition string, optAmount ...iotago.BaseToken) *mock.Node {
	return t.addNodeToPartition(name, partition, true, optAmount...)
}

func (t *TestSuite) AddValidatorNode(name string, optAmount ...iotago.BaseToken) *mock.Node {
	node := t.addNodeToPartition(name, mock.NetworkMainPartition, true, optAmount...)
	// create a wallet for each validator node which uses the validator account as a block issuer
	t.addWallet(name, node, node.Validator.AccountID, node.KeyManager)

	return node
}

func (t *TestSuite) AddNodeToPartition(name string, partition string, optAmount ...iotago.BaseToken) *mock.Node {
	return t.addNodeToPartition(name, partition, false, optAmount...)
}

func (t *TestSuite) AddNode(name string, optAmount ...iotago.BaseToken) *mock.Node {
	return t.addNodeToPartition(name, mock.NetworkMainPartition, false, optAmount...)
}

func (t *TestSuite) RemoveNode(name string) {
	t.nodes.Delete(name)
}

// AddGenesisWallet adds a wallet to the test suite with a block issuer in the genesis snapshot and access to the genesis seed.
// If no block issuance credits are provided, the wallet will be assigned half of the maximum block issuance credits.
func (t *TestSuite) AddGenesisWallet(name string, node *mock.Node, blockIssuanceCredits ...iotago.BlockIssuanceCredits) *mock.Wallet {
	newWallet := t.addWallet(name, node, iotago.EmptyAccountID, t.genesisKeyManager)
	var bic iotago.BlockIssuanceCredits
	if len(blockIssuanceCredits) == 0 {
		bic = iotago.MaxBlockIssuanceCredits / 2
	} else {
		bic = blockIssuanceCredits[0]
	}

	accountDetails := snapshotcreator.AccountDetails{
		Address:              iotago.Ed25519AddressFromPubKey(newWallet.BlockIssuer.PublicKey),
		Amount:               mock.MinIssuerAccountAmount,
		Mana:                 iotago.Mana(mock.MinIssuerAccountAmount),
		IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(newWallet.BlockIssuer.PublicKey)),
		ExpirySlot:           iotago.MaxSlotIndex,
		BlockIssuanceCredits: bic,
	}

	t.optsAccounts = append(t.optsAccounts, accountDetails)

	return newWallet
}

func (t *TestSuite) addWallet(name string, node *mock.Node, accountID iotago.AccountID, keyManager *mock.KeyManager) *mock.Wallet {
	newWallet := mock.NewWallet(t.Testing, name, node, keyManager)
	newWallet.SetBlockIssuer(accountID)
	t.wallets.Set(name, newWallet)

	return newWallet
}

func (t *TestSuite) DefaultWallet() *mock.Wallet {
	defaultWallet, exists := t.wallets.Get("default")
	if !exists {
		return nil
	}

	return defaultWallet
}

func (t *TestSuite) Run(failOnBlockFiltered bool, nodesOptions ...map[string][]options.Option[protocol.Protocol]) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Create accounts for any block issuer nodes added before starting the network.
	if t.optsAccounts != nil {
		t.optsSnapshotOptions = append(t.optsSnapshotOptions, snapshotcreator.WithAccounts(lo.Map(t.optsAccounts, func(accountDetails snapshotcreator.AccountDetails) snapshotcreator.AccountDetails {
			// if no custom address is assigned to the account, assign an address generated from GenesisKeyManager
			if accountDetails.Address == nil {
				accountDetails.Address = t.genesisKeyManager.Address(iotago.AddressEd25519)
			}

			if accountDetails.AccountID.Empty() {
				blockIssuerKeyEd25519, ok := accountDetails.IssuerKey.(*iotago.Ed25519PublicKeyBlockIssuerKey)
				if !ok {
					panic("block issuer key must be of type ed25519")
				}
				ed25519PubKey := blockIssuerKeyEd25519.ToEd25519PublicKey()
				accountID := blake2b.Sum256(ed25519PubKey[:])
				accountDetails.AccountID = accountID
			}

			return accountDetails
		})...))
	}

	err := snapshotcreator.CreateSnapshot(append([]options.Option[snapshotcreator.Options]{snapshotcreator.WithGenesisKeyManager(t.genesisKeyManager)}, t.optsSnapshotOptions...)...)
	if err != nil {
		panic(fmt.Sprintf("failed to create snapshot: %s", err))
	}

	t.nodes.ForEach(func(_ string, node *mock.Node) bool {
		baseOpts := []options.Option[protocol.Protocol]{
			protocol.WithSnapshotPath(t.snapshotPath),
			protocol.WithBaseDirectory(t.Directory.PathWithCreate(node.Name)),
			protocol.WithEpochGadgetProvider(
				sybilprotectionv1.NewProvider(),
			),
		}
		if len(nodesOptions) == 1 {
			if opts, exists := nodesOptions[0][node.Name]; exists {
				baseOpts = append(baseOpts, opts...)
			}
		}

		node.Initialize(failOnBlockFiltered, baseOpts...)

		if defaultWallet := t.DefaultWallet(); defaultWallet != nil {
			if err := node.Protocol.MainEngineInstance().Ledger.ForEachUnspentOutput(func(output *utxoledger.Output) bool {
				defaultWallet.AddOutput(fmt.Sprintf("Genesis:%d", output.OutputID().Index()), output)
				return true
			}); err != nil {
				panic(err)
			}
		}

		return true
	})

	t.running = true
}

func (t *TestSuite) Validators() []*mock.Node {
	validators := make([]*mock.Node, 0)
	t.nodes.ForEach(func(_ string, node *mock.Node) bool {
		if node.IsValidator() {
			validators = append(validators, node)
		}

		return true
	})

	return validators
}

// BlockIssersForNodes returns a map of block issuers for each node. If the node is a validator, its block issuer is the validator block issuer. Else, it is the block issuer for the test suite.
func (t *TestSuite) BlockIssuersForNodes(nodes []*mock.Node) []*mock.BlockIssuer {
	blockIssuers := make([]*mock.BlockIssuer, 0)
	for _, node := range nodes {
		if node.IsValidator() {
			blockIssuers = append(blockIssuers, node.Validator)
		} else {
			blockIssuers = append(blockIssuers, t.DefaultWallet().BlockIssuer)
		}
	}

	return blockIssuers
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

func (t *TestSuite) SplitIntoPartitions(partitions map[string][]*mock.Node) {
	for partition, nodes := range partitions {
		for _, node := range nodes {
			node.Partition = partition
			t.network.JoinWithEndpoint(node.Endpoint, partition)
		}
	}
}

func (t *TestSuite) MergePartitionsToMain(partitions ...string) {
	t.network.MergePartitionsToMain(partitions...)
}

func (t *TestSuite) SetAutomaticTransactionIssuingCounters(partition string, newValue int) {
	t.automaticTransactionIssuingCounters.Set(partition, newValue)
}
