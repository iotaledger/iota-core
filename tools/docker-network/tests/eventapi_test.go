//go:build dockertests

package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

var eventAPITests = map[string]func(t *testing.T, e *EventAPIDockerTestFramework){
	"Test_Commitments":                 test_Commitments,
	"Test_ValidationBlocks":            test_ValidationBlocks,
	"Test_BasicTaggedDataBlocks":       test_BasicTaggedDataBlocks,
	"Test_DelegationTransactionBlocks": test_DelegationTransactionBlocks,
	"Test_AccountTransactionBlocks":    test_AccountTransactionBlocks,
	"Test_FoundryTransactionBlocks":    test_FoundryTransactionBlocks,
	"Test_NFTTransactionBlocks":        test_NFTTransactionBlocks,
	"Test_BlockMetadataMatchedCoreAPI": test_BlockMetadataMatchedCoreAPI,
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

	e := NewEventAPIDockerTestFramework(t, d)

	for name, test := range eventAPITests {
		t.Run(name, func(t *testing.T) {
			test(t, e)
		})
	}
}

func test_Commitments(t *testing.T, e *EventAPIDockerTestFramework) {

	// get event API client ready
	ctx, cancel := context.WithCancel(context.Background())
	eventClt := e.ConnectEventAPIClient(ctx)
	defer eventClt.Close()

	infoResp, err := e.DefaultClient.Info(ctx)
	require.NoError(t, err)

	// prepare the expected commitments to be received
	expectedLatestSlots := make([]iotago.SlotIndex, 0)
	for i := infoResp.Status.LatestCommitmentID.Slot() + 2; i < infoResp.Status.LatestCommitmentID.Slot()+6; i++ {
		expectedLatestSlots = append(expectedLatestSlots, i)
	}

	expectedFinalizedSlots := make([]iotago.SlotIndex, 0)
	for i := infoResp.Status.LatestFinalizedSlot + 2; i < infoResp.Status.LatestFinalizedSlot+6; i++ {
		expectedFinalizedSlots = append(expectedFinalizedSlots, i)
	}

	assertions := []func(){
		func() { e.AssertLatestCommitments(ctx, eventClt, expectedLatestSlots) },
		func() { e.AssertFinalizedCommitments(ctx, eventClt, expectedFinalizedSlots) },
	}

	totalTopics := len(assertions)
	for _, assertion := range assertions {
		assertion()
	}

	// wait until all topics receives all expected objects
	err = e.AwaitEventAPITopics(t, cancel, totalTopics)
	require.NoError(t, err)
}

func test_ValidationBlocks(t *testing.T, e *EventAPIDockerTestFramework) {

	// get event API client ready
	ctx, cancel := context.WithCancel(context.Background())
	eventClt := e.ConnectEventAPIClient(ctx)
	defer eventClt.Close()

	// prepare the expected commitments to be received
	validators := make(map[string]struct{}, 0)
	nodes := e.dockerFramework.Nodes("V1", "V2", "V3", "V4")
	for _, node := range nodes {
		validators[node.AccountAddressBech32] = struct{}{}
	}

	assertions := []func(){
		func() {
			e.AssertValidationBlocks(ctx, eventClt, e.DefaultClient.CommittedAPI().ProtocolParameters().Bech32HRP(), validators)
		},
	}

	totalTopics := len(assertions)
	for _, assertion := range assertions {
		assertion()
	}

	// wait until all topics receives all expected objects
	err := e.AwaitEventAPITopics(t, cancel, totalTopics)
	require.NoError(t, err)
}

