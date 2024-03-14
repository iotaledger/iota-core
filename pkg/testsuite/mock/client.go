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
	BlockByBlockID(ctx context.Context, blockID iotago.BlockID) (*iotago.Block, error)
	BlockIssuance(ctx context.Context) *api.IssuanceBlockHeaderResponse
	BlockIssuer(ctx context.Context) nodeclient.BlockIssuerClient
	BlockMetadataByBlockID(ctx context.Context, blockID iotago.BlockID) (*api.BlockMetadataResponse, error)
	BlockWithMetadataByBlockID(ctx context.Context, blockID iotago.BlockID) (*api.BlockWithMetadataResponse, error)
	CommitmentByID(ctx context.Context, commitmentID iotago.CommitmentID) *iotago.Commitment
	CommitmentBySlot(ctx context.Context, slot iotago.SlotIndex) (*iotago.Commitment, error)
	CommitmentUTXOChangesByID(ctx context.Context, commitmentID iotago.CommitmentID) *api.UTXOChangesResponse
	CommitmentUTXOChangesBySlot(ctx context.Context, slot iotago.SlotIndex) *api.UTXOChangesResponse
	CommitmentUTXOChangesFullByID(ctx context.Context, commitmentID iotago.CommitmentID) *api.UTXOChangesFullResponse
	CommitmentUTXOChangesFullBySlot(ctx context.Context, slot iotago.SlotIndex) *api.UTXOChangesFullResponse
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
	Name() string
	NodeSupportsRoute(ctx context.Context, route string) bool
	OutputByID(ctx context.Context, outputID iotago.OutputID) iotago.Output
	OutputMetadataByID(ctx context.Context, outputID iotago.OutputID) *api.OutputMetadata
	OutputWithMetadataByID(ctx context.Context, outputID iotago.OutputID) (iotago.Output, *api.OutputMetadata)
	Rewards(ctx context.Context, outputID iotago.OutputID) *api.ManaRewardsResponse
	Routes(ctx context.Context) *api.RoutesResponse
	SubmitBlock(ctx context.Context, m *iotago.Block) (iotago.BlockID, error)
	TransactionIncludedBlock(ctx context.Context, txID iotago.TransactionID) *iotago.Block
	TransactionIncludedBlockMetadata(ctx context.Context, txID iotago.TransactionID) *api.BlockMetadataResponse
	TransactionMetadata(ctx context.Context, txID iotago.TransactionID) *api.TransactionMetadataResponse
	Validator(ctx context.Context, accountAddress *iotago.AccountAddress) *api.ValidatorResponse
	Validators(ctx context.Context, pageSize uint64, cursor ...string) *api.ValidatorsResponse
	ValidatorsAll(ctx context.Context, maxPages ...int) (validators *api.ValidatorsResponse, allRetrieved bool)
}

type TestSuiteClient struct {
	Node *Node
}

func NewTestSuiteClient(node *Node) *TestSuiteClient {
	return &TestSuiteClient{Node: node}
}

func (c *TestSuiteClient) Name() string {
	return c.Node.Name
}

func (c *TestSuiteClient) APIForEpoch(epoch iotago.EpochIndex) iotago.API {
	return c.Node.Protocol.APIForEpoch(epoch)
}

func (c *TestSuiteClient) APIForSlot(slot iotago.SlotIndex) iotago.API {
	return c.Node.Protocol.APIForSlot(slot)
}

func (c *TestSuiteClient) APIForTime(t time.Time) iotago.API {
	return c.Node.Protocol.APIForTime(t)
}

func (c *TestSuiteClient) APIForVersion(version iotago.Version) iotago.API {
	api, err := c.Node.Protocol.APIForVersion(version)
	require.NoError(c.Node.Testing, err, "failed to get API for version %s", version)

	return api
}

func (c *TestSuiteClient) BlockByBlockID(_ context.Context, blockID iotago.BlockID) (*iotago.Block, error) {
	return c.Node.RequestHandler.BlockFromBlockID(blockID)
}

func (c *TestSuiteClient) BlockIssuance(_ context.Context) *api.IssuanceBlockHeaderResponse {
	blockIssuanceResponse, err := c.Node.RequestHandler.BlockIssuance()
	require.NoError(c.Node.Testing, err, "failed to get block issuance response")

	return blockIssuanceResponse
}

