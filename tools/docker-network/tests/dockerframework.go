//go:build dockertests

package tests

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/testsuite/snapshotcreator"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/nodeclient"
	"github.com/iotaledger/iota.go/v4/tpkg"
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
	Name                 string
	ContainerName        string
	ClientURL            string
	Client               *nodeclient.Client
	AccountAddressBech32 string
	ContainerConfigs     string
	PrivateKey           string
}

type Account struct {
	AccountID      iotago.AccountID
	AccountAddress *iotago.AccountAddress
	BlockIssuerKey ed25519.PrivateKey
}

type DockerTestFramework struct {
	Testing *testing.T

	nodes map[string]*Node

	snapshotPath     string
	logDirectoryPath string
	seed             [32]byte
	latestUsedIndex  atomic.Uint32

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
		seed:            tpkg.RandEd25519Seed(),
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

func (d *DockerTestFramework) Run() error {
	ch := make(chan error)
	stopCh := make(chan struct{})
	defer close(ch)
	defer close(stopCh)

	go func() {
		cmd := exec.Command("docker", "compose", "up")
		var out strings.Builder
		cmd.Stderr = &out
		err := cmd.Run()

		if err != nil {
			fmt.Println("Docker compose up failed with error:", err, ":", out.String())
		}

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
	for _, node := range d.Nodes() {
		client, err := nodeclient.New(node.ClientURL)
		if err != nil {
			return ierrors.Wrapf(err, "failed to create node client for node %s", node.Name)
		}

		node.Client = client
		d.nodes[node.Name] = node
	}

	return nil
}

func (d *DockerTestFramework) WaitUntilSync() error {
	fmt.Println("Wait until the nodes are synced...")
	defer fmt.Println("Wait until the nodes are synced......done")

	d.Eventually(func() error {
		for _, node := range d.nodes {
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

	return nil
}

func (d *DockerTestFramework) AddValidatorNode(name string, containerName string, clientURL string, accAddrBech32 string) {
	d.nodes[name] = &Node{
		Name:                 name,
		ContainerName:        containerName,
		ClientURL:            clientURL,
		AccountAddressBech32: accAddrBech32,
	}
}

func (d *DockerTestFramework) AddNode(name string, containerName string, clientURL string) {
	d.nodes[name] = &Node{
		Name:          name,
		ContainerName: containerName,
		ClientURL:     clientURL,
	}
}

func (d *DockerTestFramework) Nodes(names ...string) []*Node {
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

func (d *DockerTestFramework) StopIssueCandidacyPayload(nodes ...*Node) {
	// build a new image from the current one so we could set IssueCandidacyPayload to false,
	// the committed image will not remember the container configs, so it's fine to commit the first validator of the nodes
	newImageName := "no-candidacy-payload-image"
	err := exec.Command("docker", "commit", nodes[0].ContainerName, newImageName).Run()
	require.NoError(d.Testing, err)

	for _, node := range nodes {
		if node.AccountAddressBech32 == "" {
			continue
		}

		// stop the inx-validator that issues candidacy payload
		err = d.StopContainer(node.ContainerName)
		require.NoError(d.Testing, err)

		// start a new inx-validator that does not issue candidacy payload
		newContainerName := fmt.Sprintf("%s-1", node.ContainerName)
		cmd := fmt.Sprintf("docker run --network docker-network_iota-core --env %s --name %s  %s %s --validator.issueCandidacyPayload=false &", node.PrivateKey, newContainerName, newImageName, node.ContainerConfigs)
		err = exec.Command("bash", "-c", cmd).Run()
		require.NoError(d.Testing, err)
	}
}

func (d *DockerTestFramework) StartIssueCandidacyPayload(nodes ...*Node) {
	for _, node := range nodes {
		if node.AccountAddressBech32 == "" {
			continue
		}

		// stop and remove the inx-validator that does not issue candidacy payload
		newContainerName := fmt.Sprintf("%s-1", node.ContainerName)
		err := d.StopContainer(newContainerName)
		require.NoError(d.Testing, err)

		err = exec.Command("docker", "rm", newContainerName).Run()
		require.NoError(d.Testing, err)

		// start the inx-validator that issues candidacy payload
		err = d.RestartContainer(node.ContainerName)
		require.NoError(d.Testing, err)
	}
}

func (d *DockerTestFramework) CreateAccount(opts ...options.Option[builder.AccountOutputBuilder]) *Account {
	// create implicit account by requesting faucet funds
	ctx := context.TODO()
	receiverAddr, implicitPrivateKey := d.getAddress(iotago.AddressImplicitAccountCreation)
	implicitOutputID, implicitAccountOutput := d.RequestFaucetFunds(ctx, receiverAddr)

	accountID := iotago.AccountIDFromOutputID(implicitOutputID)
	accountAddress, ok := accountID.ToAddress().(*iotago.AccountAddress)
	require.True(d.Testing, ok)

	// make sure implicit account is committed
	d.CheckAccountStatus(ctx, iotago.EmptyBlockID, implicitOutputID.TransactionID(), implicitOutputID, accountAddress)

	// transition to full account with new Ed25519 address and staking feature
	accEd25519Addr, accPrivateKey := d.getAddress(iotago.AddressEd25519)
	accBlockIssuerKey := iotago.Ed25519PublicKeyHashBlockIssuerKeyFromPublicKey(accPrivateKey.Public().(ed25519.PublicKey))
	accountOutput := options.Apply(builder.NewAccountOutputBuilder(accEd25519Addr, implicitAccountOutput.BaseTokenAmount()),
		opts, func(b *builder.AccountOutputBuilder) {
			b.AccountID(accountID).
				BlockIssuer(iotago.NewBlockIssuerKeys(accBlockIssuerKey), iotago.MaxSlotIndex)
		}).MustBuild()

	clt := d.Node("V1").Client
	currentSlot := clt.LatestAPI().TimeProvider().SlotFromTime(time.Now())
	apiForSlot := clt.APIForSlot(currentSlot)

	issuerResp, err := clt.BlockIssuance(ctx)
	require.NoError(d.Testing, err)

	congestionResp, err := clt.Congestion(ctx, accountAddress, lo.PanicOnErr(issuerResp.LatestCommitment.ID()))
	require.NoError(d.Testing, err)

	implicitAddrSigner := iotago.NewInMemoryAddressSigner(iotago.NewAddressKeysForImplicitAccountCreationAddress(receiverAddr.(*iotago.ImplicitAccountCreationAddress), implicitPrivateKey))
	signedTx, err := builder.NewTransactionBuilder(apiForSlot).
		AddInput(&builder.TxInput{
			UnlockTarget: receiverAddr,
			InputID:      implicitOutputID,
			Input:        implicitAccountOutput,
		}).
		AddOutput(accountOutput).
		SetCreationSlot(currentSlot).
		AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: lo.Return1(issuerResp.LatestCommitment.ID())}).
		AddBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{AccountID: accountID}).
		WithTransactionCapabilities(iotago.TransactionCapabilitiesBitMaskWithCapabilities(iotago.WithTransactionCanDoAnything())).
		AllotAllMana(currentSlot, accountID).
		Build(implicitAddrSigner)
	require.NoError(d.Testing, err)

	blkID := d.SubmitPayload(ctx, signedTx, wallet.NewEd25519Account(accountID, implicitPrivateKey), congestionResp, issuerResp)

	// check if the account is committed
	accOutputID := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0)
	d.CheckAccountStatus(ctx, blkID, lo.PanicOnErr(signedTx.Transaction.ID()), accOutputID, accountAddress, true)

	return &Account{
		AccountID:      accountID,
		AccountAddress: accountAddress,
		BlockIssuerKey: accPrivateKey,
	}
}

// DelegateToValidator requests faucet funds and delegate the UTXO output to the validator.
func (d *DockerTestFramework) DelegateToValidator(from *Account, validator *Node) iotago.EpochIndex {
	// requesting faucet funds as delegation input
	ctx := context.TODO()
	fundsAddr, privateKey := d.getAddress(iotago.AddressEd25519)
	fundsOutputID, fundsUTXOOutput := d.RequestFaucetFunds(ctx, fundsAddr)
	fundsAddrSigner := iotago.NewInMemoryAddressSigner(iotago.NewAddressKeysForEd25519Address(fundsAddr.(*iotago.Ed25519Address), privateKey))

	_, validatorAccountAddr, err := iotago.ParseBech32(validator.AccountAddressBech32)
	require.NoError(d.Testing, err)

	clt := validator.Client
	currentSlot := clt.LatestAPI().TimeProvider().SlotFromTime(time.Now())
	apiForSlot := clt.APIForSlot(currentSlot)

	// construct delegation transaction
	issuerResp, err := clt.BlockIssuance(ctx)
	require.NoError(d.Testing, err)

	congestionResp, err := clt.Congestion(ctx, from.AccountAddress, lo.PanicOnErr(issuerResp.LatestCommitment.ID()))
	require.NoError(d.Testing, err)

	delegationOutput := builder.NewDelegationOutputBuilder(validatorAccountAddr.(*iotago.AccountAddress), fundsAddr, fundsUTXOOutput.BaseTokenAmount()).
		StartEpoch(getDelegationStartEpoch(apiForSlot, issuerResp.LatestCommitment.Slot)).
		DelegatedAmount(fundsUTXOOutput.BaseTokenAmount()).MustBuild()

	signedTx, err := builder.NewTransactionBuilder(apiForSlot).
		AddInput(&builder.TxInput{
			UnlockTarget: fundsAddr,
			InputID:      fundsOutputID,
			Input:        fundsUTXOOutput,
		}).
		AddOutput(delegationOutput).
		SetCreationSlot(currentSlot).
		AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: lo.Return1(issuerResp.LatestCommitment.ID())}).
		WithTransactionCapabilities(iotago.TransactionCapabilitiesBitMaskWithCapabilities(iotago.WithTransactionCanDoAnything())).
		AllotAllMana(currentSlot, from.AccountID).
		Build(fundsAddrSigner)
	require.NoError(d.Testing, err)

	blkID := d.SubmitPayload(ctx, signedTx, wallet.NewEd25519Account(from.AccountID, from.BlockIssuerKey), congestionResp, issuerResp)

	d.AwaitTransactionPayloadAccepted(ctx, blkID)

	return delegationOutput.StartEpoch
}

