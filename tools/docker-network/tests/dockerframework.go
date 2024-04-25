//go:build dockertests

package tests

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/pkg/testsuite/snapshotcreator"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/nodeclient"
	"github.com/iotaledger/iota.go/v4/wallet"
)

var (
	// need to build snapshotfile in tools/docker-network.
	snapshotFilePath = "../docker-network.snapshot"
	keyManager       = func() *wallet.KeyManager {
		genesisSeed, err := base58.Decode("7R1itJx5hVuo9w9hjg5cwKFmek4HMSoBDgJZN8hKGxih")
		if err != nil {
			log.Fatal(ierrors.Wrap(err, "failed to decode base58 seed"))
		}
		keyManager, err := wallet.NewKeyManager(genesisSeed[:], wallet.DefaultIOTAPath)
		if err != nil {
			log.Fatal(ierrors.Wrap(err, "failed to create KeyManager from seed"))
		}

		return keyManager
	}
)

type Node struct {
	Name                  string
	ContainerName         string
	ClientURL             string
	AccountAddressBech32  string
	ContainerConfigs      string
	PrivateKey            string
	IssueCandidacyPayload bool
}

func (n *Node) AccountAddress(t *testing.T) *iotago.AccountAddress {
	_, addr, err := iotago.ParseBech32(n.AccountAddressBech32)
	require.NoError(t, err)
	accAddress, ok := addr.(*iotago.AccountAddress)
	require.True(t, ok)

	return accAddress
}

type DockerWalletClock struct {
	client mock.Client
}

func (c *DockerWalletClock) SetCurrentSlot(slot iotago.SlotIndex) {
	panic("Cannot set current slot in DockerWalletClock, the slot is set by time.Now()")
}

func (c *DockerWalletClock) CurrentSlot() iotago.SlotIndex {
	return c.client.LatestAPI().TimeProvider().CurrentSlot()
}

type DockerTestFramework struct {
	Testing *testing.T
	// we use the fake testing so that actual tests don't fail if an assertion fails
	fakeTesting *testing.T

	nodes     map[string]*Node
	clients   map[string]mock.Client
	nodesLock syncutils.RWMutex

	snapshotPath     string
	logDirectoryPath string

	defaultWallet *mock.Wallet

	optsProtocolParameterOptions []options.Option[iotago.V3ProtocolParameters]
	optsSnapshotOptions          []options.Option[snapshotcreator.Options]
	optsWaitForSync              time.Duration
	optsWaitFor                  time.Duration
	optsTick                     time.Duration
	optsFaucetURL                string
}

func NewDockerTestFramework(t *testing.T, opts ...options.Option[DockerTestFramework]) *DockerTestFramework {
	return options.Apply(&DockerTestFramework{
		Testing:         t,
		fakeTesting:     &testing.T{},
		nodes:           make(map[string]*Node),
		clients:         make(map[string]mock.Client),
		optsWaitForSync: 5 * time.Minute,
		optsWaitFor:     2 * time.Minute,
		optsTick:        5 * time.Second,
		optsFaucetURL:   "http://localhost:8088",
	}, opts, func(d *DockerTestFramework) {
		d.optsProtocolParameterOptions = append(DefaultProtocolParametersOptions, d.optsProtocolParameterOptions...)
		protocolParams := iotago.NewV3SnapshotProtocolParameters(d.optsProtocolParameterOptions...)
		testAPI := iotago.V3API(protocolParams)

		d.logDirectoryPath = createLogDirectory(t.Name())
		d.snapshotPath = snapshotFilePath
		d.optsSnapshotOptions = append(DefaultAccountOptions(protocolParams),
			[]options.Option[snapshotcreator.Options]{
				snapshotcreator.WithDatabaseVersion(protocol.DatabaseVersion),
				snapshotcreator.WithFilePath(d.snapshotPath),
				snapshotcreator.WithProtocolParameters(testAPI.ProtocolParameters()),
				snapshotcreator.WithRootBlocks(map[iotago.BlockID]iotago.CommitmentID{
					testAPI.ProtocolParameters().GenesisBlockID(): iotago.NewEmptyCommitment(testAPI).MustID(),
				}),
				snapshotcreator.WithGenesisKeyManager(keyManager()),
			}...)

		err := snapshotcreator.CreateSnapshot(d.optsSnapshotOptions...)
		if err != nil {
			panic(fmt.Sprintf("failed to create snapshot: %s", err))
		}
	})
}

