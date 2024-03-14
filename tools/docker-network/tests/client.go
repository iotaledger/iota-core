package tests

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/nodeclient"

	iotago "github.com/iotaledger/iota.go/v4"
)

type DockerTestClient struct {
	name    string
	client  *nodeclient.Client
	testing *testing.T
}

func NewDockerTestClient(name string, client *nodeclient.Client) *DockerTestClient {
	return &DockerTestClient{
		name:   name,
		client: client,
	}
}

func (c *DockerTestClient) Name() string {
	return c.name
}

func (c *DockerTestClient) APIForEpoch(epoch iotago.EpochIndex) iotago.API {
	return c.client.APIForEpoch(epoch)
}

func (c *DockerTestClient) APIForSlot(slot iotago.SlotIndex) iotago.API {
	return c.client.APIForSlot(slot)
}

func (c *DockerTestClient) APIForTime(t time.Time) iotago.API {
	return c.client.APIForTime(t)
}

func (c *DockerTestClient) APIForVersion(version iotago.Version) iotago.API {
	api, err := c.client.APIForVersion(version)
	require.NoError(c.testing, err, "failed to get API for version %s", version)

	return api
}

func (c *DockerTestClient) BlockByBlockID(ctx context.Context, blockID iotago.BlockID) (*iotago.Block, error) {
	return c.client.BlockByBlockID(ctx, blockID)
}

func (c *DockerTestClient) BlockIssuance(ctx context.Context) *api.IssuanceBlockHeaderResponse {
	blockIssuanceResponse, err := c.client.BlockIssuance(ctx)
	require.NoError(c.testing, err, "failed to get block issuance response")

	return blockIssuanceResponse
}

func (c *DockerTestClient) BlockIssuer(_ context.Context) nodeclient.BlockIssuerClient {
	panic("not implemented")
}

func (c *DockerTestClient) BlockMetadataByBlockID(ctx context.Context, blockID iotago.BlockID) (*api.BlockMetadataResponse, error) {
	return c.client.BlockMetadataByBlockID(ctx, blockID)
}

func (c *DockerTestClient) BlockWithMetadataByBlockID(ctx context.Context, blockID iotago.BlockID) (*api.BlockWithMetadataResponse, error) {
	return c.client.BlockWithMetadataByBlockID(ctx, blockID)
}

func (c *DockerTestClient) CommitmentByID(ctx context.Context, commitmentID iotago.CommitmentID) *iotago.Commitment {
	commitment, err := c.client.CommitmentByID(ctx, commitmentID)
	require.NoError(c.testing, err, "failed to get commitment by ID %s", commitmentID)

	return commitment
}

func (c *DockerTestClient) CommitmentBySlot(ctx context.Context, slot iotago.SlotIndex) (*iotago.Commitment, error) {
	return c.client.CommitmentBySlot(ctx, slot)
}

func (c *DockerTestClient) CommitmentUTXOChangesByID(ctx context.Context, commitmentID iotago.CommitmentID) *api.UTXOChangesResponse {
	resp, err := c.client.CommitmentUTXOChangesByID(ctx, commitmentID)
	require.NoError(c.testing, err, "failed to get UTXO changes by ID %s", commitmentID)

	return resp
}

func (c *DockerTestClient) CommitmentUTXOChangesBySlot(ctx context.Context, slot iotago.SlotIndex) *api.UTXOChangesResponse {
	resp, err := c.client.CommitmentUTXOChangesBySlot(ctx, slot)
	require.NoError(c.testing, err, "failed to get UTXO changes by slot %d", slot)

	return resp
}

func (c *DockerTestClient) CommitmentUTXOChangesFullByID(ctx context.Context, commitmentID iotago.CommitmentID) *api.UTXOChangesFullResponse {
	resp, err := c.client.CommitmentUTXOChangesFullByID(ctx, commitmentID)
	require.NoError(c.testing, err, "failed to get full UTXO changes by ID %s", commitmentID)

	return resp
}

func (c *DockerTestClient) CommitmentUTXOChangesFullBySlot(ctx context.Context, slot iotago.SlotIndex) *api.UTXOChangesFullResponse {
	resp, err := c.client.CommitmentUTXOChangesFullBySlot(ctx, slot)
	require.NoError(c.testing, err, "failed to get full UTXO changes by slot %d", slot)

	return resp
}

func (c *DockerTestClient) CommittedAPI() iotago.API {
	return c.client.CommittedAPI()
}

func (c *DockerTestClient) Committee(ctx context.Context, optEpochIndex ...iotago.EpochIndex) *api.CommitteeResponse {
	resp, err := c.client.Committee(ctx, optEpochIndex...)
	require.NoError(c.testing, err, "failed to get committee")

	return resp
}