func (d *DockerTestFramework) CheckAccountStatus(ctx context.Context, blkID iotago.BlockID, txID iotago.TransactionID, creationOutputID iotago.OutputID, accountAddress *iotago.AccountAddress, checkIndexer ...bool) {
	// request by blockID if provided, otherwise use txID
	// we take the slot from the blockID in case the tx is created earlier than the block.
	clt := d.Node("V1").Client
	slot := blkID.Slot()

	if blkID == iotago.EmptyBlockID {
		blkMetadata, err := clt.TransactionIncludedBlockMetadata(ctx, txID)
		require.NoError(d.Testing, err)

		blkID = blkMetadata.BlockID
		slot = blkMetadata.BlockID.Slot()
	}

	d.AwaitTransactionPayloadAccepted(ctx, blkID)

	// wait for the account to be committed
	d.AwaitCommitment(slot)

	// Check the indexer
	if len(checkIndexer) > 0 && checkIndexer[0] {
		indexerClt, err := d.Node("V1").Client.Indexer(ctx)
		require.NoError(d.Testing, err)

		_, _, _, err = indexerClt.Account(ctx, accountAddress)
		require.NoError(d.Testing, err)
	}

	// check if the creation output exists
	_, err := clt.OutputByID(ctx, creationOutputID)
	require.NoError(d.Testing, err)

	fmt.Printf("Account created, Bech addr: %s, slot: %d\n", accountAddress.Bech32(clt.CommittedAPI().ProtocolParameters().Bech32HRP()), slot)
}