func (d *DockerTestFramework) DockerComposeUp(detach ...bool) error {
	cmd := exec.Command("docker", "compose", "up")

	if len(detach) > 0 && detach[0] {
		cmd = exec.Command("docker", "compose", "up", "-d")
	}

	cmd.Env = os.Environ()
	for _, node := range d.Nodes() {
		cmd.Env = append(cmd.Env, fmt.Sprintf("ISSUE_CANDIDACY_PAYLOAD_%s=%t", node.Name, node.IssueCandidacyPayload))
	}

	var out strings.Builder
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		fmt.Println("Docker compose up failed with error:", err, ":", out.String())
	}

	return err
}

func (d *DockerTestFramework) Run() error {
	ch := make(chan error)
	stopCh := make(chan struct{})
	defer close(ch)
	defer close(stopCh)

	go func() {
		err := d.DockerComposeUp()

		// make sure that the channel is not already closed
		select {
		case <-stopCh:
			return
		default:
		}

		ch <- err
	}()

	timer := time.NewTimer(d.optsWaitForSync)
	defer timer.Stop()

	ticker := time.NewTicker(d.optsTick)
	defer ticker.Stop()

loop:
	for {
		select {
		case <-timer.C:
			require.FailNow(d.Testing, "Docker network did not start in time")
		case err := <-ch:
			if err != nil {
				require.FailNow(d.Testing, "failed to start Docker network", err)
			}
		case <-ticker.C:
			fmt.Println("Waiting for nodes to become available...")
			if d.waitForNodesAndGetClients() == nil {
				break loop
			}
		}
	}

	d.GetContainersConfigs()

	// make sure all nodes are up then we can start dumping logs
	d.DumpContainerLogsToFiles()

	return nil
}

func (d *DockerTestFramework) waitForNodesAndGetClients() error {
	nodes := d.Nodes()

	d.nodesLock.Lock()
	defer d.nodesLock.Unlock()
	for _, node := range nodes {
		client, err := nodeclient.New(node.ClientURL)
		if err != nil {
			return ierrors.Wrapf(err, "failed to create node client for node %s", node.Name)
		}
		d.nodes[node.Name] = node
		d.clients[node.Name] = client
	}
	d.defaultWallet = mock.NewWallet(
		d.Testing,
		"default",
		d.clients["V1"],
		&DockerWalletClock{client: d.clients["V1"]},
		lo.PanicOnErr(wallet.NewKeyManagerFromRandom(wallet.DefaultIOTAPath)),
	)

	return nil
}

func (d *DockerTestFramework) WaitUntilNetworkReady() {
	d.WaitUntilSync()

	// inx-faucet is up only when the node and indexer are healthy, thus need to check the faucet even after nodes are synced.
	d.WaitUntilFaucetHealthy()

	d.DumpContainerLogsToFiles()
}

func (d *DockerTestFramework) WaitUntilFaucetHealthy() {
	fmt.Println("Wait until the faucet is healthy...")
	defer fmt.Println("Wait until the faucet is healthy......done")

	d.Eventually(func() error {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, d.optsFaucetURL+"/health", nil)
		if err != nil {
			return err
		}

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			return ierrors.Errorf("faucet is not healthy, status code: %d", res.StatusCode)
		}

		return nil
	}, true)
}

func (d *DockerTestFramework) WaitUntilSync() {
	fmt.Println("Wait until the nodes are synced...")
	defer fmt.Println("Wait until the nodes are synced......done")

	d.Eventually(func() error {
		for _, node := range d.Nodes() {
			for {
				synced, err := d.Client(node.Name).Health(context.TODO())
				if err != nil {
					return err
				}

				if synced {
					fmt.Println("Node", node.Name, "is synced")
					break
				}
			}
		}

		return nil
	}, true)
}

