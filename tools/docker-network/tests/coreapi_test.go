//go:build dockertests

package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func Test_CoreAPI(t *testing.T) {
	d := NewDockerTestFramework(t,
		WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 4),
			iotago.WithLivenessOptions(10, 10, 2, 4, 8),
			iotago.WithRewardsOptions(8, 10, 2, 384),
			iotago.WithTargetCommitteeSize(4),
		))
	defer d.Stop()

	d.AddValidatorNode("V1", "docker-network-inx-validator-1-1", "http://localhost:8050", "rms1pzg8cqhfxqhq7pt37y8cs4v5u4kcc48lquy2k73ehsdhf5ukhya3y5rx2w6")
	d.AddValidatorNode("V2", "docker-network-inx-validator-2-1", "http://localhost:8060", "rms1pqm4xk8e9ny5w5rxjkvtp249tfhlwvcshyr3pc0665jvp7g3hc875k538hl")
	d.AddValidatorNode("V3", "docker-network-inx-validator-3-1", "http://localhost:8070", "rms1pp4wuuz0y42caz48vv876qfpmffswsvg40zz8v79sy8cp0jfxm4kunflcgt")
	d.AddValidatorNode("V4", "docker-network-inx-validator-4-1", "http://localhost:8040", "rms1pr8cxs3dzu9xh4cduff4dd4cxdthpjkpwmz2244f75m0urslrsvtsshrrjw")
	d.AddNode("node5", "docker-network-node-5-1", "http://localhost:8090")

	runErr := d.Run()
	require.NoError(t, runErr)

	d.WaitUntilNetworkReady()

	assetsPerSlot, lastSlot := d.prepareAssets(3)

	fmt.Println("AwaitCommitment for slot", lastSlot)
	d.AwaitCommitment(lastSlot)

	tests := []struct {
		name     string
		testFunc func(t *testing.T, nodeAlias string)
	}{
		{
			name: "Test_Info",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.wallet.Clients[nodeAlias].Info(context.Background())
				require.NoError(t, err)
				require.NotNil(t, resp)
			},
		},
		{
			name: "Test_BlockByBlockID",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachBlock(t, func(t *testing.T, block *iotago.Block) {
					respBlock, err := d.wallet.Clients[nodeAlias].BlockByBlockID(context.Background(), block.MustID())
					require.NoError(t, err)
					require.NotNil(t, respBlock)
					require.Equal(t, block.MustID(), respBlock.MustID(), "BlockID of retrieved block does not match: %s != %s", block.MustID(), respBlock.MustID())
					//require.EqualValues(t, block, respBlock)
				})
			},
		},
		{
			name: "Test_BlockMetadataByBlockID",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachBlock(t, func(t *testing.T, block *iotago.Block) {
					resp, err := d.wallet.Clients[nodeAlias].BlockMetadataByBlockID(context.Background(), block.MustID())
					require.NoError(t, err)
					require.NotNil(t, resp)
					//require.Equal(t, block.MustID(), resp.BlockID)
					require.Equal(t, api.BlockStateFinalized, resp.BlockState)
				})
			},
		},
		{
			name: "Test_BlockWithMetadata",
			testFunc: func(t *testing.T, nodeAlias string) {
				// TODO missing client side implementation for BlockWithMetadata
				assetsPerSlot.forEachBlock(t, func(t *testing.T, block *iotago.Block) {
					resp, err := d.wallet.Clients[nodeAlias].BlockMetadataByBlockID(context.Background(), block.MustID())
					require.NoError(t, err)
					require.NotNil(t, resp)
					require.Equal(t, block.MustID(), resp.BlockID)
					require.Equal(t, api.BlockStateFinalized, resp.BlockState)
				})
			},
		},
		{
			name: "Test_BlockIssuance",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.wallet.Clients[nodeAlias].BlockIssuance(context.Background())
				require.NoError(t, err)
				require.NotNil(t, resp)

				require.GreaterOrEqual(t, len(resp.StrongParents), 1)
			},
		},
		{
			name: "Test_CommitmentBySlot",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachSlot(t, func(t *testing.T, slot iotago.SlotIndex, commitmentsPerNode map[string]iotago.CommitmentID) {
					resp, err := d.wallet.Clients[nodeAlias].CommitmentByIndex(context.Background(), slot)
					require.NoError(t, err)
					require.NotNil(t, resp)
					commitmentsPerNode[nodeAlias] = resp.MustID()
				})
			},
		},
		{
			name: "Test_CommitmentByID",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachCommitment(t, func(t *testing.T, commitmentsPerNode map[string]iotago.CommitmentID) {
					resp, err := d.wallet.Clients[nodeAlias].CommitmentByID(context.Background(), commitmentsPerNode[nodeAlias])
					require.NoError(t, err)
					require.NotNil(t, resp)
					require.Equal(t, commitmentsPerNode[nodeAlias], resp.MustID())
				})
			},
		},
		{
			name: "Test_CommitmentUTXOChangesByID",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachCommitment(t, func(t *testing.T, commitmentsPerNode map[string]iotago.CommitmentID) {
					resp, err := d.wallet.Clients[nodeAlias].CommitmentUTXOChangesByID(context.Background(), commitmentsPerNode[nodeAlias])
					require.NoError(t, err)
					require.NotNil(t, resp)
					assetsPerSlot.assertUTXOOutputIDsInSlot(t, commitmentsPerNode[nodeAlias].Slot(), resp.CreatedOutputs, resp.ConsumedOutputs)
					require.Equal(t, commitmentsPerNode[nodeAlias], resp.CommitmentID)
				})
			},
		},
		{
			"Test_CommitmentUTXOChangesFullByID",
			func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachCommitment(t, func(t *testing.T, commitmentsPerNode map[string]iotago.CommitmentID) {
					resp, err := d.wallet.Clients[nodeAlias].CommitmentUTXOChangesFullByID(context.Background(), commitmentsPerNode[nodeAlias])
					require.NoError(t, err)
					require.NotNil(t, resp)
					assetsPerSlot.assertUTXOOutputsInSlot(t, commitmentsPerNode[nodeAlias].Slot(), resp.CreatedOutputs, resp.ConsumedOutputs)
					require.Equal(t, commitmentsPerNode[nodeAlias], resp.CommitmentID)
				})
			},
		},
		{
			name: "Test_CommitmentUTXOChangesBySlot",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachCommitment(t, func(t *testing.T, commitmentsPerNode map[string]iotago.CommitmentID) {
					resp, err := d.wallet.Clients[nodeAlias].CommitmentUTXOChangesByIndex(context.Background(), commitmentsPerNode[nodeAlias].Slot())
					require.NoError(t, err)
					require.NotNil(t, resp)
					assetsPerSlot.assertUTXOOutputIDsInSlot(t, commitmentsPerNode[nodeAlias].Slot(), resp.CreatedOutputs, resp.ConsumedOutputs)
					require.Equal(t, commitmentsPerNode[nodeAlias], resp.CommitmentID)
				})
			},
		},
		{
			name: "Test_CommitmentUTXOChangesFullBySlot",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachCommitment(t, func(t *testing.T, commitmentsPerNode map[string]iotago.CommitmentID) {
					resp, err := d.wallet.Clients[nodeAlias].CommitmentUTXOChangesFullByIndex(context.Background(), commitmentsPerNode[nodeAlias].Slot())
					require.NoError(t, err)
					require.NotNil(t, resp)
					assetsPerSlot.assertUTXOOutputsInSlot(t, commitmentsPerNode[nodeAlias].Slot(), resp.CreatedOutputs, resp.ConsumedOutputs)
					require.Equal(t, commitmentsPerNode[nodeAlias], resp.CommitmentID)
				})
			},
		},
		{
			name: "Test_OutputByID",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachOutput(t, func(t *testing.T, outputID iotago.OutputID, output iotago.Output) {
					resp, err := d.wallet.Clients[nodeAlias].OutputByID(context.Background(), outputID)
					require.NoError(t, err)
					require.NotNil(t, resp)
					require.EqualValues(t, output, resp)
				})
			},
		},
		{
			name: "Test_OutputMetadata",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachOutput(t, func(t *testing.T, outputID iotago.OutputID, output iotago.Output) {
					resp, err := d.wallet.Clients[nodeAlias].OutputMetadataByID(context.Background(), outputID)
					require.NoError(t, err)
					require.NotNil(t, resp)
					require.EqualValues(t, outputID, resp.OutputID)
					require.EqualValues(t, outputID.Slot(), resp.Included.Slot)
					require.EqualValues(t, outputID.TransactionID(), resp.Included.TransactionID)
				})
			},
		},
		{
			name: "Test_TransactionsIncludedBlock",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachTransaction(t, func(t *testing.T, transaction *iotago.SignedTransaction) {
					resp, err := d.wallet.Clients[nodeAlias].TransactionIncludedBlock(context.Background(), lo.PanicOnErr(transaction.Transaction.ID()))
					require.NoError(t, err)
					require.NotNil(t, resp)
				})

				// todo issue second block with the same tx, and make sure that the first one is returned here
			},
		},
		{
			name: "Test_TransactionsIncludedBlockMetadata",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachTransaction(t, func(t *testing.T, transaction *iotago.SignedTransaction) {
					resp, err := d.wallet.Clients[nodeAlias].TransactionIncludedBlockMetadata(context.Background(), lo.PanicOnErr(transaction.Transaction.ID()))
					require.NoError(t, err)
					require.NotNil(t, resp)
				})
			},
		},
		{
			name: "Test_TransactionsMetadata",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachTransaction(t, func(t *testing.T, transaction *iotago.SignedTransaction) {
					resp, err := d.wallet.Clients[nodeAlias].TransactionMetadata(context.Background(), lo.PanicOnErr(transaction.Transaction.ID()))
					require.NoError(t, err)
					require.NotNil(t, resp)
					require.Equal(t, api.TransactionStateFinalized, resp.TransactionState)
				})
			},
		},
		{
			name: "Test_Congestion",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachAccountAddress(t, func(t *testing.T, accountAddress *iotago.AccountAddress, commitmentPerNode map[string]iotago.CommitmentID) {

					resp, err := d.wallet.Clients[nodeAlias].Congestion(context.Background(), accountAddress, 0)
					require.NoError(t, err)
					require.NotNil(t, resp)

					resp, err = d.wallet.Clients[nodeAlias].Congestion(context.Background(), accountAddress, 0, commitmentPerNode[nodeAlias])
					require.NoError(t, err)
					require.NotNil(t, resp)
				})
			},
		},
		{
			name: "Test_Validators",
			testFunc: func(t *testing.T, nodeAlias string) {
				pageSize := uint64(3)
				resp, err := d.wallet.Clients[nodeAlias].Validators(context.Background(), pageSize)
				require.NoError(t, err)
				require.NotNil(t, resp)
				require.Equal(t, len(resp.Validators), int(pageSize))

				resp, err = d.wallet.Clients[nodeAlias].Validators(context.Background(), pageSize, resp.Cursor)
				require.NoError(t, err)
				require.NotNil(t, resp)
				require.Equal(t, len(resp.Validators), 1)
			},
		},
		{
			name: "Test_ValidatorsAll",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, all, err := d.wallet.Clients[nodeAlias].ValidatorsAll(context.Background())
				require.NoError(t, err)
				require.True(t, all)
				require.Equal(t, 4, len(resp.Validators))
			},
		},
		{
			name: "Test_Rewards",
			testFunc: func(t *testing.T, nodeAlias string) {
				//skip: we will test it with caliming tests
			},
		},
		{
			name: "Test_Committee",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.wallet.Clients[nodeAlias].Committee(context.Background())
				require.NoError(t, err)
				require.NotNil(t, resp)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d.requestFromClients(test.testFunc)
		})
	}

	assetsPerSlot.assertCommitments(t)
}
