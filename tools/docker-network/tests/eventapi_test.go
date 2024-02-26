//go:build dockertests

package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/stretchr/testify/require"
)

var eventAPITests = map[string]func(t *testing.T, d *DockerTestFramework){
	"Test_Commitments":                 test_Commitments,
	"Test_ValidationBlocks":            test_ValidationBlocks,
	"Test_BasicTaggedDataBlocks":       test_BasicTaggedDataBlocks,
	"Test_DelegationTransactionBlocks": test_DelegationTransactionBlocks,
	"Test_AccountTransactionBlocks":    test_AccountTransactionBlocks,
	"Test_FoundryTransactionBlocks":    test_FoundryTransactionBlocks,
	"Test_NFTTransactionBlocks":        test_NFTTransactionBlocks,
}

func Test_MQTTTopics(t *testing.T) {
	d := NewDockerTestFramework(t,
		WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 4),
			iotago.WithLivenessOptions(10, 10, 2, 4, 8),
		))
	defer d.Stop()

	d.AddValidatorNode("V1", "docker-network-inx-validator-1-1", "http://localhost:8050", "rms1pzg8cqhfxqhq7pt37y8cs4v5u4kcc48lquy2k73ehsdhf5ukhya3y5rx2w6")
	d.AddValidatorNode("V2", "docker-network-inx-validator-2-1", "http://localhost:8060", "rms1pqm4xk8e9ny5w5rxjkvtp249tfhlwvcshyr3pc0665jvp7g3hc875k538hl")
	d.AddValidatorNode("V3", "docker-network-inx-validator-3-1", "http://localhost:8070", "rms1pp4wuuz0y42caz48vv876qfpmffswsvg40zz8v79sy8cp0jfxm4kunflcgt")
	d.AddValidatorNode("V4", "docker-network-inx-validator-4-1", "http://localhost:8040", "rms1pr8cxs3dzu9xh4cduff4dd4cxdthpjkpwmz2244f75m0urslrsvtsshrrjw")
	d.AddNode("node5", "docker-network-node-5-1", "http://localhost:8090")

	err := d.Run()
	require.NoError(t, err)

	d.WaitUntilNetworkReady()

	for name, test := range eventAPITests {
		t.Run(name, func(t *testing.T) {
			test(t, d)
		})
	}
}

func test_Commitments(t *testing.T, d *DockerTestFramework) {
	finish := make(chan struct{})

	// get event API client ready
	clt := d.Node("V1").Client
	ctx, cancel := context.WithCancel(context.Background())
	eventClt, err := clt.EventAPI(ctx)
	require.NoError(t, err)
	err = eventClt.Connect(ctx)
	require.NoError(t, err)
	defer eventClt.Close()

	infoResp, err := clt.Info(ctx)
	require.NoError(t, err)

	// prepare the expected commitments to be received
	expectedLatestSlots := make([]iotago.SlotIndex, 0)
	for i := infoResp.Status.LatestCommitmentID.Slot() + 2; i < infoResp.Status.LatestCommitmentID.Slot()+6; i++ {
		expectedLatestSlots = append(expectedLatestSlots, iotago.SlotIndex(i))
	}

	expectedFinalizedSlots := make([]iotago.SlotIndex, 0)
	for i := infoResp.Status.LatestFinalizedSlot + 2; i < infoResp.Status.LatestFinalizedSlot+6; i++ {
		expectedFinalizedSlots = append(expectedFinalizedSlots, iotago.SlotIndex(i))
	}

	totalTopics := 2

	d.AssertLatestCommitments(ctx, eventClt, expectedLatestSlots, finish)
	d.AssertFinalizedCommitments(ctx, eventClt, expectedFinalizedSlots, finish)

	// wait until all topics receives all expected objects
	err = AwaitEventAPITopics(t, d.optsWaitFor, cancel, finish, totalTopics)
	require.NoError(t, err)
}