func (d *DockerTestFramework) AddValidatorNode(name string, containerName string, clientURL string, accAddrBech32 string, optIssueCandidacyPayload ...bool) {
	d.nodesLock.Lock()
	defer d.nodesLock.Unlock()

	issueCandidacyPayload := true
	if len(optIssueCandidacyPayload) > 0 {
		issueCandidacyPayload = optIssueCandidacyPayload[0]
	}

	d.nodes[name] = &Node{
		Name:                  name,
		ContainerName:         containerName,
		ClientURL:             clientURL,
		AccountAddressBech32:  accAddrBech32,
		IssueCandidacyPayload: issueCandidacyPayload,
	}
}

func (d *DockerTestFramework) AddNode(name string, containerName string, clientURL string) {
	d.nodesLock.Lock()
	defer d.nodesLock.Unlock()

	d.nodes[name] = &Node{
		Name:          name,
		ContainerName: containerName,
		ClientURL:     clientURL,
	}
}

func (d *DockerTestFramework) Nodes(names ...string) []*Node {
	d.nodesLock.RLock()
	defer d.nodesLock.RUnlock()

	if len(names) == 0 {
		nodes := make([]*Node, 0, len(d.nodes))
		for _, node := range d.nodes {
			nodes = append(nodes, node)
		}

		return nodes
	}

	nodes := make([]*Node, len(names))
	for i, name := range names {
		nodes[i] = d.Node(name)
	}

	return nodes
}

func (d *DockerTestFramework) Node(name string) *Node {
	d.nodesLock.RLock()
	defer d.nodesLock.RUnlock()

	node, exist := d.nodes[name]
	require.True(d.Testing, exist)

	return node
}

func (d *DockerTestFramework) Clients(names ...string) map[string]mock.Client {
	d.nodesLock.RLock()
	defer d.nodesLock.RUnlock()

	if len(names) == 0 {
		return d.clients
	}

	clients := make(map[string]mock.Client, len(names))
	for _, name := range names {
		client, exist := d.clients[name]
		require.True(d.Testing, exist)

		clients[name] = client
	}

	return clients
}

func (d *DockerTestFramework) Client(name string) mock.Client {
	d.nodesLock.RLock()
	defer d.nodesLock.RUnlock()

	client, exist := d.clients[name]
	require.True(d.Testing, exist)

	return client
}

func (d *DockerTestFramework) NodeStatus(name string) *api.InfoResNodeStatus {
	node := d.Node(name)

	info, err := d.Client(node.Name).Info(context.TODO())
	require.NoError(d.Testing, err)

	return info.Status
}

func (d *DockerTestFramework) AccountsFromNodes(nodes ...*Node) []string {
	var accounts []string
	for _, node := range nodes {
		if node.AccountAddressBech32 != "" {
			accounts = append(accounts, node.AccountAddressBech32)
		}
	}

	return accounts
}

func (d *DockerTestFramework) StartIssueCandidacyPayload(nodes ...*Node) {
	if len(nodes) == 0 {
		return
	}

	for _, node := range nodes {
		node.IssueCandidacyPayload = true
	}

	err := d.DockerComposeUp(true)
	require.NoError(d.Testing, err)
}

func (d *DockerTestFramework) StopIssueCandidacyPayload(nodes ...*Node) {
	if len(nodes) == 0 {
		return
	}

	for _, node := range nodes {
		node.IssueCandidacyPayload = false
	}

	err := d.DockerComposeUp(true)
	require.NoError(d.Testing, err)
}

func (d *DockerTestFramework) IssueCandidacyPayloadFromAccount(wallet *mock.Wallet) iotago.BlockID {
	block, err := wallet.CreateAndSubmitBasicBlock(context.TODO(), "candidacy_payload", mock.WithPayload(&iotago.CandidacyAnnouncement{}))
	require.NoError(d.Testing, err)

	return block.ID()
}

// CreateTaggedDataBlock creates and submits a block of a tagged data payload.
func (d *DockerTestFramework) CreateTaggedDataBlock(wallet *mock.Wallet, tag []byte) *iotago.Block {
	ctx := context.TODO()

	return lo.PanicOnErr(wallet.CreateBasicBlock(ctx, "", mock.WithPayload(&iotago.TaggedData{
		Tag: tag,
	}))).ProtocolBlock()
}