func test_BasicTaggedDataBlocks(t *testing.T, e *EventAPIDockerTestFramework) {
	// get event API client ready
	ctx, cancel := context.WithCancel(context.Background())
	eventClt := e.ConnectEventAPIClient(ctx)
	defer eventClt.Close()

	// create an account to issue blocks
	account := e.dockerFramework.CreateAccount()

	// prepare data blocks to send
	expectedBlocks := make(map[string]*iotago.Block)
	for i := 0; i < 10; i++ {
		blk := e.dockerFramework.CreateTaggedDataBlock(account.ID, []byte("tag"))
		expectedBlocks[blk.MustID().ToHex()] = blk
	}

	assertions := []func(){
		func() { e.AssertBlocks(ctx, eventClt, expectedBlocks) },
		func() { e.AssertBasicBlocks(ctx, eventClt, expectedBlocks) },
		func() { e.AssertTaggedDataBlocks(ctx, eventClt, expectedBlocks) },
		func() { e.AssertTaggedDataBlocksByTag(ctx, eventClt, expectedBlocks, []byte("tag")) },
		func() { e.AssertBlockMetadataAcceptedBlocks(ctx, eventClt, expectedBlocks) },
		func() { e.AssertBlockMetadataConfirmedBlocks(ctx, eventClt, expectedBlocks) },
	}

	totalTopics := len(assertions)
	for _, assertion := range assertions {
		assertion()
	}

	// wait until all topics starts listening
	err := e.AwaitEventAPITopics(t, cancel, totalTopics)
	require.NoError(t, err)

	// issue blocks
	go func() {
		for _, blk := range expectedBlocks {
			fmt.Println("submitting a block")
			e.dockerFramework.SubmitBlock(context.Background(), blk)
		}
	}()

	// wait until all topics receives all expected objects
	err = e.AwaitEventAPITopics(t, cancel, totalTopics)
	require.NoError(t, err)
}

func test_DelegationTransactionBlocks(t *testing.T, e *EventAPIDockerTestFramework) {
	// get event API client ready
	ctx, cancel := context.WithCancel(context.Background())
	eventClt := e.ConnectEventAPIClient(ctx)
	defer eventClt.Close()

	// create an account to issue blocks
	account := e.dockerFramework.CreateAccount()
	fundsOutputID := e.dockerFramework.RequestFaucetFunds(ctx, iotago.AddressEd25519)

	// prepare data blocks to send
	delegationId, outputId, blk := e.dockerFramework.CreateDelegationBlockFromInput(account.ID, e.dockerFramework.Node("V2").AccountAddress(t), fundsOutputID)
	expectedBlocks := map[string]*iotago.Block{
		blk.MustID().ToHex(): blk,
	}
	delegationOutput := e.dockerFramework.wallet.Output(outputId)

	asserts := []func(){
		func() { e.AssertTransactionBlocks(ctx, eventClt, expectedBlocks) },
		func() { e.AssertBasicBlocks(ctx, eventClt, expectedBlocks) },
		func() { e.AssertBlockMetadataAcceptedBlocks(ctx, eventClt, expectedBlocks) },
		func() { e.AssertBlockMetadataConfirmedBlocks(ctx, eventClt, expectedBlocks) },
		func() { e.AssertTransactionBlocksByTag(ctx, eventClt, expectedBlocks, []byte("delegation")) },
		func() { e.AssertTransactionMetadataByTransactionID(ctx, eventClt, outputId.TransactionID()) },
		func() { e.AssertTransactionMetadataIncludedBlocks(ctx, eventClt, outputId.TransactionID()) },
		func() { e.AssertDelegationOutput(ctx, eventClt, delegationId) },
		func() { e.AssertOutput(ctx, eventClt, outputId) },
		func() {
			e.AssertOutputsWithMetadataByUnlockConditionAndAddress(ctx, eventClt, api.EventAPIUnlockConditionAny, delegationOutput.Address)
		},
		func() {
			e.AssertOutputsWithMetadataByUnlockConditionAndAddress(ctx, eventClt, api.EventAPIUnlockConditionAddress, delegationOutput.Address)
		},
	}

	totalTopics := len(asserts)
	for _, assert := range asserts {
		assert()
	}

	// wait until all topics starts listening
	err := e.AwaitEventAPITopics(t, cancel, totalTopics)
	require.NoError(t, err)

	// issue blocks
	go func() {
		for _, blk := range expectedBlocks {
			fmt.Println("submitting a block: ", blk.MustID().ToHex())
			e.dockerFramework.SubmitBlock(context.Background(), blk)
		}
	}()

	// wait until all topics receives all expected objects
	err = e.AwaitEventAPITopics(t, cancel, totalTopics)
	require.NoError(t, err)
}

