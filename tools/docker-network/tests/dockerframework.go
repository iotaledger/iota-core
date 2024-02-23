//go:build dockertests

package tests

import (
	"context"
	"crypto/ed25519"
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
	"github.com/iotaledger/iota-core/pkg/testsuite/snapshotcreator"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/nodeclient"
	"github.com/iotaledger/iota.go/v4/wallet"
)

var (
	// need to build snapshotfile in tools/docker-network
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
	Client                *nodeclient.Client
	AccountAddressBech32  string
	ContainerConfigs      string
	PrivateKey            string
	IssueCandidacyPayload bool
}

type Account struct {
	AccountID      iotago.AccountID
	AccountAddress *iotago.AccountAddress
	BlockIssuerKey ed25519.PrivateKey
	AccountOutput  *iotago.AccountOutput
	OutputID       iotago.OutputID
}

type DockerTestFramework struct {
	Testing *testing.T

	nodes     map[string]*Node
	nodesLock syncutils.RWMutex

	snapshotPath     string
	logDirectoryPath string

	defaultWallet *Wallet

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
		nodes:           make(map[string]*Node),
		defaultWallet:   NewWallet(t, "default"),
		optsWaitForSync: 5 * time.Minute,
		optsWaitFor:     2 * time.Minute,
		optsTick:        5 * time.Second,
		optsFaucetURL:   "http://localhost:8088",
	}, opts, func(d *DockerTestFramework) {
		d.optsProtocolParameterOptions = append(DefaultProtocolParametersOptions, d.optsProtocolParameterOptions...)
		protocolParams := iotago.NewV3SnapshotProtocolParameters(d.optsProtocolParameterOptions...)
		api := iotago.V3API(protocolParams)

		d.logDirectoryPath = createLogDirectory(t.Name())
		d.snapshotPath = snapshotFilePath
		d.optsSnapshotOptions = append(DefaultAccountOptions(protocolParams),
			[]options.Option[snapshotcreator.Options]{
				snapshotcreator.WithDatabaseVersion(protocol.DatabaseVersion),
				snapshotcreator.WithFilePath(d.snapshotPath),
				snapshotcreator.WithProtocolParameters(api.ProtocolParameters()),
				snapshotcreator.WithRootBlocks(map[iotago.BlockID]iotago.CommitmentID{
					api.ProtocolParameters().GenesisBlockID(): iotago.NewEmptyCommitment(api).MustID(),
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

		node.Client = client
		d.nodes[node.Name] = node
	}

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
		require.NoError(d.Testing, err)

		res, err := http.DefaultClient.Do(req)
		require.NoError(d.Testing, err)
		defer res.Body.Close()

		require.Equal(d.Testing, http.StatusOK, res.StatusCode)

		return nil
	}, true)
}

func (d *DockerTestFramework) WaitUntilSync() {
	fmt.Println("Wait until the nodes are synced...")
	defer fmt.Println("Wait until the nodes are synced......done")

	d.Eventually(func() error {
		for _, node := range d.Nodes() {
			for {
				synced, err := node.Client.Health(context.TODO())
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

func (d *DockerTestFramework) NodeStatus(name string) *api.InfoResNodeStatus {
	node := d.Node(name)

	info, err := node.Client.Info(context.TODO())
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

	d.DockerComposeUp(true)
}

func (d *DockerTestFramework) StopIssueCandidacyPayload(nodes ...*Node) {
	if len(nodes) == 0 {
		return
	}

	for _, node := range nodes {
		node.IssueCandidacyPayload = false
	}

	d.DockerComposeUp(true)
}

func (d *DockerTestFramework) CreateTaggedDataBlock(issuerId iotago.AccountID, tag []byte) *iotago.Block {
	issuer := d.defaultWallet.Account(issuerId)
	ctx := context.TODO()
	clt := d.Node("V1").Client

	issuerResp, err := clt.BlockIssuance(ctx)
	require.NoError(d.Testing, err)

	congestionResp, err := clt.Congestion(ctx, issuer.AccountAddress, 0, lo.PanicOnErr(issuerResp.LatestCommitment.ID()))
	require.NoError(d.Testing, err)

	if congestionResp.BlockIssuanceCredits <= 500 {
		d.IncreaseBIC(issuerId)

		// need to renew BlockIssuance and Congestion response
		issuerResp, err = clt.BlockIssuance(ctx)
		require.NoError(d.Testing, err)

		congestionResp, err = clt.Congestion(ctx, issuer.AccountAddress, 0, lo.PanicOnErr(issuerResp.LatestCommitment.ID()))
		require.NoError(d.Testing, err)
	}

	return d.CreateBlock(ctx, &iotago.TaggedData{
		Tag: tag,
	}, wallet.NewEd25519Account(issuer.AccountID, issuer.BlockIssuerKey), congestionResp, issuerResp)
}

func (d *DockerTestFramework) CreateDelegationBlockFromInput(issuerId iotago.AccountID, validator *Node, inputId iotago.OutputID) (iotago.DelegationID, iotago.OutputID, *iotago.Block) {
	issuer := d.defaultWallet.Account(issuerId)
	ctx := context.TODO()
	clt := d.Node("V1").Client

	issuerResp, err := clt.BlockIssuance(ctx)
	require.NoError(d.Testing, err)

	congestionResp, err := clt.Congestion(ctx, issuer.AccountAddress, 0, lo.PanicOnErr(issuerResp.LatestCommitment.ID()))
	require.NoError(d.Testing, err)

	if congestionResp.BlockIssuanceCredits <= 500 {
		d.IncreaseBIC(issuerId)

		// need to renew BlockIssuance and Congestion response
		issuerResp, err = clt.BlockIssuance(ctx)
		require.NoError(d.Testing, err)

		congestionResp, err = clt.Congestion(ctx, issuer.AccountAddress, 0, lo.PanicOnErr(issuerResp.LatestCommitment.ID()))
		require.NoError(d.Testing, err)
	}

	signedTx := d.defaultWallet.CreateDelegationFromInput(clt, issuerId, validator, inputId, issuerResp)
	delegationOutput := signedTx.Transaction.Outputs[0].(*iotago.DelegationOutput)
	txId, err := signedTx.Transaction.ID()
	require.NoError(d.Testing, err)

	return delegationOutput.DelegationID,
		iotago.OutputIDFromTransactionIDAndIndex(txId, 0),
		d.CreateBlock(ctx, signedTx, wallet.NewEd25519Account(issuer.AccountID, issuer.BlockIssuerKey), congestionResp, issuerResp)
}

func (d *DockerTestFramework) CreateFoundryBlockFromInput(issuerId iotago.AccountID, inputId iotago.OutputID, mintedAmount iotago.BaseToken, maxSupply iotago.BaseToken) (iotago.FoundryID, iotago.OutputID, *iotago.Block) {
	issuer := d.defaultWallet.Account(issuerId)
	ctx := context.TODO()
	clt := d.Node("V1").Client

	issuerResp, err := clt.BlockIssuance(ctx)
	require.NoError(d.Testing, err)

	congestionResp, err := clt.Congestion(ctx, issuer.AccountAddress, 0, lo.PanicOnErr(issuerResp.LatestCommitment.ID()))
	require.NoError(d.Testing, err)

	signedTx := d.defaultWallet.CreateFoundryAndNativeTokensFromInput(clt, issuerId, inputId, mintedAmount, maxSupply, issuerResp)
	txId, err := signedTx.Transaction.ID()
	require.NoError(d.Testing, err)

	return signedTx.Transaction.Outputs[1].(*iotago.FoundryOutput).MustFoundryID(),
		iotago.OutputIDFromTransactionIDAndIndex(txId, 1),
		d.CreateBlock(ctx, signedTx, wallet.NewEd25519Account(issuer.AccountID, issuer.BlockIssuerKey), congestionResp, issuerResp)
}

func (d *DockerTestFramework) CreateFoundryTransitionBlockFromInput(issuerId iotago.AccountID, inputId iotago.OutputID) (iotago.FoundryID, iotago.OutputID, *iotago.Block) {
	ctx := context.TODO()
	clt := d.Node("V1").Client
	issuer := d.defaultWallet.Account(issuerId)

	issuerResp, err := clt.BlockIssuance(ctx)
	require.NoError(d.Testing, err)

	congestionResp, err := clt.Congestion(ctx, issuer.AccountAddress, 0, lo.PanicOnErr(issuerResp.LatestCommitment.ID()))
	require.NoError(d.Testing, err)

	signedTx := d.defaultWallet.TransitionFoundry(clt, issuerId, inputId, issuerResp)
	txId, err := signedTx.Transaction.ID()
	require.NoError(d.Testing, err)

	return signedTx.Transaction.Outputs[1].(*iotago.FoundryOutput).MustFoundryID(),
		iotago.OutputIDFromTransactionIDAndIndex(txId, 1),
		d.CreateBlock(ctx, signedTx, wallet.NewEd25519Account(issuer.AccountID, issuer.BlockIssuerKey), congestionResp, issuerResp)
}

// CreateAccountBlockFromInput consumes the given output, which should be either an basic output with implicit address or an account output, then build block with the given account output options.
func (d *DockerTestFramework) CreateAccountBlockFromInput(inputId iotago.OutputID, opts ...options.Option[builder.AccountOutputBuilder]) (*Account, iotago.OutputID, *iotago.Block) {
	ctx := context.TODO()
	clt := d.Node("V1").Client
	input := d.defaultWallet.Output(inputId)

	issuerResp, err := clt.BlockIssuance(ctx)
	require.NoError(d.Testing, err)

	congestionResp, err := clt.Congestion(ctx, input.Address.(*iotago.AccountAddress), 0, lo.PanicOnErr(issuerResp.LatestCommitment.ID()))
	require.NoError(d.Testing, err)

	account, signedTx := d.defaultWallet.TransitionImplicitAccountToAccountOutput(clt, input.ID, issuerResp)
	txId, err := signedTx.Transaction.ID()
	require.NoError(d.Testing, err)

	return account,
		iotago.OutputIDFromTransactionIDAndIndex(txId, 0),
		d.CreateBlock(ctx, signedTx, wallet.NewEd25519Account(account.AccountID, account.BlockIssuerKey), congestionResp, issuerResp)
}

// CreateImplicitAccount requests faucet funds and creates an implicit account. It already wait until the transaction is committed and the created account is useable.
func (d *DockerTestFramework) CreateImplicitAccount(ctx context.Context) *Account {
	fundsOutputID := d.RequestFaucetFunds(ctx, iotago.AddressImplicitAccountCreation)

	accountID := iotago.AccountIDFromOutputID(fundsOutputID)
	accountAddress, ok := accountID.ToAddress().(*iotago.AccountAddress)
	require.True(d.Testing, ok)

	accountInfo := &Account{
		AccountID:      accountID,
		AccountAddress: accountAddress,
		BlockIssuerKey: d.defaultWallet.Output(fundsOutputID).PrivateKey,
		OutputID:       fundsOutputID,
	}

	d.defaultWallet.AddAccount(accountID, accountInfo)

	// make sure an implicit account is committed
	d.CheckAccountStatus(ctx, iotago.EmptyBlockID, fundsOutputID.TransactionID(), fundsOutputID, accountAddress)

	// Note: the implicit account output is not an AccountOutput, thus we ignore it here.
	return accountInfo
}

// CreateAccount creates an new account from implicit one to full one, it already wait until the transaction is committed and the created account is useable.
func (d *DockerTestFramework) CreateAccount(opts ...options.Option[builder.AccountOutputBuilder]) *Account {
	// create an implicit account by requesting faucet funds
	ctx := context.TODO()
	implicitAccount := d.CreateImplicitAccount(ctx)

	// transition to a full account with new Ed25519 address and staking feature
	clt := d.Node("V1").Client

	issuerResp, err := clt.BlockIssuance(ctx)
	require.NoError(d.Testing, err)

	congestionResp, err := clt.Congestion(ctx, implicitAccount.AccountAddress, 0, lo.PanicOnErr(issuerResp.LatestCommitment.ID()))
	require.NoError(d.Testing, err)

	fullAccount, signedTx := d.defaultWallet.TransitionImplicitAccountToAccountOutput(clt, implicitAccount.OutputID, issuerResp, opts...)

	// The account transition block should be issued by the implicit account block issuer key.
	blkID := d.SubmitPayload(ctx, signedTx, wallet.NewEd25519Account(implicitAccount.AccountID, implicitAccount.BlockIssuerKey), congestionResp, issuerResp)

	// check if the account is committed
	accOutputID := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0)
	d.CheckAccountStatus(ctx, blkID, lo.PanicOnErr(signedTx.Transaction.ID()), accOutputID, implicitAccount.AccountAddress, true)

	fmt.Printf("Account created, Bech addr: %s, in txID: %s, slot: %d\n", implicitAccount.AccountAddress.Bech32(clt.CommittedAPI().ProtocolParameters().Bech32HRP()), lo.PanicOnErr(signedTx.Transaction.ID()).ToHex(), blkID.Slot())

	return fullAccount
}

// DelegateToValidator requests faucet funds and delegate the UTXO output to the validator.
func (d *DockerTestFramework) DelegateToValidator(fromId iotago.AccountID, validator *Node) iotago.EpochIndex {
	from := d.defaultWallet.Account(fromId)

	// requesting faucet funds as delegation input
	ctx := context.TODO()
	fundsOutputID := d.RequestFaucetFunds(ctx, iotago.AddressEd25519)

	clt := validator.Client

	issuerResp, err := clt.BlockIssuance(ctx)
	require.NoError(d.Testing, err)

	congestionResp, err := clt.Congestion(ctx, from.AccountAddress, 0, lo.PanicOnErr(issuerResp.LatestCommitment.ID()))
	require.NoError(d.Testing, err)

	signedTx := d.defaultWallet.CreateDelegationFromInput(clt, fromId, validator, fundsOutputID, issuerResp)

	blkID := d.SubmitPayload(ctx, signedTx, wallet.NewEd25519Account(from.AccountID, from.BlockIssuerKey), congestionResp, issuerResp)

	d.AwaitTransactionPayloadAccepted(ctx, blkID)

	delegationOutput := signedTx.Transaction.Outputs[0].(*iotago.DelegationOutput)

	return delegationOutput.StartEpoch
}

// IncreaseBIC requests faucet funds then uses it to allots mana to an account.
func (d *DockerTestFramework) IncreaseBIC(toId iotago.AccountID) {
	to := d.defaultWallet.Account(toId)

	// requesting faucet funds for allotment
	ctx := context.TODO()
	fundsOutputID := d.RequestFaucetFunds(ctx, iotago.AddressEd25519)
	clt := d.Node("V1").Client

	issuerResp, err := clt.BlockIssuance(ctx)
	require.NoError(d.Testing, err)

	congestionResp, err := clt.Congestion(ctx, to.AccountAddress, 0, lo.PanicOnErr(issuerResp.LatestCommitment.ID()))
	require.NoError(d.Testing, err)

	signedTx := d.defaultWallet.AllotManaFromInput(clt, toId, fundsOutputID, issuerResp)

	blkID := d.SubmitPayload(ctx, signedTx, wallet.NewEd25519Account(to.AccountID, to.BlockIssuerKey), congestionResp, issuerResp)

	fmt.Println("Allot mana transaction sent, blkID:", blkID.ToHex(), ", txID:", lo.PanicOnErr(signedTx.Transaction.ID()).ToHex(), ", slot:", blkID.Slot())

	d.AwaitTransactionPayloadAccepted(ctx, blkID)

	// wait until BIC is updated
	d.AwaitCommitment(blkID.Slot())
}

// AllotManaTo requests faucet funds then uses it to allots mana from one account to another.
func (d *DockerTestFramework) AllotManaTo(fromId iotago.AccountID, toId iotago.AccountID, manaToAllot iotago.Mana) {
	from := d.defaultWallet.Account(fromId)
	// requesting faucet funds for allotment
	ctx := context.TODO()
	fundsOutputID := d.RequestFaucetFunds(ctx, iotago.AddressEd25519)
	clt := d.Node("V1").Client

	issuerResp, err := clt.BlockIssuance(ctx)
	require.NoError(d.Testing, err)

	congestionResp, err := clt.Congestion(ctx, from.AccountAddress, 0, lo.PanicOnErr(issuerResp.LatestCommitment.ID()))
	require.NoError(d.Testing, err)

	signedTx := d.defaultWallet.AllotManaFromAccount(clt, fromId, toId, manaToAllot, fundsOutputID, issuerResp)

	blkID := d.SubmitPayload(ctx, signedTx, wallet.NewEd25519Account(from.AccountID, from.BlockIssuerKey), congestionResp, issuerResp)

	fmt.Println("Allot mana transaction sent, blkID:", blkID.ToHex(), ", txID:", lo.PanicOnErr(signedTx.Transaction.ID()).ToHex(), ", slot:", blkID.Slot())

	d.AwaitTransactionPayloadAccepted(ctx, blkID)
}

// CreateNativeToken request faucet funds then use it to create native token for the account, and returns the updated Account.
func (d *DockerTestFramework) CreateNativeToken(fromId iotago.AccountID, mintedAmount iotago.BaseToken, maxSupply iotago.BaseToken) {
	require.GreaterOrEqual(d.Testing, maxSupply, mintedAmount)
	from := d.defaultWallet.Account(fromId)

	// requesting faucet funds for native token creation
	ctx := context.TODO()
	fundsOutputID := d.RequestFaucetFunds(ctx, iotago.AddressEd25519)

	clt := d.Node("V1").Client

	issuerResp, err := clt.BlockIssuance(ctx)
	require.NoError(d.Testing, err)

	congestionResp, err := clt.Congestion(ctx, from.AccountAddress, 0, lo.PanicOnErr(issuerResp.LatestCommitment.ID()))
	require.NoError(d.Testing, err)

	signedTx := d.defaultWallet.CreateFoundryAndNativeTokensFromInput(clt, fromId, fundsOutputID, mintedAmount, maxSupply, issuerResp)

	blkID := d.SubmitPayload(ctx, signedTx, wallet.NewEd25519Account(from.AccountID, from.BlockIssuerKey), congestionResp, issuerResp)

	d.AwaitTransactionPayloadAccepted(ctx, blkID)

	fmt.Println("Create native tokens transaction sent, blkID:", blkID.ToHex(), ", txID:", lo.PanicOnErr(signedTx.Transaction.ID()).ToHex(), ", slot:", blkID.Slot())

	// wait for the account to be committed
	d.AwaitCommitment(blkID.Slot())

	from = d.defaultWallet.Account(fromId)
	d.AssertIndexerAccount(from)
	d.AssertIndexerFoundry(signedTx.Transaction.Outputs[1].(*iotago.FoundryOutput).MustFoundryID())
}

func (d *DockerTestFramework) RequestFaucetFunds(ctx context.Context, addressType iotago.AddressType) iotago.OutputID {
	privateKey, _ := d.defaultWallet.KeyPair()
	var address iotago.Address
	if addressType == iotago.AddressImplicitAccountCreation {
		address = d.defaultWallet.ImplicitAccountCreationAddress()
	} else {
		address = d.defaultWallet.Address()
	}

	d.SendFaucetRequest(ctx, address)

	outputID, output, err := d.AwaitAddressUnspentOutputAccepted(ctx, address)
	require.NoError(d.Testing, err)

	d.defaultWallet.AddOutput(outputID, &Output{
		ID:         outputID,
		Address:    address,
		PrivateKey: privateKey,
		Output:     output,
	})

	fmt.Println("Faucet funds received, txID:", outputID.TransactionID().ToHex(), ", amount:", output.BaseTokenAmount(), ", mana:", output.StoredMana())

	return outputID
}

func (d *DockerTestFramework) Stop() {
	fmt.Println("Stop the network...")
	defer fmt.Println("Stop the network.....done")

	_ = exec.Command("docker", "compose", "down").Run()
	_ = exec.Command("rm", d.snapshotPath).Run()
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

func (d *DockerTestFramework) CreateBlock(ctx context.Context, payload iotago.Payload, issuer wallet.Account, congestionResp *api.CongestionResponse, issuerResp *api.IssuanceBlockHeaderResponse) *iotago.Block {
	clt := d.Node("V1").Client
	issuingTime := time.Now()
	apiForSlot := clt.APIForSlot(clt.LatestAPI().TimeProvider().SlotFromTime(issuingTime))
	blockBuilder := builder.NewBasicBlockBuilder(apiForSlot)

	commitmentID, err := issuerResp.LatestCommitment.ID()
	require.NoError(d.Testing, err)

	blockBuilder.ProtocolVersion(apiForSlot.Version()).
		SlotCommitmentID(commitmentID).
		LatestFinalizedSlot(issuerResp.LatestFinalizedSlot).
		IssuingTime(issuingTime).
		StrongParents(issuerResp.StrongParents).
		WeakParents(issuerResp.WeakParents).
		ShallowLikeParents(issuerResp.ShallowLikeParents).
		Payload(payload).
		CalculateAndSetMaxBurnedMana(congestionResp.ReferenceManaCost).
		Sign(issuer.Address().AccountID(), issuer.PrivateKey())

	blk, err := blockBuilder.Build()
	require.NoError(d.Testing, err)

	return blk
}

func (d *DockerTestFramework) SubmitBlock(ctx context.Context, blk *iotago.Block) {
	clt := d.Node("V1").Client

	_, err := clt.SubmitBlock(ctx, blk)
	require.NoError(d.Testing, err)
}

func (d *DockerTestFramework) SubmitPayload(ctx context.Context, payload iotago.Payload, issuer wallet.Account, congestionResp *api.CongestionResponse, issuerResp *api.IssuanceBlockHeaderResponse) iotago.BlockID {
	clt := d.Node("V1").Client

	blk := d.CreateBlock(ctx, payload, issuer, congestionResp, issuerResp)

	blkID, err := clt.SubmitBlock(ctx, blk)
	require.NoError(d.Testing, err)

	return blkID
}