func (c *DockerTestClient) Congestion(ctx context.Context, accountAddress *iotago.AccountAddress, workScore iotago.WorkScore, optCommitmentID ...iotago.CommitmentID) *api.CongestionResponse {
	resp, err := c.client.Congestion(ctx, accountAddress, workScore, optCommitmentID...)
	require.NoError(c.testing, err, "failed to get congestion for account address %s", accountAddress)

	return resp
}

func (c *DockerTestClient) Do(_ context.Context, _ string, _ string, _ interface{}, _ interface{}) *http.Response {
	panic("not implemented")
}

func (c *DockerTestClient) DoWithRequestHeaderHook(_ context.Context, _ string, _ string, _ nodeclient.RequestHeaderHook, _ interface{}, _ interface{}) *http.Response {
	panic("not implemented")
}

func (c *DockerTestClient) EventAPI(_ context.Context) *nodeclient.EventAPIClient {
	panic("not implemented")
}

func (c *DockerTestClient) HTTPClient() *http.Client {
	panic("not implemented")
}

func (c *DockerTestClient) Health(_ context.Context) bool {
	panic("not implemented")
}

func (c *DockerTestClient) Indexer(_ context.Context) nodeclient.IndexerClient {
	panic("not implemented")
}

func (c *DockerTestClient) Info(ctx context.Context) *api.InfoResponse {
	info, err := c.client.Info(ctx)
	require.NoError(c.testing, err, "failed to get info")

	return info
}

func (c *DockerTestClient) LatestAPI() iotago.API {
	return c.client.LatestAPI()
}

func (c *DockerTestClient) Management(_ context.Context) nodeclient.ManagementClient {
	panic("not implemented")
}

func (c *DockerTestClient) NodeSupportsRoute(_ context.Context, _ string) bool {
	panic("not implemented")
}

func (c *DockerTestClient) OutputByID(ctx context.Context, outputID iotago.OutputID) iotago.Output {
	resp, err := c.client.OutputByID(ctx, outputID)
	require.NoError(c.testing, err, "failed to get output by ID %s", outputID)

	return resp
}

func (c *DockerTestClient) OutputMetadataByID(ctx context.Context, outputID iotago.OutputID) *api.OutputMetadata {
	resp, err := c.client.OutputMetadataByID(ctx, outputID)
	require.NoError(c.testing, err, "failed to get output metadata by ID %s", outputID)

	return resp
}

func (c *DockerTestClient) OutputWithMetadataByID(ctx context.Context, outputID iotago.OutputID) (iotago.Output, *api.OutputMetadata) {
	output, outputMetadata, err := c.client.OutputWithMetadataByID(ctx, outputID)
	require.NoError(c.testing, err, "failed to get output with metadata by ID %s", outputID)

	return output, outputMetadata
}

// TODO: check if we should have slot parameter on this interface like we do on rewards endpoint of API.
func (c *DockerTestClient) Rewards(ctx context.Context, outputID iotago.OutputID) *api.ManaRewardsResponse {
	resp, err := c.client.Rewards(ctx, outputID)
	require.NoError(c.testing, err, "failed to get rewards by output ID %s", outputID)

	return resp
}

func (c *DockerTestClient) Routes(_ context.Context) *api.RoutesResponse {
	panic("not implemented")
}

func (c *DockerTestClient) Validator(ctx context.Context, accountAddress *iotago.AccountAddress) *api.ValidatorResponse {
	resp, err := c.client.Validator(ctx, accountAddress)
	require.NoError(c.testing, err, "failed to get staking account by address %s", accountAddress)

	return resp
}

func (c *DockerTestClient) SubmitBlock(ctx context.Context, block *iotago.Block) (iotago.BlockID, error) {
	return c.client.SubmitBlock(ctx, block)
}

func (c *DockerTestClient) TransactionIncludedBlock(ctx context.Context, txID iotago.TransactionID) *iotago.Block {
	block, err := c.client.TransactionIncludedBlock(ctx, txID)
	require.NoError(c.testing, err, "failed to get block by ID %s", txID)

	return block
}

func (c *DockerTestClient) TransactionIncludedBlockMetadata(ctx context.Context, txID iotago.TransactionID) *api.BlockMetadataResponse {
	resp, err := c.client.TransactionIncludedBlockMetadata(ctx, txID)
	require.NoError(c.testing, err, "failed to get block metadata by transaction ID %s", txID)

	return resp
}

func (c *DockerTestClient) TransactionMetadata(ctx context.Context, txID iotago.TransactionID) *api.TransactionMetadataResponse {
	resp, err := c.client.TransactionMetadata(ctx, txID)
	require.NoError(c.testing, err, "failed to get transaction metadata by ID %s", txID)

	return resp
}

func (c *DockerTestClient) Validators(_ context.Context, _ uint64, _ ...string) *api.ValidatorsResponse {
	panic("not implemented")
}

func (c *DockerTestClient) ValidatorsAll(_ context.Context, _ ...int) (validators *api.ValidatorsResponse, allRetrieved bool) {
	panic("not implemented")
}