func (c *TestSuiteClient) BlockIssuer(_ context.Context) nodeclient.BlockIssuerClient {
	panic("not implemented")
}

func (c *TestSuiteClient) BlockMetadataByBlockID(_ context.Context, blockID iotago.BlockID) (*api.BlockMetadataResponse, error) {
	return c.Node.RequestHandler.BlockMetadataFromBlockID(blockID)
}

func (c *TestSuiteClient) BlockWithMetadataByBlockID(_ context.Context, blockID iotago.BlockID) (*api.BlockWithMetadataResponse, error) {
	return c.Node.RequestHandler.BlockWithMetadataFromBlockID(blockID)
}

func (c *TestSuiteClient) CommitmentByID(_ context.Context, commitmentID iotago.CommitmentID) *iotago.Commitment {
	commitment, err := c.Node.RequestHandler.GetCommitmentByID(commitmentID)
	require.NoError(c.Node.Testing, err, "failed to get commitment by ID %s", commitmentID)

	return commitment.Commitment()
}

func (c *TestSuiteClient) CommitmentBySlot(_ context.Context, slot iotago.SlotIndex) (*iotago.Commitment, error) {
	commitment, err := c.Node.RequestHandler.GetCommitmentBySlot(slot)

	return commitment.Commitment(), err
}

func (c *TestSuiteClient) CommitmentUTXOChangesByID(_ context.Context, commitmentID iotago.CommitmentID) *api.UTXOChangesResponse {
	resp, err := c.Node.RequestHandler.GetUTXOChangesByCommitmentID(commitmentID)
	require.NoError(c.Node.Testing, err, "failed to get UTXO changes by ID %s", commitmentID)

	return resp
}

func (c *TestSuiteClient) CommitmentUTXOChangesBySlot(_ context.Context, slot iotago.SlotIndex) *api.UTXOChangesResponse {
	resp, err := c.Node.RequestHandler.GetUTXOChangesBySlot(slot)
	require.NoError(c.Node.Testing, err, "failed to get UTXO changes by slot %d", slot)

	return resp
}

func (c *TestSuiteClient) CommitmentUTXOChangesFullByID(_ context.Context, commitmentID iotago.CommitmentID) *api.UTXOChangesFullResponse {
	resp, err := c.Node.RequestHandler.GetUTXOChangesFullByCommitmentID(commitmentID)
	require.NoError(c.Node.Testing, err, "failed to get full UTXO changes by ID %s", commitmentID)

	return resp
}

func (c *TestSuiteClient) CommitmentUTXOChangesFullBySlot(_ context.Context, slot iotago.SlotIndex) *api.UTXOChangesFullResponse {
	resp, err := c.Node.RequestHandler.GetUTXOChangesFullBySlot(slot)
	require.NoError(c.Node.Testing, err, "failed to get full UTXO changes by slot %d", slot)

	return resp
}

func (c *TestSuiteClient) CommittedAPI() iotago.API {
	return c.Node.RequestHandler.CommittedAPI()
}