func test_ValidationBlocks(t *testing.T, d *DockerTestFramework) {
	finish := make(chan struct{})

	// get event API client ready
	clt := d.Node("V1").Client
	ctx, cancel := context.WithCancel(context.Background())
	eventClt, err := clt.EventAPI(ctx)
	require.NoError(t, err)
	err = eventClt.Connect(ctx)
	require.NoError(t, err)
	defer eventClt.Close()

	// prepare the expected commitments to be received
	validators := make(map[string]struct{}, 0)
	nodes := d.Nodes("V1", "V2", "V3", "V4")
	for _, node := range nodes {
		validators[node.AccountAddressBech32] = struct{}{}
	}

	totalTopics := 1

	d.AssertValidationBlocks(ctx, eventClt, clt.CommittedAPI().ProtocolParameters().Bech32HRP(), validators, finish)

	// wait until all topics receives all expected objects
	err = AwaitEventAPITopics(t, d.optsWaitFor, cancel, finish, totalTopics)
	require.NoError(t, err)
}

func test_BasicTaggedDataBlocks(t *testing.T, d *DockerTestFramework) {
	// get event API client ready
	clt := d.Node("V1").Client
	ctx, cancel := context.WithCancel(context.Background())
	eventClt, err := clt.EventAPI(ctx)
	require.NoError(t, err)
	err = eventClt.Connect(ctx)
	require.NoError(t, err)
	defer eventClt.Close()

	// create an account to issue blocks
	account := d.CreateAccount()

	// prepare data blocks to send
	expectedBlocks := make(map[string]*iotago.Block, 0)
	for i := 0; i < 10; i++ {
		blk := d.CreateTaggedDataBlock(account.ID, []byte("tag"))
		expectedBlocks[blk.MustID().ToHex()] = blk
	}
	finish := make(chan struct{})
	totalTopics := 6

	d.AssertBlocks(ctx, eventClt, expectedBlocks, finish)
	d.AssertBasicBlocks(ctx, eventClt, expectedBlocks, finish)
	d.AssertTaggedDataBlocks(ctx, eventClt, expectedBlocks, finish)
	d.AssertTaggedDataBlocksByTag(ctx, eventClt, expectedBlocks, []byte("tag"), finish)
	d.AssertBlockMetadataAcceptedBlocks(ctx, eventClt, expectedBlocks, finish)
	d.AssertBlockMetadataConfirmedBlocks(ctx, eventClt, expectedBlocks, finish)

	// wait until all topics starts listening
	err = AwaitEventAPITopics(t, d.optsWaitFor, cancel, finish, totalTopics)
	require.NoError(t, err)

	// issue blocks
	go func() {
		for _, blk := range expectedBlocks {
			fmt.Println("submitting a block")
			d.SubmitBlock(context.Background(), blk)
		}
	}()

	// wait until all topics receives all expected objects
	err = AwaitEventAPITopics(t, d.optsWaitFor, cancel, finish, totalTopics)
	require.NoError(t, err)
}

func test_DelegationTransactionBlocks(t *testing.T, d *DockerTestFramework) {
	// get event API client ready
	clt := d.Node("V1").Client
	ctx, cancel := context.WithCancel(context.Background())
	eventClt, err := clt.EventAPI(ctx)
	require.NoError(t, err)
	err = eventClt.Connect(ctx)
	require.NoError(t, err)
	defer eventClt.Close()

	// create an account to issue blocks
	account := d.CreateAccount()
	fundsOutputID := d.RequestFaucetFunds(ctx, iotago.AddressEd25519)

	// prepare data blocks to send
	delegationId, outputId, blk := d.CreateDelegationBlockFromInput(account.ID, d.Node("V2"), fundsOutputID)
	expectedBlocks := map[string]*iotago.Block{
		blk.MustID().ToHex(): blk,
	}
	finish := make(chan struct{})
	totalTopics := 8

	d.AssertTransactionBlocks(ctx, eventClt, expectedBlocks, finish)
	d.AssertBasicBlocks(ctx, eventClt, expectedBlocks, finish)
	d.AssertBlockMetadataAcceptedBlocks(ctx, eventClt, expectedBlocks, finish)
	d.AssertBlockMetadataConfirmedBlocks(ctx, eventClt, expectedBlocks, finish)

	// d.AssertTransactionMetadataByTransactionID(ctx, eventClt, outputId.TransactionID(), finish)
	// d.AssertTransactionMetadataIncludedBlocks(ctx, eventClt, outputId.TransactionID(), finish)

	d.AssertDelegationOutput(ctx, eventClt, delegationId, finish)
	d.AssertOutput(ctx, eventClt, outputId, finish)

	delegationOutput := d.wallet.Output(outputId)
	d.AssertOutputsWithMetadataByUnlockConditionAndAddress(ctx, eventClt, api.EventAPIUnlockConditionAny, delegationOutput.Address, finish)
	d.AssertOutputsWithMetadataByUnlockConditionAndAddress(ctx, eventClt, api.EventAPIUnlockConditionAddress, delegationOutput.Address, finish)

	// wait until all topics starts listening
	err = AwaitEventAPITopics(t, d.optsWaitFor, cancel, finish, totalTopics)
	require.NoError(t, err)

	// issue blocks
	go func() {
		for _, blk := range expectedBlocks {
			fmt.Println("submitting a block")
			d.SubmitBlock(context.Background(), blk)
		}
	}()

	// wait until all topics receives all expected objects
	err = AwaitEventAPITopics(t, d.optsWaitFor, cancel, finish, totalTopics)
	require.NoError(t, err)
}