func test_AccountTransactionBlocks(t *testing.T, e *EventAPIDockerTestFramework) {
	// get event API client ready
	ctx, cancel := context.WithCancel(context.Background())
	eventClt := e.ConnectEventAPIClient(ctx)
	defer eventClt.Close()

	// implicit account transition
	{
		implicitAccount := e.dockerFramework.CreateImplicitAccount(ctx)

		// prepare fullAccount transaction block to send
		fullAccount, outputId, blk := e.dockerFramework.CreateAccountBlockFromInput(implicitAccount.OutputID)
		expectedBlocks := map[string]*iotago.Block{
			blk.MustID().ToHex(): blk,
		}
		accountOutput := e.dockerFramework.wallet.Output(outputId)

		assertions := []func(){
			func() { e.AssertTransactionBlocks(ctx, eventClt, expectedBlocks) },
			func() { e.AssertBasicBlocks(ctx, eventClt, expectedBlocks) },
			func() { e.AssertBlockMetadataAcceptedBlocks(ctx, eventClt, expectedBlocks) },
			func() { e.AssertBlockMetadataConfirmedBlocks(ctx, eventClt, expectedBlocks) },
			func() { e.AssertTransactionBlocksByTag(ctx, eventClt, expectedBlocks, []byte("account")) },
			func() { e.AssertTransactionMetadataByTransactionID(ctx, eventClt, outputId.TransactionID()) },
			func() { e.AssertTransactionMetadataIncludedBlocks(ctx, eventClt, outputId.TransactionID()) },
			func() { e.AssertAccountOutput(ctx, eventClt, fullAccount.ID) },
			func() { e.AssertOutput(ctx, eventClt, outputId) },
			func() {
				e.AssertOutputsWithMetadataByUnlockConditionAndAddress(ctx, eventClt, api.EventAPIUnlockConditionAny, accountOutput.Address)
			},
			func() {
				e.AssertOutputsWithMetadataByUnlockConditionAndAddress(ctx, eventClt, api.EventAPIUnlockConditionAddress, accountOutput.Address)
			},
		}

		totalTopics := len(assertions)
		for _, assertion := range assertions {
			assertion()
		}

		// wait until all topics starts listening
		err := e.AwaitEventAPITopics(t, cancel, totalTopics)
		require.NoError(t, err)

		// issue blocks
		go func() {
			for _, blk := range expectedBlocks {
				fmt.Println("submitting a block: ", blk.MustID().ToHex())
				e.dockerFramework.SubmitBlock(context.Background(), blk)
			}
		}()

		// update full account information
		e.dockerFramework.wallet.AddAccount(fullAccount.ID, fullAccount)

		// wait until all topics receives all expected objects
		err = e.AwaitEventAPITopics(t, cancel, totalTopics)
		require.NoError(t, err)
	}
}

func test_FoundryTransactionBlocks(t *testing.T, e *EventAPIDockerTestFramework) {
	// get event API client ready
	ctx, cancel := context.WithCancel(context.Background())
	eventClt := e.ConnectEventAPIClient(ctx)
	defer eventClt.Close()

	{
		account := e.dockerFramework.CreateAccount()
		fundsOutputID := e.dockerFramework.RequestFaucetFunds(ctx, iotago.AddressEd25519)

		// prepare foundry output block
		foundryId, outputId, blk := e.dockerFramework.CreateFoundryBlockFromInput(account.ID, fundsOutputID, 5_000_000, 10_000_000_000)
		expectedBlocks := map[string]*iotago.Block{
			blk.MustID().ToHex(): blk,
		}
		foundryOutput := e.dockerFramework.wallet.Output(outputId)

		assertions := []func(){
			func() { e.AssertTransactionBlocks(ctx, eventClt, expectedBlocks) },
			func() { e.AssertBasicBlocks(ctx, eventClt, expectedBlocks) },
			func() { e.AssertBlockMetadataAcceptedBlocks(ctx, eventClt, expectedBlocks) },
			func() { e.AssertBlockMetadataConfirmedBlocks(ctx, eventClt, expectedBlocks) },
			func() { e.AssertTransactionBlocksByTag(ctx, eventClt, expectedBlocks, []byte("foundry")) },
			func() { e.AssertTransactionMetadataByTransactionID(ctx, eventClt, outputId.TransactionID()) },
			func() { e.AssertTransactionMetadataIncludedBlocks(ctx, eventClt, outputId.TransactionID()) },
			func() { e.AssertAccountOutput(ctx, eventClt, account.ID) },
			func() { e.AssertFoundryOutput(ctx, eventClt, foundryId) },
			func() { e.AssertOutput(ctx, eventClt, outputId) },
			func() { e.AssertOutput(ctx, eventClt, account.OutputID) },
			func() {
				e.AssertOutputsWithMetadataByUnlockConditionAndAddress(ctx, eventClt, api.EventAPIUnlockConditionAny, foundryOutput.Address)
			},
			func() {
				e.AssertOutputsWithMetadataByUnlockConditionAndAddress(ctx, eventClt, api.EventAPIUnlockConditionImmutableAccount, foundryOutput.Address)
			},
		}

		totalTopics := len(assertions)
		for _, assertion := range assertions {
			assertion()
		}

		// wait until all topics starts listening
		err := e.AwaitEventAPITopics(t, cancel, totalTopics)
		require.NoError(t, err)

		// issue blocks
		go func() {
			for _, blk := range expectedBlocks {
				fmt.Println("submitting a block: ", blk.MustID().ToHex())
				e.dockerFramework.SubmitBlock(context.Background(), blk)
			}
		}()

		// wait until all topics receives all expected objects
		err = e.AwaitEventAPITopics(t, cancel, totalTopics)
		require.NoError(t, err)
	}
}