func (d *DockerTestFramework) RequestFaucetFunds(ctx context.Context, receiveAddr iotago.Address) (iotago.OutputID, iotago.Output) {
	d.SendFaucetRequest(ctx, receiveAddr)

	outputID, output, err := d.AwaitAddressUnspentOutputAccepted(ctx, receiveAddr)
	require.NoError(d.Testing, err)

	return outputID, output
}

func (d *DockerTestFramework) AssertValidatorExists(accountAddr *iotago.AccountAddress) {
	d.Eventually(func() error {
		for _, node := range d.nodes {
			_, err := node.Client.StakingAccount(context.TODO(), accountAddr)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (d *DockerTestFramework) AssertCommittee(expectedEpoch iotago.EpochIndex, expectedCommitteeMember []string) {
	fmt.Println("Wait for committee selection..., expected epoch: ", expectedEpoch, ", expected committee size: ", len(expectedCommitteeMember))
	defer fmt.Println("Wait for committee selection......done")

	sort.Strings(expectedCommitteeMember)

	status := d.NodeStatus("V1")
	api := d.Node("V1").Client.CommittedAPI()
	expectedSlotStart := api.TimeProvider().EpochStart(expectedEpoch)
	require.Greater(d.Testing, expectedSlotStart, status.LatestAcceptedBlockSlot)

	slotToWait := expectedSlotStart - status.LatestAcceptedBlockSlot
	secToWait := time.Duration(slotToWait) * time.Duration(api.ProtocolParameters().SlotDurationInSeconds()) * time.Second
	fmt.Println("Wait for ", secToWait, "until expected epoch: ", expectedEpoch)
	time.Sleep(secToWait)

	d.Eventually(func() error {
		for _, node := range d.nodes {
			resp, err := node.Client.Committee(context.TODO())
			if err != nil {
				return err
			}

			if resp.Epoch == expectedEpoch {
				members := make([]string, len(resp.Committee))
				for i, member := range resp.Committee {
					members[i] = member.AddressBech32
				}

				sort.Strings(members)
				if match := lo.Equal(expectedCommitteeMember, members); match {
					return nil
				}

				return ierrors.Errorf("committee members does not match as expected, expected: %v, actual: %v", expectedCommitteeMember, members)
			}
		}

		return nil
	})
}

func (d *DockerTestFramework) AssertFinalizedSlot(condition func(iotago.SlotIndex) error) {
	for _, node := range d.nodes {
		status := d.NodeStatus(node.Name)

		err := condition(status.LatestFinalizedSlot)
		require.NoError(d.Testing, err)
	}
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
		logCmd := fmt.Sprintf("docker logs -f %s > %s 2>&1 &", name, filePath)
		err := exec.Command("bash", "-c", logCmd).Run()
		require.NoError(d.Testing, err)
	}
}

func (d *DockerTestFramework) GetContainersConfigs() {
	// get container configs
	for _, node := range d.Nodes() {
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

func (d *DockerTestFramework) SubmitPayload(ctx context.Context, payload iotago.Payload, issuer wallet.Account, congestionResp *api.CongestionResponse, issuerResp *api.IssuanceBlockHeaderResponse) iotago.BlockID {
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

	blkID, err := clt.SubmitBlock(ctx, blk)
	require.NoError(d.Testing, err)

	return blkID
}
