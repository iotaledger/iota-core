//go:build dockertests

package tests

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/nodeclient"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

// Test_ValidatorsAPI tests if the validators API returns the expected validators.
// 1. Run docker network.
// 2. Create 50 new accounts with staking feature.
// 3. Wait until next epoch then issue candidacy payload for each account.
// 4. Check if all 54 validators are returned from the validators API with pageSize 10, the pagination of api is also tested.
// 5. Wait until next epoch then check again if the results remain.
func Test_ValidatorsAPI(t *testing.T) {
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
	hrp := d.wallet.DefaultClient().CommittedAPI().ProtocolParameters().Bech32HRP()

	// Create registered validators
	var wg sync.WaitGroup
	clt := d.wallet.DefaultClient()
	status := d.NodeStatus("V1")
	currentEpoch := clt.CommittedAPI().TimeProvider().EpochFromSlot(status.LatestAcceptedBlockSlot)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			account := d.CreateAccount(WithStakingFeature(100, 1, 0))

			// issue candidacy payload in the next epoch (currentEpoch + 1), in order to issue it before epochNearingThreshold
			d.AwaitCommitment(clt.CommittedAPI().TimeProvider().EpochEnd(currentEpoch))
			blkID := d.IssueCandidacyPayloadFromAccount(account.ID)
			fmt.Println("Candidacy payload:", blkID.ToHex(), blkID.Slot())
			d.AwaitCommitment(blkID.Slot())
		}()
	}
	wg.Wait()

	expectedValidators := d.AccountsFromNodes(d.Nodes()...)
	for _, v := range d.wallet.Accounts() {
		expectedValidators = append(expectedValidators, v.Address.Bech32(hrp))
	}
	// get all validators of currentEpoch+1 with pageSize 10
	actualValidators := getAllValidatorsOnEpoch(t, clt, 0, 10)
	require.ElementsMatch(t, expectedValidators, actualValidators)

	// wait until currentEpoch+3 and check the results again
	targetSlot := clt.CommittedAPI().TimeProvider().EpochEnd(currentEpoch + 2)
	d.AwaitCommitment(targetSlot)
	actualValidators = getAllValidatorsOnEpoch(t, clt, currentEpoch+1, 10)
	require.ElementsMatch(t, expectedValidators, actualValidators)
}

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

	assetsPerSlot, lastSlot := d.prepareAssets(5)

	fmt.Println("Await finalisation of slot", lastSlot)
	d.AwaitFinalization(lastSlot)

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
					require.Equal(t, block.MustID(), resp.BlockID, "BlockID of retrieved block does not match: %s != %s", block.MustID(), resp.BlockID)
					require.Equal(t, api.BlockStateFinalized, resp.BlockState)
				})

				assetsPerSlot.forEachReattachment(t, func(t *testing.T, blockID iotago.BlockID) {
					resp, err := d.wallet.Clients[nodeAlias].BlockMetadataByBlockID(context.Background(), blockID)
					require.NoError(t, err)
					require.NotNil(t, resp)
					require.Equal(t, blockID, resp.BlockID, "BlockID of retrieved block does not match: %s != %s", blockID, resp.BlockID)
					require.Equal(t, api.BlockStateFinalized, resp.BlockState)
				})
			},
		},
		{
			name: "Test_BlockWithMetadata",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachBlock(t, func(t *testing.T, block *iotago.Block) {
					resp, err := d.wallet.Clients[nodeAlias].BlockWithMetadataByBlockID(context.Background(), block.MustID())
					require.NoError(t, err)
					require.NotNil(t, resp)
					require.Equal(t, block.MustID(), resp.Block.MustID(), "BlockID of retrieved block does not match: %s != %s", block.MustID(), resp.Block.MustID())
					require.Equal(t, api.BlockStateFinalized, resp.Metadata.BlockState)
				})
			},
		},
		{
			name: "Test_BlockIssuance",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.wallet.Clients[nodeAlias].BlockIssuance(context.Background())
				require.NoError(t, err)
				require.NotNil(t, resp)

				require.GreaterOrEqual(t, len(resp.StrongParents), 1, "There should be at least 1 strong parent provided")
			},
		},
		{
			name: "Test_CommitmentBySlot",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachSlot(t, func(t *testing.T, slot iotago.SlotIndex, commitmentsPerNode map[string]iotago.CommitmentID) {
					resp, err := d.wallet.Clients[nodeAlias].CommitmentBySlot(context.Background(), slot)
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
					require.Equal(t, commitmentsPerNode[nodeAlias], resp.MustID(), "Commitment does not match commitment got for the same slot from the same node: %s != %s", commitmentsPerNode[nodeAlias], resp.MustID())
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
					require.Equal(t, commitmentsPerNode[nodeAlias], resp.CommitmentID, "CommitmentID of retrieved UTXO changes does not match: %s != %s", commitmentsPerNode[nodeAlias], resp.CommitmentID)
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
					require.Equal(t, commitmentsPerNode[nodeAlias], resp.CommitmentID, "CommitmentID of retrieved UTXO changes does not match: %s != %s", commitmentsPerNode[nodeAlias], resp.CommitmentID)
				})
			},
		},
		{
			name: "Test_CommitmentUTXOChangesBySlot",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachCommitment(t, func(t *testing.T, commitmentsPerNode map[string]iotago.CommitmentID) {
					resp, err := d.wallet.Clients[nodeAlias].CommitmentUTXOChangesBySlot(context.Background(), commitmentsPerNode[nodeAlias].Slot())
					require.NoError(t, err)
					require.NotNil(t, resp)
					assetsPerSlot.assertUTXOOutputIDsInSlot(t, commitmentsPerNode[nodeAlias].Slot(), resp.CreatedOutputs, resp.ConsumedOutputs)
					require.Equal(t, commitmentsPerNode[nodeAlias], resp.CommitmentID, "CommitmentID of retrieved UTXO changes does not match: %s != %s", commitmentsPerNode[nodeAlias], resp.CommitmentID)
				})
			},
		},
		{
			name: "Test_CommitmentUTXOChangesFullBySlot",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachCommitment(t, func(t *testing.T, commitmentsPerNode map[string]iotago.CommitmentID) {
					resp, err := d.wallet.Clients[nodeAlias].CommitmentUTXOChangesFullBySlot(context.Background(), commitmentsPerNode[nodeAlias].Slot())
					require.NoError(t, err)
					require.NotNil(t, resp)
					assetsPerSlot.assertUTXOOutputsInSlot(t, commitmentsPerNode[nodeAlias].Slot(), resp.CreatedOutputs, resp.ConsumedOutputs)
					require.Equal(t, commitmentsPerNode[nodeAlias], resp.CommitmentID, "CommitmentID of retrieved UTXO changes does not match: %s != %s", commitmentsPerNode[nodeAlias], resp.CommitmentID)
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
					require.EqualValues(t, output, resp, "Output created is different than retrieved from the API: %s != %s", output, resp)
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
					require.EqualValues(t, outputID, resp.OutputID, "OutputID of retrieved output does not match: %s != %s", outputID, resp.OutputID)
					require.EqualValues(t, outputID.TransactionID(), resp.Included.TransactionID, "TransactionID of retrieved output does not match: %s != %s", outputID.TransactionID(), resp.Included.TransactionID)
				})
			},
		},
		{
			name: "Test_OutputWithMetadata",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachOutput(t, func(t *testing.T, outputID iotago.OutputID, output iotago.Output) {
					out, outMetadata, err := d.wallet.Clients[nodeAlias].OutputWithMetadataByID(context.Background(), outputID)
					require.NoError(t, err)
					require.NotNil(t, outMetadata)
					require.NotNil(t, out)
					require.EqualValues(t, outputID, outMetadata.OutputID, "OutputID of retrieved output does not match: %s != %s", outputID, outMetadata.OutputID)
					require.EqualValues(t, outputID.TransactionID(), outMetadata.Included.TransactionID, "TransactionID of retrieved output does not match: %s != %s", outputID.TransactionID(), outMetadata.Included.TransactionID)
					require.EqualValues(t, output, out, "OutputID of retrieved output does not match: %s != %s", output, out)
				})
			},
		},
		{
			name: "Test_TransactionsIncludedBlock",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachTransaction(t, func(t *testing.T, transaction *iotago.SignedTransaction, firstAttachmentID iotago.BlockID) {
					resp, err := d.wallet.Clients[nodeAlias].TransactionIncludedBlock(context.Background(), lo.PanicOnErr(transaction.Transaction.ID()))
					require.NoError(t, err)
					require.NotNil(t, resp)
					require.EqualValues(t, firstAttachmentID, resp.MustID())
				})
			},
		},
		{
			name: "Test_TransactionsIncludedBlockMetadata",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachTransaction(t, func(t *testing.T, transaction *iotago.SignedTransaction, firstAttachmentID iotago.BlockID) {
					resp, err := d.wallet.Clients[nodeAlias].TransactionIncludedBlockMetadata(context.Background(), lo.PanicOnErr(transaction.Transaction.ID()))
					require.NoError(t, err)
					require.NotNil(t, resp)
					require.EqualValues(t, api.BlockStateFinalized, resp.BlockState)
					require.EqualValues(t, firstAttachmentID, resp.BlockID, "Inclusion BlockID of retrieved transaction does not match: %s != %s", firstAttachmentID, resp.BlockID)
				})
			},
		},
		{
			name: "Test_TransactionsMetadata",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachTransaction(t, func(t *testing.T, transaction *iotago.SignedTransaction, firstAttachmentID iotago.BlockID) {
					resp, err := d.wallet.Clients[nodeAlias].TransactionMetadata(context.Background(), lo.PanicOnErr(transaction.Transaction.ID()))
					require.NoError(t, err)
					require.NotNil(t, resp)
					require.Equal(t, api.TransactionStateFinalized, resp.TransactionState)
					require.EqualValues(t, resp.EarliestAttachmentSlot, firstAttachmentID.Slot())
				})
			},
		},
		{
			name: "Test_Congestion",
			testFunc: func(t *testing.T, nodeAlias string) {
				assetsPerSlot.forEachAccountAddress(t, func(
					t *testing.T,
					accountAddress *iotago.AccountAddress,
					commitmentPerNode map[string]iotago.CommitmentID,
					bicPerNoode map[string]iotago.BlockIssuanceCredits,
				) {
					resp, err := d.wallet.Clients[nodeAlias].Congestion(context.Background(), accountAddress, 0)
					require.NoError(t, err)
					require.NotNil(t, resp)

					// node allows to get account only for the slot newer than lastCommittedSlot - MCA, we need fresh commitment
					infoRes, err := d.wallet.Clients[nodeAlias].Info(context.Background())
					require.NoError(t, err)
					commitment, err := d.wallet.Clients[nodeAlias].CommitmentBySlot(context.Background(), infoRes.Status.LatestCommitmentID.Slot())
					require.NoError(t, err)

					resp, err = d.wallet.Clients[nodeAlias].Congestion(context.Background(), accountAddress, 0, commitment.MustID())
					require.NoError(t, err)
					require.NotNil(t, resp)
					// later we check if all nodes have returned the same BIC value for this account
					bicPerNoode[nodeAlias] = resp.BlockIssuanceCredits
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
				//TODO after finishing validators endpoint and including registered validators
				//require.Equal(t, int(pageSize), len(resp.Validators), "There should be exactly %d validators returned on the first page", pageSize)

				resp, err = d.wallet.Clients[nodeAlias].Validators(context.Background(), pageSize, resp.Cursor)
				require.NoError(t, err)
				require.NotNil(t, resp)
				//TODO after finishing validators endpoint and including registered validators
				//require.Equal(t, 1, len(resp.Validators), "There should be only one validator returned on the last page")
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
				assetsPerSlot.forEachOutput(t, func(t *testing.T, outputID iotago.OutputID, output iotago.Output) {
					if output.Type() != iotago.OutputDelegation {
						return
					}

					resp, err := d.wallet.Clients[nodeAlias].Rewards(context.Background(), outputID)
					require.NoError(t, err)
					require.NotNil(t, resp)
					// rewards are zero, because we do not wait for the epoch end
					require.EqualValues(t, 0, resp.Rewards)
				})
			},
		},
		{
			name: "Test_Committee",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.wallet.Clients[nodeAlias].Committee(context.Background())
				require.NoError(t, err)
				require.NotNil(t, resp)
				require.EqualValues(t, 4, len(resp.Committee))
			},
		},
		{
			name: "Test_CommitteeWithEpoch",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.wallet.Clients[nodeAlias].Committee(context.Background(), 0)
				require.NoError(t, err)
				require.Equal(t, iotago.EpochIndex(0), resp.Epoch)
				require.Equal(t, 4, len(resp.Committee))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d.requestFromClients(test.testFunc)
		})
	}

	// check if the same values were returned by all nodes for the same slot
	assetsPerSlot.assertCommitments(t)
	assetsPerSlot.assertBICs(t)
}

