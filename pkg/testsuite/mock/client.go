package mock

import (
	"context"
	"net/http"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/nodeclient"

	iotago "github.com/iotaledger/iota.go/v4"
)

type Client interface {
	APIForEpoch(epoch iotago.EpochIndex) iotago.API
	APIForSlot(slot iotago.SlotIndex) iotago.API
	APIForTime(t time.Time) iotago.API
	APIForVersion(version iotago.Version) iotago.API
	BlockByBlockID(ctx context.Context, blockID iotago.BlockID) *iotago.Block
	BlockIssuance(ctx context.Context) *api.IssuanceBlockHeaderResponse
	BlockIssuer(ctx context.Context) nodeclient.BlockIssuerClient
	BlockMetadataByBlockID(ctx context.Context, blockID iotago.BlockID) *api.BlockMetadataResponse
	CommitmentByID(ctx context.Context, commitmentID iotago.CommitmentID) *iotago.Commitment
	CommitmentByIndex(ctx context.Context, slot iotago.SlotIndex) *iotago.Commitment
	CommitmentUTXOChangesByID(ctx context.Context, commitmentID iotago.CommitmentID) *api.UTXOChangesResponse
	CommitmentUTXOChangesByIndex(ctx context.Context, slot iotago.SlotIndex) *api.UTXOChangesResponse
	CommitmentUTXOChangesFullByID(ctx context.Context, commitmentID iotago.CommitmentID) *api.UTXOChangesFullResponse
	CommitmentUTXOChangesFullByIndex(ctx context.Context, slot iotago.SlotIndex) *api.UTXOChangesFullResponse
	CommittedAPI() iotago.API
	Committee(ctx context.Context, optEpochIndex ...iotago.EpochIndex) *api.CommitteeResponse
	Congestion(ctx context.Context, accountAddress *iotago.AccountAddress, workScore iotago.WorkScore, optCommitmentID ...iotago.CommitmentID) *api.CongestionResponse
	Do(ctx context.Context, method string, route string, reqObj interface{}, resObj interface{}) *http.Response
	DoWithRequestHeaderHook(ctx context.Context, method string, route string, requestHeaderHook nodeclient.RequestHeaderHook, reqObj interface{}, resObj interface{}) *http.Response
	EventAPI(ctx context.Context) *nodeclient.EventAPIClient
	HTTPClient() *http.Client
	Health(ctx context.Context) bool
	Indexer(ctx context.Context) nodeclient.IndexerClient
	Info(ctx context.Context) *api.InfoResponse
	LatestAPI() iotago.API
	Management(ctx context.Context) nodeclient.ManagementClient
	NodeSupportsRoute(ctx context.Context, route string) bool
	OutputByID(ctx context.Context, outputID iotago.OutputID) iotago.Output
	OutputMetadataByID(ctx context.Context, outputID iotago.OutputID) *api.OutputMetadata
	OutputWithMetadataByID(ctx context.Context, outputID iotago.OutputID) (iotago.Output, *api.OutputMetadata)
	Rewards(ctx context.Context, outputID iotago.OutputID) *api.ManaRewardsResponse
	Routes(ctx context.Context) *api.RoutesResponse
	StakingAccount(ctx context.Context, accountAddress *iotago.AccountAddress) *api.ValidatorResponse
	SubmitBlock(ctx context.Context, m *iotago.Block) (iotago.BlockID, error)
	TransactionIncludedBlock(ctx context.Context, txID iotago.TransactionID) *iotago.Block
	TransactionIncludedBlockMetadata(ctx context.Context, txID iotago.TransactionID) *api.BlockMetadataResponse
	TransactionMetadata(ctx context.Context, txID iotago.TransactionID) *api.TransactionMetadataResponse
	Validators(ctx context.Context, pageSize uint64, cursor ...string) *api.ValidatorsResponse
	ValidatorsAll(ctx context.Context, maxPages ...int) (validators *api.ValidatorsResponse, allRetrieved bool)
}

type TestClient struct {
	Node *Node
}

func (c *TestClient) APIForEpoch(epoch iotago.EpochIndex) iotago.API {
	return c.Node.Protocol.APIForEpoch(epoch)
}

func (c *TestClient) APIForSlot(slot iotago.SlotIndex) iotago.API {
	return c.Node.Protocol.APIForSlot(slot)
}

func (c *TestClient) APIForTime(t time.Time) iotago.API {
	return c.Node.Protocol.APIForTime(t)
}

func (c *TestClient) APIForVersion(version iotago.Version) iotago.API {
	api, err := c.Node.Protocol.APIForVersion(version)
	require.NoError(c.Node.Testing, err, "failed to get API for version %s", version)

	return api
}