func (d *DockerTestFramework) CreateBasicOutputBlock(wallet *mock.Wallet) (*iotago.Block, *iotago.SignedTransaction, *mock.OutputData) {
	fundsOutputData := d.RequestFaucetFunds(context.Background(), wallet, iotago.AddressEd25519)

	signedTx := wallet.CreateBasicOutputFromInput(fundsOutputData)
	block, err := wallet.CreateBasicBlock(context.Background(), "", mock.WithPayload(signedTx))
	require.NoError(d.Testing, err)

	return block.ProtocolBlock(), signedTx, fundsOutputData
}

// CreateDelegationBlockFromInput consumes the given basic output, then build a block of a transaction that includes a delegation output, in order to delegate the given validator.
func (d *DockerTestFramework) CreateDelegationBlockFromInput(wallet *mock.Wallet, accountAdddress *iotago.AccountAddress, input *mock.OutputData) (iotago.DelegationID, iotago.OutputID, *iotago.Block) {
	ctx := context.TODO()
	clt := wallet.Client

	signedTx := wallet.CreateDelegationFromInput(
		"",
		input,
		mock.WithDelegatedValidatorAddress(accountAdddress),
		mock.WithDelegationStartEpoch(getDelegationStartEpoch(clt.LatestAPI(), wallet.GetNewBlockIssuanceResponse().LatestCommitment.Slot)),
	)
	outputID := iotago.OutputIDFromTransactionIDAndIndex(signedTx.Transaction.MustID(), 0)

	return iotago.DelegationIDFromOutputID(outputID),
		outputID,
		lo.PanicOnErr(wallet.CreateBasicBlock(ctx, "", mock.WithPayload(signedTx))).ProtocolBlock()
}

// CreateFoundryBlockFromInput consumes the given basic output, then build a block of a transaction that includes a foundry output with the given mintedAmount and maxSupply.
func (d *DockerTestFramework) CreateFoundryBlockFromInput(wallet *mock.Wallet, inputID iotago.OutputID, mintedAmount iotago.BaseToken, maxSupply iotago.BaseToken) (iotago.FoundryID, iotago.OutputID, *iotago.Block) {
	input := wallet.Output(inputID)
	signedTx := wallet.CreateFoundryAndNativeTokensFromInput(input, mintedAmount, maxSupply)
	txID, err := signedTx.Transaction.ID()
	require.NoError(d.Testing, err)

	//nolint:forcetypeassert
	return signedTx.Transaction.Outputs[1].(*iotago.FoundryOutput).MustFoundryID(),
		iotago.OutputIDFromTransactionIDAndIndex(txID, 1),
		lo.PanicOnErr(wallet.CreateBasicBlock(context.Background(), "", mock.WithPayload(signedTx))).ProtocolBlock()
}

// CreateNFTBlockFromInput consumes the given basic output, then build a block of a transaction that includes a NFT output with the given NFT output options.
func (d *DockerTestFramework) CreateNFTBlockFromInput(wallet *mock.Wallet, input *mock.OutputData, opts ...options.Option[builder.NFTOutputBuilder]) (iotago.NFTID, iotago.OutputID, *iotago.Block) {
	signedTx := wallet.CreateTaggedNFTFromInput("", input, opts...)
	outputID := iotago.OutputIDFromTransactionIDAndIndex(signedTx.Transaction.MustID(), 0)

	return iotago.NFTIDFromOutputID(outputID),
		outputID,
		lo.PanicOnErr(wallet.CreateBasicBlock(context.Background(), "", mock.WithPayload(signedTx))).ProtocolBlock()
}

// CreateFoundryTransitionBlockFromInput consumes the given foundry output, then build block by increasing the minted amount by 1.
func (d *DockerTestFramework) CreateFoundryTransitionBlockFromInput(issuerID iotago.AccountID, foundryInput, accountInput *mock.OutputData) (iotago.FoundryID, iotago.OutputID, *iotago.Block) {
	signedTx := d.defaultWallet.TransitionFoundry("", foundryInput, accountInput)
	txID, err := signedTx.Transaction.ID()
	require.NoError(d.Testing, err)

	//nolint:forcetypeassert
	return signedTx.Transaction.Outputs[1].(*iotago.FoundryOutput).MustFoundryID(),
		iotago.OutputIDFromTransactionIDAndIndex(txID, 1),
		lo.PanicOnErr(d.defaultWallet.CreateAndSubmitBasicBlock(context.Background(), "foundry_transition", mock.WithPayload(signedTx))).ProtocolBlock()
}

