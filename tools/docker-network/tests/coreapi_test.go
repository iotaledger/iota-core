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

func Test_ValidatorAPI(t *testing.T) {
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

	d.WaitUntilNetworkReady()

	ctx := context.Background()

	prePreparedAssets := struct {
		slot           iotago.SlotIndex
		epoch          iotago.EpochIndex
		dataBlock      *iotago.Block
		valueBlock     *iotago.Block
		transaction    *iotago.SignedTransaction
		transactionID  iotago.TransactionID
		basicOutput    iotago.Output
		basicOutputID  iotago.OutputID
		accountAddress *iotago.AccountAddress
	}{
		slot: iotago.SlotIndex(2),
	}

	sharedAssets := struct {
		commitmentIDs map[string]iotago.CommitmentID
	}{}

	clt := d.Node("V1").Client
	prePreparedAssets.epoch = clt.APIForSlot(prePreparedAssets.slot).TimeProvider().EpochFromSlot(prePreparedAssets.slot)
	prePreparedAssets.dataBlock = d.CreateTaggedDataBlock(d.Node("V1").AccountAddress(t).AccountID(), []byte{'t', 'a', 'g'})

	block, signedTx, basicOut := d.CreateValueBlock("V1")
	d.SubmitBlock(ctx, block)

	prePreparedAssets.valueBlock = block
	prePreparedAssets.basicOutput = basicOut
	prePreparedAssets.transaction = signedTx
	txID, err := signedTx.Transaction.ID()
	require.NoError(d.Testing, err)
	prePreparedAssets.transactionID = txID
	prePreparedAssets.basicOutputID = iotago.OutputIDFromTransactionIDAndIndex(txID, 0)

	account := d.CreateAccount()
	prePreparedAssets.accountAddress = account.Address

	d.AwaitCommitment(prePreparedAssets.slot)

	tests := []struct {
		name     string
		testFunc func(t *testing.T, nodeAlias string)
	}{
		{
			name: "Test_Info",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.Node(nodeAlias).Client.Info(context.Background())
				require.NoError(t, err)
				require.NotNil(t, resp)

				fmt.Println(resp)
			},
		},
		{
			name: "Test_BlockByBlockID",
			testFunc: func(t *testing.T, nodeAlias string) {
				respBlock, err := d.Node(nodeAlias).Client.BlockByBlockID(context.Background(), prePreparedAssets.dataBlock.MustID())
				require.NoError(t, err)
				require.NotNil(t, respBlock)
				require.Equal(t, prePreparedAssets.dataBlock, respBlock)
			},
		},
		{
			name: "Test_BlockMetadataByBlockID",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.Node(nodeAlias).Client.BlockMetadataByBlockID(context.Background(), prePreparedAssets.dataBlock.MustID())
				require.NoError(t, err)
				require.NotNil(t, resp)

				require.Equal(t, prePreparedAssets.dataBlock.MustID(), resp.BlockID)
				require.Equal(t, api.BlockStateFinalized, resp.BlockState)
			},
		},
		{
			name: "Test_BlockWithMetadata",
			testFunc: func(t *testing.T, nodeAlias string) {
				// TODO missing client side implementation
				resp, err := d.Node(nodeAlias).Client.BlockMetadataByBlockID(context.Background(), prePreparedAssets.dataBlock.MustID())
				require.NoError(t, err)
				require.NotNil(t, resp)
			},
		},
		{
			name: "Test_BlockIssuance",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.Node(nodeAlias).Client.BlockIssuance(context.Background())
				require.NoError(t, err)
				require.NotNil(t, resp)

				require.GreaterOrEqual(t, resp.StrongParents, 1)
			},
		},
		{
			name: "Test_CommitmentBySlot",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.Node(nodeAlias).Client.CommitmentByIndex(context.Background(), prePreparedAssets.slot)
				require.NoError(t, err)
				require.NotNil(t, resp)
				sharedAssets.commitmentIDs[nodeAlias] = resp.MustID()
			},
		},
		{
			name: "Test_CommitmentByID",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.Node(nodeAlias).Client.CommitmentByID(context.Background(), sharedAssets.commitmentIDs[nodeAlias])
				require.NoError(t, err)
				require.NotNil(t, resp)
				require.Equal(t, sharedAssets.commitmentIDs[nodeAlias], resp.MustID())
			},
		},
		{
			name: "Test_CommitmentUTXOChangesByID",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.Node(nodeAlias).Client.CommitmentUTXOChangesByID(context.Background(), sharedAssets.commitmentIDs[nodeAlias])
				require.NoError(t, err)
				require.NotNil(t, resp)
				fmt.Println(resp)
			},
		},
		{
			name: "Test_CommitmentUTXOChangesFullByID",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.Node(nodeAlias).Client.CommitmentUTXOChangesFullByID(context.Background(), sharedAssets.commitmentIDs[nodeAlias])
				require.NoError(t, err)
				require.NotNil(t, resp)
				fmt.Println(resp)
			},
		},
		{
			name: "Test_CommitmentUTXOChangesBySlot",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.Node(nodeAlias).Client.CommitmentUTXOChangesByIndex(context.Background(), prePreparedAssets.slot)
				require.NoError(t, err)
				require.NotNil(t, resp)
				// check by what I have sent

				resp, err = d.Node(nodeAlias).Client.CommitmentUTXOChangesByIndex(context.Background(), sharedAssets.commitmentIDs[nodeAlias].Slot())
				require.NoError(t, err)
				require.NotNil(t, resp)
				require.Equal(t, sharedAssets.commitmentIDs[nodeAlias], resp.CommitmentID)

				// todo check with by commitment resp

			},
		},
		{
			name: "Test_CommitmentUTXOChangesFullBySlot",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.Node(nodeAlias).Client.CommitmentUTXOChangesFullByIndex(context.Background(), prePreparedAssets.slot)
				require.NoError(t, err)
				require.NotNil(t, resp)
			},
		},
		{
			name: "Test_OutputByID",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.Node(nodeAlias).Client.OutputByID(context.Background(), prePreparedAssets.basicOutputID)
				require.NoError(t, err)
				require.NotNil(t, resp)
				// todo calculate outID
			},
		},
		{
			name: "Test_OutputMetadata",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.Node(nodeAlias).Client.OutputMetadataByID(context.Background(), prePreparedAssets.basicOutputID)
				require.NoError(t, err)
				require.NotNil(t, resp)
			},
		},
		{
			name: "Test_TransactionsIncludedBlock",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.Node(nodeAlias).Client.TransactionIncludedBlock(context.Background(), prePreparedAssets.transactionID)
				require.NoError(t, err)
				require.NotNil(t, resp)

				// todo issue second block with the same tx, and make sure that the first one is returned here
			},
		},
		{
			name: "Test_TransactionsIncludedBlockMetadata",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.Node(nodeAlias).Client.TransactionIncludedBlockMetadata(context.Background(), prePreparedAssets.transactionID)
				require.NoError(t, err)
				require.NotNil(t, resp)
			},
		},
		{
			name: "Test_TransactionsMetadata",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.Node(nodeAlias).Client.TransactionMetadata(context.Background(), prePreparedAssets.transactionID)
				require.NoError(t, err)
				require.NotNil(t, resp)
			},
		},
		{
			name: "Test_Congestion",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.Node(nodeAlias).Client.Congestion(context.Background(), d.Node(nodeAlias).AccountAddress(t), 0)
				require.NoError(t, err)
				require.NotNil(t, resp)

				resp, err = d.Node(nodeAlias).Client.Congestion(context.Background(), prePreparedAssets.accountAddress, 0, sharedAssets.commitmentIDs[nodeAlias])
				require.NoError(t, err)
				require.NotNil(t, resp)
			},
		},
		{
			name: "Test_Validators",
			testFunc: func(t *testing.T, nodeAlias string) {
				pageSize := uint64(3)
				resp, err := d.Node(nodeAlias).Client.Validators(context.Background(), pageSize)
				require.NoError(t, err)
				require.NotNil(t, resp)
				require.Equal(t, len(resp.Validators), int(pageSize))

				resp, err = d.Node(nodeAlias).Client.Validators(context.Background(), pageSize, resp.Cursor)
				require.NoError(t, err)
				require.NotNil(t, resp)
				require.Equal(t, len(resp.Validators), 1)
			},
		},
		{
			name: "Test_ValidatorsAll",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, all, err := d.Node(nodeAlias).Client.ValidatorsAll(context.Background())
				require.NoError(t, err)
				require.True(t, all)
				require.Equal(t, 4, len(resp.Validators))
			},
		},
		{
			name: "Test_Rewards",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.Node(nodeAlias).Client.Rewards(context.Background(), prePreparedAssets.basicOutputID)
				require.NoError(t, err)
				require.NotNil(t, resp)
			},
		},
		{
			name: "Test_Committee",
			testFunc: func(t *testing.T, nodeAlias string) {
				resp, err := d.Node(nodeAlias).Client.Committee(context.Background(), prePreparedAssets.epoch)
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

	// TDO Assert shared assets are the same
}