func (c *TestClient) BlockByBlockID(ctx context.Context, blockID iotago.BlockID) *iotago.Block {
	block, err := c.Node.RequestHandler.BlockFromBlockID(blockID)
	require.NoError(c.Node.Testing, err, "failed to get block by ID %s", blockID)

	return block
}

func (c *TestClient) BlockIssuance(ctx context.Context) *api.IssuanceBlockHeaderResponse {
	blockIssuanceResponse, err := c.Node.RequestHandler.BlockIssuance()
	require.NoError(c.Node.Testing, err, "failed to get block issuance response")

	return blockIssuanceResponse
}

func (c *TestClient) BlockIssuer(_ context.Context) nodeclient.BlockIssuerClient {
	panic("not implemented")
}

func (c *TestClient) BlockMetadataByBlockID(_ context.Context, blockID iotago.BlockID) *api.BlockMetadataResponse {
	blockMetadataResponse, err := c.Node.RequestHandler.BlockMetadataFromBlockID(blockID)
	require.NoError(c.Node.Testing, err, "failed to get block metadata by ID %s", blockID)

	return blockMetadataResponse
}

func (c *TestClient) CommitmentByID(_ context.Context, commitmentID iotago.CommitmentID) *iotago.Commitment {
	commitment, err := c.Node.RequestHandler.GetCommitmentByID(commitmentID)
	require.NoError(c.Node.Testing, err, "failed to get commitment by ID %s", commitmentID)

	return commitment.Commitment()
}

func (c *TestClient) CommitmentByIndex(_ context.Context, slot iotago.SlotIndex) *iotago.Commitment {
	commitment, err := c.Node.RequestHandler.GetCommitmentBySlot(slot)
	require.NoError(c.Node.Testing, err, "failed to get commitment by slot %d", slot)

	return commitment.Commitment()
}

func (c *TestClient) CommitmentUTXOChangesByID(ctx context.Context, commitmentID iotago.CommitmentID) *api.UTXOChangesResponse {
	resp, err := c.Node.RequestHandler.GetUTXOChangesByCommitmentID(commitmentID)
	require.NoError(c.Node.Testing, err, "failed to get UTXO changes by ID %s", commitmentID)

	return resp
}

func (c *TestClient) CommitmentUTXOChangesByIndex(ctx context.Context, slot iotago.SlotIndex) *api.UTXOChangesResponse {
	resp, err := c.Node.RequestHandler.GetUTXOChangesBySlot(slot)
	require.NoError(c.Node.Testing, err, "failed to get UTXO changes by slot %d", slot)

	return resp
}

func (c *TestClient) CommitmentUTXOChangesFullByID(ctx context.Context, commitmentID iotago.CommitmentID) *api.UTXOChangesFullResponse {
	resp, err := c.Node.RequestHandler.GetUTXOChangesFullByCommitmentID(commitmentID)
	require.NoError(c.Node.Testing, err, "failed to get full UTXO changes by ID %s", commitmentID)

	return resp
}

func (c *TestClient) CommitmentUTXOChangesFullByIndex(ctx context.Context, slot iotago.SlotIndex) *api.UTXOChangesFullResponse {
	resp, err := c.Node.RequestHandler.GetUTXOChangesFullBySlot(slot)
	require.NoError(c.Node.Testing, err, "failed to get full UTXO changes by slot %d", slot)

	return resp
}

func (c *TestClient) CommittedAPI() iotago.API {
	return c.Node.RequestHandler.CommittedAPI()
}

func (c *TestClient) Committee(ctx context.Context, optEpochIndex ...iotago.EpochIndex) *api.CommitteeResponse {
	var epoch iotago.EpochIndex
	if len(optEpochIndex) == 0 {
		epoch = c.Node.RequestHandler.CommittedAPI().TimeProvider().CurrentEpoch()
	} else {
		epoch = optEpochIndex[0]
	}
	resp, err := c.Node.RequestHandler.SelectedCommittee(epoch)
	require.NoError(c.Node.Testing, err, "failed to get committee for epoch %d", epoch)

	return resp
}

func (c *TestClient) Congestion(ctx context.Context, accountAddress *iotago.AccountAddress, workScore iotago.WorkScore, optCommitmentID ...iotago.CommitmentID) *api.CongestionResponse {
	var commitmentID iotago.CommitmentID
	if len(optCommitmentID) == 0 {
		// passing empty commitmentID to the handler will result in the latest commitment being used
		commitmentID = iotago.EmptyCommitmentID
	} else {
		commitmentID = optCommitmentID[0]
	}
	resp, err := c.Node.RequestHandler.CongestionByAccountAddress(accountAddress, workScore, commitmentID)
	require.NoError(c.Node.Testing, err, "failed to get congestion for account address %s", accountAddress)

	return resp
}

func (c *TestClient) Do(ctx context.Context, method string, route string, reqObj interface{}, resObj interface{}) *http.Response {
	panic("not implemented")
}

