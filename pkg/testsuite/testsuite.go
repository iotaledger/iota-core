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
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/pkg/testsuite/snapshotcreator"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
	"github.com/iotaledger/iota.go/v4/wallet"
)

type WalletOptions struct {
	Amount               iotago.BaseToken
	FixedCost            iotago.Mana
	BlockIssuanceCredits iotago.BlockIssuanceCredits
}

func WithWalletAmount(amount iotago.BaseToken) options.Option[WalletOptions] {
	return func(opts *WalletOptions) {
		opts.Amount = amount
	}
}

func WithWalletFixedCost(fixedCost iotago.Mana) options.Option[WalletOptions] {
	return func(opts *WalletOptions) {
		opts.FixedCost = fixedCost
	}
}

func WithWalletBlockIssuanceCredits(blockIssuanceCredits iotago.BlockIssuanceCredits) options.Option[WalletOptions] {
	return func(opts *WalletOptions) {
		opts.BlockIssuanceCredits = blockIssuanceCredits
	}
}

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
	optsLogger          log.Logger

	uniqueBlockTimeCounter              atomic.Int64
	automaticTransactionIssuingCounters shrinkingmap.ShrinkingMap[string, int]
	mutex                               syncutils.RWMutex
	genesisKeyManager                   *wallet.KeyManager

	currentSlot iotago.SlotIndex
}