// CreateAccountBlockFromInput consumes the given output, which should be either an basic output with implicit address, then build block with the given account output options.
func (d *DockerTestFramework) CreateAccountBlock(opts ...options.Option[builder.AccountOutputBuilder]) (*mock.AccountData, *mock.Wallet, *iotago.SignedTransaction, *iotago.Block) {
	// create an implicit account by requesting faucet funds
	ctx := context.TODO()
	newWallet, implicitAccountOutputData := d.CreateImplicitAccount(ctx)

	var implicitBlockIssuerKey iotago.BlockIssuerKey = iotago.Ed25519PublicKeyHashBlockIssuerKeyFromImplicitAccountCreationAddress(newWallet.ImplicitAccountCreationAddress())
	opts = append(opts, mock.WithBlockIssuerFeature(
		iotago.NewBlockIssuerKeys(implicitBlockIssuerKey),
		iotago.MaxSlotIndex,
	))
	signedTx := newWallet.TransitionImplicitAccountToAccountOutput("", []*mock.OutputData{implicitAccountOutputData}, opts...)

	// The account transition block should be issued by the implicit account block issuer key.
	block, err := newWallet.CreateBasicBlock(ctx, "", mock.WithPayload(signedTx))
	require.NoError(d.Testing, err)
	accOutputID := iotago.OutputIDFromTransactionIDAndIndex(signedTx.Transaction.MustID(), 0)
	accOutput := signedTx.Transaction.Outputs[0].(*iotago.AccountOutput)
	accAddress := (accOutput.AccountID).ToAddress().(*iotago.AccountAddress)

	accountOutputData := &mock.AccountData{
		ID:           accOutput.AccountID,
		Address:      accAddress,
		Output:       accOutput,
		OutputID:     accOutputID,
		AddressIndex: implicitAccountOutputData.AddressIndex,
	}

	return accountOutputData, newWallet, signedTx, block.ProtocolBlock()
}

// CreateImplicitAccount requests faucet funds and creates an implicit account. It already wait until the transaction is committed and the created account is useable.
func (d *DockerTestFramework) CreateImplicitAccount(ctx context.Context) (*mock.Wallet, *mock.OutputData) {
	newWallet := mock.NewWallet(d.Testing, "", d.defaultWallet.Client, &DockerWalletClock{client: d.defaultWallet.Client})
	implicitAccountOutputData := d.RequestFaucetFunds(ctx, newWallet, iotago.AddressImplicitAccountCreation)

	accountID := iotago.AccountIDFromOutputID(implicitAccountOutputData.ID)
	accountAddress, ok := accountID.ToAddress().(*iotago.AccountAddress)
	require.True(d.Testing, ok)

	// make sure an implicit account is committed
	d.CheckAccountStatus(ctx, iotago.EmptyBlockID, implicitAccountOutputData.ID.TransactionID(), implicitAccountOutputData.ID, accountAddress)

	// update the wallet with the new account data
	newWallet.SetBlockIssuer(&mock.AccountData{
		ID:           accountID,
		Address:      accountAddress,
		OutputID:     implicitAccountOutputData.ID,
		AddressIndex: implicitAccountOutputData.AddressIndex,
	})

	return newWallet, implicitAccountOutputData
}

// CreateAccount creates an new account from implicit one to full one, it already wait until the transaction is committed and the created account is useable.
func (d *DockerTestFramework) CreateAccount(opts ...options.Option[builder.AccountOutputBuilder]) (*mock.Wallet, *mock.AccountData) {
	ctx := context.TODO()
	accountData, newWallet, signedTx, block := d.CreateAccountBlock(opts...)
	d.SubmitBlock(ctx, block)
	d.CheckAccountStatus(ctx, block.MustID(), signedTx.Transaction.MustID(), accountData.OutputID, accountData.Address, true)

	// update the wallet with the new account data
	newWallet.SetBlockIssuer(accountData)

	fmt.Printf("Account created, Bech addr: %s\n", accountData.Address.Bech32(newWallet.Client.CommittedAPI().ProtocolParameters().Bech32HRP()))

	return newWallet, newWallet.Account(accountData.ID)
}