func (c *TestClient) DoWithRequestHeaderHook(ctx context.Context, method string, route string, requestHeaderHook nodeclient.RequestHeaderHook, reqObj interface{}, resObj interface{}) *http.Response {
	panic("not implemented")
}

func (c *TestClient) EventAPI(ctx context.Context) *nodeclient.EventAPIClient {
	panic("not implemented")
}

func (c *TestClient) HTTPClient() *http.Client {
	panic("not implemented")
}

func (c *TestClient) Health(ctx context.Context) bool {
	panic("not implemented")
}

func (c *TestClient) Indexer(ctx context.Context) nodeclient.IndexerClient {
	panic("not implemented")
}

func (c *TestClient) Info(ctx context.Context) *api.InfoResponse {
	return &api.InfoResponse{
		Status:             c.Node.RequestHandler.GetNodeStatus(),
		ProtocolParameters: c.Node.RequestHandler.GetProtocolParameters(),
	}
}

func (c *TestClient) LatestAPI() iotago.API {
	return c.Node.RequestHandler.LatestAPI()
}

func (c *TestClient) Management(ctx context.Context) nodeclient.ManagementClient {
	panic("not implemented")
}

func (c *TestClient) NodeSupportsRoute(ctx context.Context, route string) bool {
	panic("not implemented")
}

func (c *TestClient) OutputByID(_ context.Context, outputID iotago.OutputID) iotago.Output {
	resp, err := c.Node.RequestHandler.OutputFromOutputID(outputID)
	require.NoError(c.Node.Testing, err, "failed to get output by ID %s", outputID)

	return resp.Output
}

func (c *TestClient) OutputMetadataByID(ctx context.Context, outputID iotago.OutputID) *api.OutputMetadata {
	resp, err := c.Node.RequestHandler.OutputMetadataFromOutputID(outputID)
	require.NoError(c.Node.Testing, err, "failed to get output metadata by ID %s", outputID)

	return resp
}

func (c *TestClient) OutputWithMetadataByID(ctx context.Context, outputID iotago.OutputID) (iotago.Output, *api.OutputMetadata) {
	resp, err := c.Node.RequestHandler.OutputWithMetadataFromOutputID(outputID)
	require.NoError(c.Node.Testing, err, "failed to get output with metadata by ID %s", outputID)

	return resp.Output, resp.Metadata
}

// TODO: check if we should have slot parameter on this interface like we do on rewards endpoint of API.
func (c *TestClient) Rewards(ctx context.Context, outputID iotago.OutputID) *api.ManaRewardsResponse {
	resp, err := c.Node.RequestHandler.RewardsByOutputID(outputID)
	require.NoError(c.Node.Testing, err, "failed to get rewards by output ID %s", outputID)

	return resp
}

func (c *TestClient) Routes(ctx context.Context) *api.RoutesResponse {
	panic("not implemented")
}

func (c *TestClient) StakingAccount(_ context.Context, accountAddress *iotago.AccountAddress) *api.ValidatorResponse {
	resp, err := c.Node.RequestHandler.ValidatorByAccountAddress(accountAddress)
	require.NoError(c.Node.Testing, err, "failed to get staking account by address %s", accountAddress)

	return resp
}

func (c *TestClient) SubmitBlock(ctx context.Context, block *iotago.Block) (iotago.BlockID, error) {
	return c.Node.RequestHandler.SubmitBlockAndAwaitBooking(ctx, block)
}

func (c *TestClient) TransactionIncludedBlock(ctx context.Context, txID iotago.TransactionID) *iotago.Block {
	block, err := c.Node.RequestHandler.BlockFromTransactionID(txID)
	require.NoError(c.Node.Testing, err, "failed to get block by ID %s", txID)

	return block
}

func (c *TestClient) TransactionIncludedBlockMetadata(ctx context.Context, txID iotago.TransactionID) *api.BlockMetadataResponse {
	resp, err := c.Node.RequestHandler.BlockMetadataFromTransactionID(txID)
	require.NoError(c.Node.Testing, err, "failed to get block metadata by transaction ID %s", txID)

	return resp
}

func (c *TestClient) TransactionMetadata(ctx context.Context, txID iotago.TransactionID) *api.TransactionMetadataResponse {
	resp, err := c.Node.RequestHandler.TransactionMetadataFromTransactionID(txID)
	require.NoError(c.Node.Testing, err, "failed to get transaction metadata by ID %s", txID)

	return resp
}

func (c *TestClient) Validators(ctx context.Context, pageSize uint64, cursor ...string) *api.ValidatorsResponse {
	panic("not implemented")
}

func (c *TestClient) ValidatorsAll(ctx context.Context, maxPages ...int) (validators *api.ValidatorsResponse, allRetrieved bool) {
	panic("not implemented")
}