func test_NFTTransactionBlocks(t *testing.T, e *EventAPIDockerTestFramework) {
	// get event API client ready
	ctx, cancel := context.WithCancel(context.Background())
	eventClt := e.ConnectEventAPIClient(ctx)
	defer eventClt.Close()

	{
		account := e.dockerFramework.CreateAccount()
		fundsOutputID := e.dockerFramework.RequestFaucetFunds(ctx, iotago.AddressEd25519)

		// prepare foundry output block
		nftId, outputId, blk := e.dockerFramework.CreateNFTBlockFromInput(account.ID, fundsOutputID)
		expectedBlocks := map[string]*iotago.Block{
			blk.MustID().ToHex(): blk,
		}
		nftOutput := e.dockerFramework.wallet.Output(outputId)

		assertions := []func(){
			func() { e.AssertTransactionBlocks(ctx, eventClt, expectedBlocks) },
			func() { e.AssertBasicBlocks(ctx, eventClt, expectedBlocks) },
			func() { e.AssertBlockMetadataAcceptedBlocks(ctx, eventClt, expectedBlocks) },
			func() { e.AssertBlockMetadataConfirmedBlocks(ctx, eventClt, expectedBlocks) },
			func() { e.AssertTransactionBlocksByTag(ctx, eventClt, expectedBlocks, []byte("nft")) },
			func() { e.AssertTransactionMetadataByTransactionID(ctx, eventClt, outputId.TransactionID()) },
			func() { e.AssertTransactionMetadataIncludedBlocks(ctx, eventClt, outputId.TransactionID()) },
			func() { e.AssertNFTOutput(ctx, eventClt, nftId) },
			func() { e.AssertOutput(ctx, eventClt, outputId) },
			func() {
				e.AssertOutputsWithMetadataByUnlockConditionAndAddress(ctx, eventClt, api.EventAPIUnlockConditionAny, nftOutput.Address)
			},
			func() {
				e.AssertOutputsWithMetadataByUnlockConditionAndAddress(ctx, eventClt, api.EventAPIUnlockConditionAddress, nftOutput.Address)
			},
		}

		totalTopics := len(assertions)
		for _, assertion := range assertions {
			assertion()
		}

		// wait until all topics starts listening
		err := e.AwaitEventAPITopics(t, cancel, totalTopics)
		require.NoError(t, err)

		// issue blocks
		go func() {
			for _, blk := range expectedBlocks {
				fmt.Println("submitting a block ", blk.MustID().ToHex())
				e.dockerFramework.SubmitBlock(context.Background(), blk)
			}
		}()

		// wait until all topics receives all expected objects
		err = e.AwaitEventAPITopics(t, cancel, totalTopics)
		require.NoError(t, err)
	}
}

func test_BlockMetadataMatchedCoreAPI(t *testing.T, e *EventAPIDockerTestFramework) {
	// get event API client ready
	ctx, cancel := context.WithCancel(context.Background())
	eventClt := e.ConnectEventAPIClient(ctx)
	defer eventClt.Close()

	{
		account := e.dockerFramework.CreateAccount()

		assertions := []func(){
			func() { e.AssertBlockMetadataStateAcceptedBlocks(ctx, eventClt) },
			func() { e.AssertBlockMetadataStateConfirmedBlocks(ctx, eventClt) },
		}

		totalTopics := len(assertions)
		for _, assertion := range assertions {
			assertion()
		}

		// wait until all topics starts listening
		err := e.AwaitEventAPITopics(t, cancel, totalTopics)
		require.NoError(t, err)

		// issue blocks
		e.SubmitDataBlockStream(account, 5*time.Minute)

		cancel()
	}
}