func NewTestSuite(testingT *testing.T, opts ...options.Option[TestSuite]) *TestSuite {
	return options.Apply(&TestSuite{
		Testing:                             testingT,
		fakeTesting:                         &testing.T{},
		genesisKeyManager:                   lo.PanicOnErr(wallet.NewKeyManagerFromRandom(wallet.DefaultIOTAPath)),
		network:                             mock.NewNetwork(),
		Directory:                           utils.NewDirectory(testingT.TempDir()),
		nodes:                               orderedmap.New[string, *mock.Node](),
		wallets:                             orderedmap.New[string, *mock.Wallet](),
		blocks:                              shrinkingmap.New[string, *blocks.Block](),
		automaticTransactionIssuingCounters: *shrinkingmap.New[string, int](),

		optsWaitFor: durationFromEnvOrDefault(5*time.Second, "CI_UNIT_TESTS_WAIT_FOR"),
		optsTick:    durationFromEnvOrDefault(2*time.Millisecond, "CI_UNIT_TESTS_TICK"),
		optsLogger:  log.NewLogger(log.WithName("TestSuite")),
	}, opts, func(t *TestSuite) {
		fmt.Println("Setup TestSuite -", testingT.Name(), " @ ", time.Now())

		t.ProtocolParameterOptions = append(t.ProtocolParameterOptions, iotago.WithNetworkOptions(testingT.Name(), iotago.PrefixTestnet))
		t.API = iotago.V3API(iotago.NewV3SnapshotProtocolParameters(t.ProtocolParameterOptions...))

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

		// The first valid slot is always +1 of the genesis slot.
		t.currentSlot = t.API.TimeProvider().GenesisSlot() + 1
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

func (t *TestSuite) AccountsOfNodes(names ...string) []iotago.AccountID {
	nodes := t.Nodes(names...)

	return lo.Map(nodes, func(node *mock.Node) iotago.AccountID {
		return node.Validator.AccountID
	})
}

func (t *TestSuite) SeatOfNodes(slot iotago.SlotIndex, names ...string) []account.SeatIndex {
	nodes := t.Nodes(names...)

	return lo.Map(nodes, func(node *mock.Node) account.SeatIndex {
		seatedAccounts, exists := node.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(slot)
		require.True(t.Testing, exists, "node %s: committee at slot %d does not exist", node.Name, slot)

		seat, exists := seatedAccounts.GetSeat(node.Validator.AccountID)
		require.True(t.Testing, exists, "node %s: seat for account %s does not exist", node.Name, node.Validator.AccountID)

		return seat
	})
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

func (t *TestSuite) addNodeToPartition(name string, partition string, validator bool, walletOpts ...options.Option[WalletOptions]) *mock.Node {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if validator && t.running {
		panic(fmt.Sprintf("cannot add validator node %s to partition %s: framework already running", name, partition))
	}

	node := mock.NewNode(t.Testing, t.optsLogger, t.network, partition, name, validator)
	t.nodes.Set(name, node)
	node.SetCurrentSlot(t.currentSlot)

	walletOptions := options.Apply(&WalletOptions{
		Amount: mock.MinValidatorAccountAmount(t.API.ProtocolParameters()),
	}, walletOpts)

	if walletOptions.Amount > 0 && validator {
		accountDetails := snapshotcreator.AccountDetails{
			Address:              iotago.Ed25519AddressFromPubKey(node.Validator.PublicKey),
			Amount:               walletOptions.Amount,
			Mana:                 iotago.Mana(walletOptions.Amount),
			IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(node.Validator.PublicKey)),
			ExpirySlot:           iotago.MaxSlotIndex,
			BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 2,
			StakedAmount:         walletOptions.Amount,
			StakingEndEpoch:      iotago.MaxEpochIndex,
			FixedCost:            walletOptions.FixedCost,
			AccountID:            node.Validator.AccountID,
		}

		t.optsAccounts = append(t.optsAccounts, accountDetails)
	}

	return node
}

func (t *TestSuite) AddValidatorNodeToPartition(name string, partition string, walletOpts ...options.Option[WalletOptions]) *mock.Node {
	return t.addNodeToPartition(name, partition, true, walletOpts...)
}

func (t *TestSuite) AddValidatorNode(name string, walletOpts ...options.Option[WalletOptions]) *mock.Node {
	node := t.addNodeToPartition(name, mock.NetworkMainPartition, true, walletOpts...)
	// create a wallet for each validator node which uses the validator account as a block issuer
	t.AddWallet(name, node, node.Validator.AccountID, node.KeyManager)

	return node
}

func (t *TestSuite) AddNodeToPartition(name string, partition string, walletOpts ...options.Option[WalletOptions]) *mock.Node {
	return t.addNodeToPartition(name, partition, false, walletOpts...)
}

func (t *TestSuite) AddNode(name string, walletOpts ...options.Option[WalletOptions]) *mock.Node {
	return t.addNodeToPartition(name, mock.NetworkMainPartition, false, walletOpts...)
}

func (t *TestSuite) RemoveNode(name string) {
	t.nodes.Delete(name)
}

// AddGenesisAccount adds an account the test suite in the genesis snapshot.
func (t *TestSuite) AddGenesisAccount(accountDetails snapshotcreator.AccountDetails) {
	t.optsAccounts = append(t.optsAccounts, accountDetails)
}

// AddGenesisWallet adds a wallet to the test suite with a block issuer in the genesis snapshot and access to the genesis seed.
// If no block issuance credits are provided, the wallet will be assigned half of the maximum block issuance credits.
func (t *TestSuite) AddGenesisWallet(name string, node *mock.Node, walletOpts ...options.Option[WalletOptions]) *mock.Wallet {
	accountID := tpkg.RandAccountID()
	newWallet := t.AddWallet(name, node, accountID, t.genesisKeyManager)

	walletOptions := options.Apply(&WalletOptions{
		Amount:               mock.MinIssuerAccountAmount(t.API.ProtocolParameters()),
		BlockIssuanceCredits: iotago.MaxBlockIssuanceCredits / 2,
	}, walletOpts)

	accountDetails := snapshotcreator.AccountDetails{
		AccountID:            accountID,
		Address:              iotago.Ed25519AddressFromPubKey(newWallet.BlockIssuer.PublicKey),
		Amount:               walletOptions.Amount,
		Mana:                 iotago.Mana(mock.MinIssuerAccountAmount(t.API.ProtocolParameters())),
		IssuerKey:            iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(newWallet.BlockIssuer.PublicKey)),
		ExpirySlot:           iotago.MaxSlotIndex,
		BlockIssuanceCredits: walletOptions.BlockIssuanceCredits,
	}

	t.optsAccounts = append(t.optsAccounts, accountDetails)

	return newWallet
}

const (
	DefaultWallet = "defaultWallet"
)

func (t *TestSuite) AddDefaultWallet(node *mock.Node, walletOpts ...options.Option[WalletOptions]) *mock.Wallet {
	return t.AddGenesisWallet(DefaultWallet, node, walletOpts...)
}

func (t *TestSuite) DefaultWallet() *mock.Wallet {
	defaultWallet, exists := t.wallets.Get(DefaultWallet)
	if !exists {
		return nil
	}

	return defaultWallet
}

func (t *TestSuite) AddWallet(name string, node *mock.Node, accountID iotago.AccountID, keyManager ...*wallet.KeyManager) *mock.Wallet {
	newWallet := mock.NewWallet(t.Testing, name, node, keyManager...)
	newWallet.SetBlockIssuer(accountID)
	t.wallets.Set(name, newWallet)
	newWallet.SetCurrentSlot(t.currentSlot)

	return newWallet
}

// Update the global time of the test suite and all nodes and wallets.
func (t *TestSuite) SetCurrentSlot(slot iotago.SlotIndex) {
	t.currentSlot = slot
	t.nodes.ForEach(func(_ string, node *mock.Node) bool {
		node.SetCurrentSlot(slot)
		return true
	})
	t.wallets.ForEach(func(_ string, wallet *mock.Wallet) bool {
		wallet.SetCurrentSlot(slot)
		return true
	})
}

func (t *TestSuite) CurrentSlot() iotago.SlotIndex {
	return t.currentSlot
}

func (t *TestSuite) Run(failOnBlockFiltered bool, nodesOptions ...map[string][]options.Option[protocol.Protocol]) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Add default wallet by default when creating the first node.
	if !t.wallets.Has(DefaultWallet) {
		_, node, exists := t.nodes.Head()
		if !exists {
			panic("at least one node is needed to create a default wallet")
		}

		t.AddDefaultWallet(node)
	}

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

		return true
	})

	if _, firstNode, exists := t.nodes.Head(); exists {
		t.wallets.ForEach(func(_ string, wallet *mock.Wallet) bool {
			if err := firstNode.Protocol.Engines.Main.Get().Ledger.ForEachUnspentOutput(func(output *utxoledger.Output) bool {
				wallet.AddOutput(fmt.Sprintf("Genesis:%d", output.OutputID().Index()), output)
				return true
			}); err != nil {
				panic(err)
			}

			return true
		})
	}

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