func (c *TestSuiteClient) Committee(_ context.Context, optEpochIndex ...iotago.EpochIndex) *api.CommitteeResponse {
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

func (c *TestSuiteClient) Congestion(_ context.Context, accountAddress *iotago.AccountAddress, workScore iotago.WorkScore, optCommitmentID ...iotago.CommitmentID) *api.CongestionResponse {
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

func (c *TestSuiteClient) Do(_ context.Context, _ string, _ string, _ interface{}, _ interface{}) *http.Response {
	panic("not implemented")
}

func (c *TestSuiteClient) DoWithRequestHeaderHook(_ context.Context, _ string, _ string, _ nodeclient.RequestHeaderHook, _ interface{}, _ interface{}) *http.Response {
	panic("not implemented")
}

func (c *TestSuiteClient) EventAPI(_ context.Context) *nodeclient.EventAPIClient {
	panic("not implemented")
}

func (c *TestSuiteClient) HTTPClient() *http.Client {
	panic("not implemented")
}

func (c *TestSuiteClient) Health(_ context.Context) bool {
	panic("not implemented")
}

func (c *TestSuiteClient) Indexer(_ context.Context) nodeclient.IndexerClient {
	panic("not implemented")
}

func (c *TestSuiteClient) Info(_ context.Context) *api.InfoResponse {
	return &api.InfoResponse{
		Status:             c.Node.RequestHandler.GetNodeStatus(),
		ProtocolParameters: c.Node.RequestHandler.GetProtocolParameters(),
	}
}

func (c *TestSuiteClient) LatestAPI() iotago.API {
	return c.Node.RequestHandler.LatestAPI()
}

func (c *TestSuiteClient) Management(_ context.Context) nodeclient.ManagementClient {
	panic("not implemented")
}

func (c *TestSuiteClient) NodeSupportsRoute(_ context.Context, _ string) bool {
	panic("not implemented")
}

func (c *TestSuiteClient) OutputByID(_ context.Context, outputID iotago.OutputID) iotago.Output {
	resp, err := c.Node.RequestHandler.OutputFromOutputID(outputID)
	require.NoError(c.Node.Testing, err, "failed to get output by ID %s", outputID)

	return resp.Output
}

func (c *TestSuiteClient) OutputMetadataByID(_ context.Context, outputID iotago.OutputID) *api.OutputMetadata {
	resp, err := c.Node.RequestHandler.OutputMetadataFromOutputID(outputID)
	require.NoError(c.Node.Testing, err, "failed to get output metadata by ID %s", outputID)

	return resp
}

func (c *TestSuiteClient) OutputWithMetadataByID(_ context.Context, outputID iotago.OutputID) (iotago.Output, *api.OutputMetadata) {
	resp, err := c.Node.RequestHandler.OutputWithMetadataFromOutputID(outputID)
	require.NoError(c.Node.Testing, err, "failed to get output with metadata by ID %s", outputID)

	return resp.Output, resp.Metadata
}

// TODO: check if we should have slot parameter on this interface like we do on rewards endpoint of API.
func (c *TestSuiteClient) Rewards(_ context.Context, outputID iotago.OutputID) *api.ManaRewardsResponse {
	resp, err := c.Node.RequestHandler.RewardsByOutputID(outputID)
	require.NoError(c.Node.Testing, err, "failed to get rewards by output ID %s", outputID)

	return resp
}

func (c *TestSuiteClient) Routes(_ context.Context) *api.RoutesResponse {
	panic("not implemented")
}

func (c *TestSuiteClient) SubmitBlock(ctx context.Context, block *iotago.Block) (iotago.BlockID, error) {
	return c.Node.RequestHandler.SubmitBlockAndAwaitBooking(ctx, block)
}

func (c *TestSuiteClient) TransactionIncludedBlock(_ context.Context, txID iotago.TransactionID) *iotago.Block {
	block, err := c.Node.RequestHandler.BlockFromTransactionID(txID)
	require.NoError(c.Node.Testing, err, "failed to get block by ID %s", txID)

	return block
}

func (c *TestSuiteClient) TransactionIncludedBlockMetadata(_ context.Context, txID iotago.TransactionID) *api.BlockMetadataResponse {
	resp, err := c.Node.RequestHandler.BlockMetadataFromTransactionID(txID)
	require.NoError(c.Node.Testing, err, "failed to get block metadata by transaction ID %s", txID)

	return resp
}

func (c *TestSuiteClient) TransactionMetadata(_ context.Context, txID iotago.TransactionID) *api.TransactionMetadataResponse {
	resp, err := c.Node.RequestHandler.TransactionMetadataFromTransactionID(txID)
	require.NoError(c.Node.Testing, err, "failed to get transaction metadata by ID %s", txID)

	return resp
}

func (c *TestSuiteClient) Validator(_ context.Context, accountAddress *iotago.AccountAddress) *api.ValidatorResponse {
	resp, err := c.Node.RequestHandler.ValidatorByAccountAddress(accountAddress)
	require.NoError(c.Node.Testing, err, "failed to get staking account by address %s", accountAddress)

	return resp
}

func (c *TestSuiteClient) Validators(_ context.Context, _ uint64, _ ...string) *api.ValidatorsResponse {
	panic("not implemented")
}

func (c *TestSuiteClient) ValidatorsAll(_ context.Context, _ ...int) (validators *api.ValidatorsResponse, allRetrieved bool) {
	panic("not implemented")
}