func Test_CoreAPI_BadRequests(t *testing.T) {
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

	tests := []struct {
		name     string
		testFunc func(t *testing.T, nodeAlias string)
	}{
		{
			name: "Test_BlockByBlockID_Failure",
			testFunc: func(t *testing.T, nodeAlias string) {
				blockID := tpkg.RandBlockID()
				respBlock, err := d.wallet.Clients[nodeAlias].BlockByBlockID(context.Background(), blockID)
				require.Error(t, err)
				require.True(t, isStatusCode(err, http.StatusNotFound))
				require.Nil(t, respBlock)
			},
		},
		{
			name: "Test_BlockMetadataByBlockID_Failure",
			testFunc: func(t *testing.T, nodeAlias string) {
				blockID := tpkg.RandBlockID()
				resp, err := d.wallet.Clients[nodeAlias].BlockMetadataByBlockID(context.Background(), blockID)
				require.Error(t, err)
				require.True(t, isStatusCode(err, http.StatusNotFound))
				require.Nil(t, resp)
			},
		},
		{
			name: "Test_BlockWithMetadata_Failure",
			testFunc: func(t *testing.T, nodeAlias string) {
				blockID := tpkg.RandBlockID()
				resp, err := d.wallet.Clients[nodeAlias].BlockWithMetadataByBlockID(context.Background(), blockID)
				require.Error(t, err)
				require.True(t, isStatusCode(err, http.StatusNotFound))
				require.Nil(t, resp)
			},
		},
		{
			name: "Test_CommitmentBySlot_Failure",
			testFunc: func(t *testing.T, nodeAlias string) {
				slot := iotago.SlotIndex(1000_000_000)
				resp, err := d.wallet.Clients[nodeAlias].CommitmentBySlot(context.Background(), slot)
				require.Error(t, err)
				require.True(t, isStatusCode(err, http.StatusBadRequest))
				require.Nil(t, resp)
			},
		},
		{
			name: "Test_CommitmentByID_Failure",
			testFunc: func(t *testing.T, nodeAlias string) {
				committmentID := tpkg.RandCommitmentID()
				resp, err := d.wallet.Clients[nodeAlias].CommitmentByID(context.Background(), committmentID)
				require.Error(t, err)
				require.True(t, isStatusCode(err, http.StatusBadRequest))
				require.Nil(t, resp)
			},
		},
		{
			name: "Test_CommitmentUTXOChangesByID_Failure",
			testFunc: func(t *testing.T, nodeAlias string) {
				committmentID := tpkg.RandCommitmentID()
				resp, err := d.wallet.Clients[nodeAlias].CommitmentUTXOChangesByID(context.Background(), committmentID)
				require.Error(t, err)
				// commitmentID is valid, but the UTXO changes does not exist in the storage
				require.True(t, isStatusCode(err, http.StatusNotFound))
				require.Nil(t, resp)
			},
		},
		{
			"Test_CommitmentUTXOChangesFullByID_Failure",
			func(t *testing.T, nodeAlias string) {
				committmentID := tpkg.RandCommitmentID()

				resp, err := d.wallet.Clients[nodeAlias].CommitmentUTXOChangesFullByID(context.Background(), committmentID)
				require.Error(t, err)
				// commitmentID is valid, but the UTXO changes does not exist in the storage
				require.True(t, isStatusCode(err, http.StatusNotFound))
				require.Nil(t, resp)
			},
		},
		{
			name: "Test_CommitmentUTXOChangesBySlot_Failure",
			testFunc: func(t *testing.T, nodeAlias string) {
				slot := iotago.SlotIndex(1000_000_000)
				resp, err := d.wallet.Clients[nodeAlias].CommitmentUTXOChangesBySlot(context.Background(), slot)
				require.Error(t, err)
				require.True(t, isStatusCode(err, http.StatusBadRequest))
				require.Nil(t, resp)
			},
		},
		{
			name: "Test_CommitmentUTXOChangesFullBySlot_Failure",
			testFunc: func(t *testing.T, nodeAlias string) {
				slot := iotago.SlotIndex(1000_000_000)

				resp, err := d.wallet.Clients[nodeAlias].CommitmentUTXOChangesFullBySlot(context.Background(), slot)
				require.Error(t, err)
				require.True(t, isStatusCode(err, http.StatusBadRequest))
				require.Nil(t, resp)
			},
		},
		{
			name: "Test_OutputByID_Failure",
			testFunc: func(t *testing.T, nodeAlias string) {
				outputID := tpkg.RandOutputID(0)
				resp, err := d.wallet.Clients[nodeAlias].OutputByID(context.Background(), outputID)
				require.Error(t, err)
				require.True(t, isStatusCode(err, http.StatusNotFound))
				require.Nil(t, resp)
			},
		},
		{
			name: "Test_OutputMetadata_Failure",
			testFunc: func(t *testing.T, nodeAlias string) {
				outputID := tpkg.RandOutputID(0)

				resp, err := d.wallet.Clients[nodeAlias].OutputMetadataByID(context.Background(), outputID)
				require.Error(t, err)
				require.True(t, isStatusCode(err, http.StatusNotFound))
				require.Nil(t, resp)
			},
		},
		{
			name: "Test_OutputWithMetadata_Failure",
			testFunc: func(t *testing.T, nodeAlias string) {
				outputID := tpkg.RandOutputID(0)

				out, outMetadata, err := d.wallet.Clients[nodeAlias].OutputWithMetadataByID(context.Background(), outputID)
				require.Error(t, err)
				require.Nil(t, out)
				require.Nil(t, outMetadata)
				require.True(t, isStatusCode(err, http.StatusNotFound))
			},
		},
		{
			name: "Test_TransactionsIncludedBlock_Failure",
			testFunc: func(t *testing.T, nodeAlias string) {
				txID := tpkg.RandTransactionID()
				resp, err := d.wallet.Clients[nodeAlias].TransactionIncludedBlock(context.Background(), txID)
				require.Error(t, err)
				require.True(t, isStatusCode(err, http.StatusBadRequest))
				require.Nil(t, resp)
			},
		},
		{
			name: "Test_TransactionsIncludedBlockMetadata_Failure",
			testFunc: func(t *testing.T, nodeAlias string) {
				txID := tpkg.RandTransactionID()

				resp, err := d.wallet.Clients[nodeAlias].TransactionIncludedBlockMetadata(context.Background(), txID)
				require.Error(t, err)
				require.True(t, isStatusCode(err, http.StatusBadRequest))
				require.Nil(t, resp)
			},
		},
		{
			name: "Test_TransactionsMetadata_Failure",
			testFunc: func(t *testing.T, nodeAlias string) {
				txID := tpkg.RandTransactionID()

				resp, err := d.wallet.Clients[nodeAlias].TransactionMetadata(context.Background(), txID)
				require.Error(t, err)
				require.True(t, isStatusCode(err, http.StatusNotFound))
				require.Nil(t, resp)
			},
		},
		{
			name: "Test_Congestion_Failure",
			testFunc: func(t *testing.T, nodeAlias string) {
				accountAddress := tpkg.RandAccountAddress()
				commitmentID := tpkg.RandCommitmentID()
				resp, err := d.wallet.Clients[nodeAlias].Congestion(context.Background(), accountAddress, 0, commitmentID)
				require.Error(t, err)
				require.True(t, isStatusCode(err, http.StatusBadRequest))
				require.Nil(t, resp)
			},
		},
		{
			name: "Test_Committee_Failure",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.wallet.Clients[nodeAlias].Committee(context.Background(), 4)
				require.Error(t, err)
				require.True(t, isStatusCode(err, http.StatusBadRequest))
				require.Nil(t, resp)
			},
		},
		{
			name: "Test_Rewards_Failure",
			testFunc: func(t *testing.T, nodeAlias string) {
				outputID := tpkg.RandOutputID(0)
				resp, err := d.wallet.Clients[nodeAlias].Rewards(context.Background(), outputID)
				require.Error(t, err)
				require.True(t, isStatusCode(err, http.StatusNotFound))
				require.Nil(t, resp)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d.requestFromClients(test.testFunc)
		})
	}
}

func getAllValidatorsOnEpoch(t *testing.T, clt *nodeclient.Client, epoch iotago.EpochIndex, pageSize uint64) []string {
	actualValidators := make([]string, 0)
	cursor := ""
	if epoch != 0 {
		cursor = fmt.Sprintf("%d,%d", epoch, 0)
	}

	for {
		resp, err := clt.Validators(context.Background(), pageSize, cursor)
		require.NoError(t, err)

		for _, v := range resp.Validators {
			actualValidators = append(actualValidators, v.AddressBech32)
		}

		cursor = resp.Cursor
		if cursor == "" {
			break
		}
	}

	return actualValidators
}