func test_AccountTransactionBlocks(t *testing.T, d *DockerTestFramework) {
	// get event API client ready
	clt := d.Node("V1").Client
	ctx, cancel := context.WithCancel(context.Background())
	eventClt, err := clt.EventAPI(ctx)
	require.NoError(t, err)
	err = eventClt.Connect(ctx)
	require.NoError(t, err)
	defer eventClt.Close()

	// implicit account transition
	{
		implicitAccount := d.CreateImplicitAccount(ctx)

		// prepare account transaction block to send
		account, outputId, blk := d.CreateAccountBlockFromInput(implicitAccount.OutputID)
		expectedBlocks := map[string]*iotago.Block{
			blk.MustID().ToHex(): blk,
		}
		finish := make(chan struct{})
		totalTopics := 8

		d.AssertTransactionBlocks(ctx, eventClt, expectedBlocks, finish)
		d.AssertBasicBlocks(ctx, eventClt, expectedBlocks, finish)
		d.AssertBlockMetadataAcceptedBlocks(ctx, eventClt, expectedBlocks, finish)
		d.AssertBlockMetadataConfirmedBlocks(ctx, eventClt, expectedBlocks, finish)

		// d.AssertTransactionMetadataByTransactionID(ctx, eventClt, outputId.TransactionID(), finish)
		// d.AssertTransactionMetadataIncludedBlocks(ctx, eventClt, outputId.TransactionID(), finish)

		d.AssertAccountOutput(ctx, eventClt, account.ID, finish)
		d.AssertOutput(ctx, eventClt, outputId, finish)

		accountOutput := d.wallet.Output(outputId)
		d.AssertOutputsWithMetadataByUnlockConditionAndAddress(ctx, eventClt, api.EventAPIUnlockConditionAny, accountOutput.Address, finish)
		d.AssertOutputsWithMetadataByUnlockConditionAndAddress(ctx, eventClt, api.EventAPIUnlockConditionAddress, accountOutput.Address, finish)

		// wait until all topics starts listening
		err = AwaitEventAPITopics(t, d.optsWaitFor, cancel, finish, totalTopics)
		require.NoError(t, err)

		// issue blocks
		go func() {
			for _, blk := range expectedBlocks {
				fmt.Println("submitting a block")
				d.SubmitBlock(context.Background(), blk)
			}
		}()

		// wait until all topics receives all expected objects
		err = AwaitEventAPITopics(t, d.optsWaitFor, cancel, finish, totalTopics)
		require.NoError(t, err)
	}
}