func (d *DockerTestFramework) ClaimRewardsForValidator(ctx context.Context, validatorWallet *mock.Wallet) {
	validatorAccountData := validatorWallet.BlockIssuer.AccountData
	outputData := &mock.OutputData{
		ID:           validatorAccountData.OutputID,
		Address:      validatorAccountData.Address,
		AddressIndex: validatorAccountData.AddressIndex,
		Output:       validatorAccountData.Output,
	}
	signedTx := validatorWallet.ClaimValidatorRewards("", outputData)

	validatorWallet.CreateAndSubmitBasicBlock(ctx, "claim_rewards_validator", mock.WithPayload(signedTx))
	d.AwaitTransactionPayloadAccepted(ctx, signedTx.Transaction.MustID())

	// update account data of validator
	validatorWallet.SetBlockIssuer(&mock.AccountData{
		ID:           validatorWallet.BlockIssuer.AccountData.ID,
		Address:      validatorWallet.BlockIssuer.AccountData.Address,
		AddressIndex: validatorWallet.BlockIssuer.AccountData.AddressIndex,
		OutputID:     iotago.OutputIDFromTransactionIDAndIndex(signedTx.Transaction.MustID(), 0),
		Output:       signedTx.Transaction.Outputs[0].(*iotago.AccountOutput),
	})
}

func (d *DockerTestFramework) ClaimRewardsForDelegator(ctx context.Context, wallet *mock.Wallet, delegationOutputData *mock.OutputData) iotago.OutputID {
	signedTx := wallet.ClaimDelegatorRewards("", delegationOutputData)

	wallet.CreateAndSubmitBasicBlock(ctx, "claim_rewards_delegator", mock.WithPayload(signedTx))
	d.AwaitTransactionPayloadAccepted(ctx, signedTx.Transaction.MustID())

	return iotago.OutputIDFromTransactionIDAndIndex(signedTx.Transaction.MustID(), 0)
}

func (d *DockerTestFramework) DelayedClaimingTransition(ctx context.Context, wallet *mock.Wallet, delegationOutputData *mock.OutputData) *mock.OutputData {
	signedTx := wallet.DelayedClaimingTransition("delayed_claim_tx", delegationOutputData)

	wallet.CreateAndSubmitBasicBlock(ctx, "delayed_claim", mock.WithPayload(signedTx))
	d.AwaitTransactionPayloadAccepted(ctx, signedTx.Transaction.MustID())

	return &mock.OutputData{
		ID:           iotago.OutputIDFromTransactionIDAndIndex(signedTx.Transaction.MustID(), 0),
		Address:      wallet.Address(),
		AddressIndex: 0,
		Output:       signedTx.Transaction.Outputs[0].(*iotago.DelegationOutput),
	}
}

// DelegateToValidator requests faucet funds and delegate the UTXO output to the validator.
func (d *DockerTestFramework) DelegateToValidator(fromWallet *mock.Wallet, accountAddress *iotago.AccountAddress) *mock.OutputData {
	// requesting faucet funds as delegation input
	ctx := context.TODO()
	fundsOutputData := d.RequestFaucetFunds(ctx, fromWallet, iotago.AddressEd25519)

	signedTx := fromWallet.CreateDelegationFromInput(
		"delegation_tx",
		fundsOutputData,
		mock.WithDelegatedValidatorAddress(accountAddress),
		mock.WithDelegationStartEpoch(getDelegationStartEpoch(fromWallet.Client.LatestAPI(), fromWallet.GetNewBlockIssuanceResponse().LatestCommitment.Slot)),
	)

	fromWallet.CreateAndSubmitBasicBlock(ctx, "delegation", mock.WithPayload(signedTx))
	d.AwaitTransactionPayloadAccepted(ctx, signedTx.Transaction.MustID())

	delegationOutput, ok := signedTx.Transaction.Outputs[0].(*iotago.DelegationOutput)
	require.True(d.Testing, ok)

	return &mock.OutputData{
		ID:           iotago.OutputIDFromTransactionIDAndIndex(signedTx.Transaction.MustID(), 0),
		Address:      fromWallet.Address(),
		AddressIndex: 0,
		Output:       delegationOutput,
	}

}

// PrepareBlockIssuance prepares the BlockIssuance and Congestion response, and increase BIC of the issuer if necessary.
func (d *DockerTestFramework) PrepareBlockIssuance(ctx context.Context, clt mock.Client, issuerAddress *iotago.AccountAddress) (*api.IssuanceBlockHeaderResponse, *api.CongestionResponse) {
	issuerResp, err := clt.BlockIssuance(ctx)
	require.NoError(d.Testing, err)

	congestionResp, err := clt.Congestion(ctx, issuerAddress, 0, lo.PanicOnErr(issuerResp.LatestCommitment.ID()))
	require.NoError(d.Testing, err)

	return issuerResp, congestionResp
}

// AllotManaTo requests faucet funds then uses it to allots mana from one account to another.
func (d *DockerTestFramework) AllotManaTo(fromWallet *mock.Wallet, to *mock.AccountData, manaToAllot iotago.Mana) {
	// requesting faucet funds for allotment
	ctx := context.TODO()
	fundsOutputID := d.RequestFaucetFunds(ctx, fromWallet, iotago.AddressEd25519)
	clt := fromWallet.Client

	signedTx := fromWallet.AllotManaFromBasicOutput(
		"allotment_tx",
		fundsOutputID,
		manaToAllot,
		to.ID,
	)
	preAllotmentCommitmentID := fromWallet.GetNewBlockIssuanceResponse().LatestCommitment.MustID()
	block, err := fromWallet.CreateAndSubmitBasicBlock(ctx, "allotment", mock.WithPayload(signedTx))
	require.NoError(d.Testing, err)
	fmt.Println("Allot mana transaction sent, blkID:", block.ID().ToHex(), ", txID:", signedTx.Transaction.MustID().ToHex(), ", slot:", block.ID().Slot())

	d.AwaitTransactionPayloadAccepted(ctx, signedTx.Transaction.MustID())

	// allotment is updated when the transaction is committed
	d.AwaitCommitment(block.ID().Slot())

	// check if the mana is allotted
	toCongestionResp, err := clt.Congestion(ctx, to.Address, 0, preAllotmentCommitmentID)
	require.NoError(d.Testing, err)
	oldBIC := toCongestionResp.BlockIssuanceCredits

	toCongestionResp, err = clt.Congestion(ctx, to.Address, 0)
	require.NoError(d.Testing, err)
	newBIC := toCongestionResp.BlockIssuanceCredits

	decayedOldBIC, err := clt.LatestAPI().ManaDecayProvider().DecayManaBySlots(iotago.Mana(oldBIC), preAllotmentCommitmentID.Slot(), block.ID().Slot())
	expectedBIC := iotago.BlockIssuanceCredits(decayedOldBIC + manaToAllot)
	require.Equal(d.Testing, expectedBIC, newBIC)
}

// CreateNativeToken request faucet funds then use it to create native token for the account, and returns the updated Account.
func (d *DockerTestFramework) CreateNativeToken(fromWallet *mock.Wallet, mintedAmount iotago.BaseToken, maxSupply iotago.BaseToken) {
	require.GreaterOrEqual(d.Testing, maxSupply, mintedAmount)

	ctx := context.TODO()

	// requesting faucet funds for native token creation
	fundsOutputData := d.RequestFaucetFunds(ctx, fromWallet, iotago.AddressEd25519)

	signedTx := fromWallet.CreateFoundryAndNativeTokensFromInput(fundsOutputData, mintedAmount, maxSupply)

	block, err := fromWallet.CreateAndSubmitBasicBlock(ctx, "native_token", mock.WithPayload(signedTx))
	require.NoError(d.Testing, err)

	txID := signedTx.Transaction.MustID()
	d.AwaitTransactionPayloadAccepted(ctx, txID)

	fmt.Println("Create native tokens transaction sent, blkID:", block.ID().ToHex(), ", txID:", signedTx.Transaction.MustID().ToHex(), ", slot:", block.ID().Slot())

	// wait for the account to be committed
	d.AwaitCommitment(block.ID().Slot())

	d.AssertIndexerAccount(fromWallet.BlockIssuer.AccountData)
	//nolint:forcetypeassert
	d.AssertIndexerFoundry(signedTx.Transaction.Outputs[1].(*iotago.FoundryOutput).MustFoundryID())
}