func test_FoundryTransactionBlocks(t *testing.T, d *DockerTestFramework) {
	// get event API client ready
	clt := d.Node("V1").Client
	ctx, cancel := context.WithCancel(context.Background())
	eventClt, err := clt.EventAPI(ctx)
	require.NoError(t, err)
	err = eventClt.Connect(ctx)
	require.NoError(t, err)
	defer eventClt.Close()

	{
		account := d.CreateAccount()
		fundsOutputID := d.RequestFaucetFunds(ctx, iotago.AddressEd25519)

		// prepare foundry output block
		foundryId, outputId, blk := d.CreateFoundryBlockFromInput(account.ID, fundsOutputID, 5_000_000, 10_000_000_000)
		expectedBlocks := map[string]*iotago.Block{
			blk.MustID().ToHex(): blk,
		}
		finish := make(chan struct{})
		totalTopics := 10

		d.AssertTransactionBlocks(ctx, eventClt, expectedBlocks, finish)
		d.AssertBasicBlocks(ctx, eventClt, expectedBlocks, finish)
		d.AssertBlockMetadataAcceptedBlocks(ctx, eventClt, expectedBlocks, finish)
		d.AssertBlockMetadataConfirmedBlocks(ctx, eventClt, expectedBlocks, finish)

		// d.AssertTransactionMetadataByTransactionID(ctx, eventClt, outputId.TransactionID(), finish)
		// d.AssertTransactionMetadataIncludedBlocks(ctx, eventClt, outputId.TransactionID(), finish)

		d.AssertAccountOutput(ctx, eventClt, account.ID, finish)
		d.AssertFoundryOutput(ctx, eventClt, foundryId, finish)
		d.AssertOutput(ctx, eventClt, outputId, finish)
		d.AssertOutput(ctx, eventClt, account.OutputID, finish)

		foundryOutput := d.wallet.Output(outputId)
		d.AssertOutputsWithMetadataByUnlockConditionAndAddress(ctx, eventClt, api.EventAPIUnlockConditionAny, foundryOutput.Address, finish)
		d.AssertOutputsWithMetadataByUnlockConditionAndAddress(ctx, eventClt, api.EventAPIUnlockConditionImmutableAccount, foundryOutput.Address, finish)

		// wait until all topics starts listening
		err = AwaitEventAPITopics(t, d.optsWaitFor, cancel, finish, totalTopics)
		require.NoError(t, err)

		// issue blocks
		go func() {
			for _, blk := range expectedBlocks {
				fmt.Println("submitting a block")
				d.SubmitBlock(context.Background(), blk)
			}
		}()

		// wait until all topics receives all expected objects
		err = AwaitEventAPITopics(t, d.optsWaitFor, cancel, finish, totalTopics)
		require.NoError(t, err)
	}
}

func test_NFTTransactionBlocks(t *testing.T, d *DockerTestFramework) {
	// get event API client ready
	clt := d.Node("V1").Client
	ctx, cancel := context.WithCancel(context.Background())
	eventClt, err := clt.EventAPI(ctx)
	require.NoError(t, err)
	err = eventClt.Connect(ctx)
	require.NoError(t, err)
	defer eventClt.Close()

	{
		account := d.CreateAccount()
		fundsOutputID := d.RequestFaucetFunds(ctx, iotago.AddressEd25519)

		// prepare foundry output block
		nftId, outputId, blk := d.CreateNFTBlockFromInput(account.ID, fundsOutputID)
		expectedBlocks := map[string]*iotago.Block{
			blk.MustID().ToHex(): blk,
		}
		finish := make(chan struct{})
		totalTopics := 10

		d.AssertTransactionBlocks(ctx, eventClt, expectedBlocks, finish)
		d.AssertBasicBlocks(ctx, eventClt, expectedBlocks, finish)
		d.AssertBlockMetadataAcceptedBlocks(ctx, eventClt, expectedBlocks, finish)
		d.AssertBlockMetadataConfirmedBlocks(ctx, eventClt, expectedBlocks, finish)

		// d.AssertTransactionMetadataByTransactionID(ctx, eventClt, outputId.TransactionID(), finish)
		// d.AssertTransactionMetadataIncludedBlocks(ctx, eventClt, outputId.TransactionID(), finish)

		d.AssertAccountOutput(ctx, eventClt, account.ID, finish)
		d.AssertNFTOutput(ctx, eventClt, nftId, finish)
		d.AssertOutput(ctx, eventClt, outputId, finish)

		nftOutput := d.wallet.Output(outputId)
		d.AssertOutputsWithMetadataByUnlockConditionAndAddress(ctx, eventClt, api.EventAPIUnlockConditionAny, nftOutput.Address, finish)
		d.AssertOutputsWithMetadataByUnlockConditionAndAddress(ctx, eventClt, api.EventAPIUnlockConditionAddress, nftOutput.Address, finish)

		// wait until all topics starts listening
		err = AwaitEventAPITopics(t, d.optsWaitFor, cancel, finish, totalTopics)
		require.NoError(t, err)

		// issue blocks
		go func() {
			for _, blk := range expectedBlocks {
				fmt.Println("submitting a block")
				d.SubmitBlock(context.Background(), blk)
			}
		}()

		// wait until all topics receives all expected objects
		err = AwaitEventAPITopics(t, d.optsWaitFor, cancel, finish, totalTopics)
		require.NoError(t, err)
	}
}