// RequestFaucetFunds requests faucet funds for the given address type, and returns the outputID of the received funds.
func (d *DockerTestFramework) RequestFaucetFunds(ctx context.Context, wallet *mock.Wallet, addressType iotago.AddressType) *mock.OutputData {
	var address iotago.Address
	if addressType == iotago.AddressImplicitAccountCreation {
		address = wallet.ImplicitAccountCreationAddress(wallet.BlockIssuer.AccountData.AddressIndex)
	} else {
		address = wallet.Address()
	}

	d.SendFaucetRequest(ctx, wallet, address)

	outputID, output, err := d.AwaitAddressUnspentOutputAccepted(ctx, wallet, address)
	require.NoError(d.Testing, err)

	outputData := &mock.OutputData{
		ID:           outputID,
		Address:      address,
		AddressIndex: wallet.BlockIssuer.AccountData.AddressIndex,
		Output:       output,
	}
	wallet.AddOutput("faucet funds", outputData)

	fmt.Printf("Faucet funds received, txID: %s, amount: %d, mana: %d\n", outputID.TransactionID().ToHex(), output.BaseTokenAmount(), output.StoredMana())

	return outputData
}

func (d *DockerTestFramework) Stop() {
	fmt.Println("Stop the network...")
	defer fmt.Println("Stop the network.....done")

	_ = exec.Command("docker", "compose", "down").Run()
	_ = exec.Command("rm", d.snapshotPath).Run() //nolint:gosec
}

func (d *DockerTestFramework) StopContainer(containerName ...string) error {
	fmt.Println("Stop validator", containerName, "......")

	args := append([]string{"stop"}, containerName...)

	return exec.Command("docker", args...).Run()
}

func (d *DockerTestFramework) RestartContainer(containerName ...string) error {
	fmt.Println("Restart validator", containerName, "......")

	args := append([]string{"restart"}, containerName...)

	return exec.Command("docker", args...).Run()
}

func (d *DockerTestFramework) DumpContainerLogsToFiles() {
	// get container names
	cmd := "docker compose ps | awk '{print $1}' | tail -n +2"
	containerNamesBytes, err := exec.Command("bash", "-c", cmd).Output()
	require.NoError(d.Testing, err)

	// dump logs to files
	fmt.Println("Dump container logs to files...")
	containerNames := strings.Split(string(containerNamesBytes), "\n")

	for _, name := range containerNames {
		if name == "" {
			continue
		}

		filePath := fmt.Sprintf("%s/%s.log", d.logDirectoryPath, name)
		// dump logs to file if the file does not exist, which means the container is just started.
		// logs should exist for the already running containers.
		_, err := os.Stat(filePath)
		if os.IsNotExist(err) {
			logCmd := fmt.Sprintf("docker logs -f %s > %s 2>&1 &", name, filePath)
			err := exec.Command("bash", "-c", logCmd).Run()
			require.NoError(d.Testing, err)
		}
	}
}

func (d *DockerTestFramework) GetContainersConfigs() {
	// get container configs
	nodes := d.Nodes()

	d.nodesLock.Lock()
	defer d.nodesLock.Unlock()
	for _, node := range nodes {
		cmd := fmt.Sprintf("docker inspect --format='{{.Config.Cmd}}' %s", node.ContainerName)
		containerConfigsBytes, err := exec.Command("bash", "-c", cmd).Output()
		require.NoError(d.Testing, err)

		configs := string(containerConfigsBytes)
		// remove "[" and "]"
		configs = configs[1 : len(configs)-2]

		// get validator private key
		cmd = fmt.Sprintf("docker inspect --format='{{.Config.Env}}' %s", node.ContainerName)
		envBytes, err := exec.Command("bash", "-c", cmd).Output()
		require.NoError(d.Testing, err)

		envs := string(envBytes)
		envs = strings.Split(envs[1:len(envs)-2], " ")[0]

		node.ContainerConfigs = configs
		node.PrivateKey = envs
		d.nodes[node.Name] = node
	}
}

func (d *DockerTestFramework) SubmitBlock(ctx context.Context, blk *iotago.Block) {
	clt := d.defaultWallet.Client

	_, err := clt.SubmitBlock(ctx, blk)
	require.NoError(d.Testing, err)
}
