package mock

import (
	"context"
	"net/http"
	"time"

	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/nodeclient"

	iotago "github.com/iotaledger/iota.go/v4"
)

type Client interface {
	APIForEpoch(epoch iotago.EpochIndex) iotago.API
	APIForSlot(slot iotago.SlotIndex) iotago.API
	APIForTime(t time.Time) iotago.API
	APIForVersion(version iotago.Version) (iotago.API, error)
	BlockByBlockID(ctx context.Context, blockID iotago.BlockID) (*iotago.Block, error)
	BlockIssuance(ctx context.Context) (*api.IssuanceBlockHeaderResponse, error)
	BlockIssuer(ctx context.Context) (nodeclient.BlockIssuerClient, error)
	BlockMetadataByBlockID(ctx context.Context, blockID iotago.BlockID) (*api.BlockMetadataResponse, error)
	BlockWithMetadataByBlockID(ctx context.Context, blockID iotago.BlockID) (*api.BlockWithMetadataResponse, error)
	CommitmentByID(ctx context.Context, commitmentID iotago.CommitmentID) (*iotago.Commitment, error)
	CommitmentBySlot(ctx context.Context, slot iotago.SlotIndex) (*iotago.Commitment, error)
	CommitmentUTXOChangesByID(ctx context.Context, commitmentID iotago.CommitmentID) (*api.UTXOChangesResponse, error)
	CommitmentUTXOChangesBySlot(ctx context.Context, slot iotago.SlotIndex) (*api.UTXOChangesResponse, error)
	CommitmentUTXOChangesFullByID(ctx context.Context, commitmentID iotago.CommitmentID) (*api.UTXOChangesFullResponse, error)
	CommitmentUTXOChangesFullBySlot(ctx context.Context, slot iotago.SlotIndex) (*api.UTXOChangesFullResponse, error)
	CommittedAPI() iotago.API
	Committee(ctx context.Context, optEpochIndex ...iotago.EpochIndex) (*api.CommitteeResponse, error)
	Congestion(ctx context.Context, accountAddress *iotago.AccountAddress, workScore iotago.WorkScore, optCommitmentID ...iotago.CommitmentID) (*api.CongestionResponse, error)
	Do(ctx context.Context, method string, route string, reqObj interface{}, resObj interface{}) (*http.Response, error)
	DoWithRequestHeaderHook(ctx context.Context, method string, route string, requestHeaderHook nodeclient.RequestHeaderHook, reqObj interface{}, resObj interface{}) (*http.Response, error)
	EventAPI(ctx context.Context) (*nodeclient.EventAPIClient, error)
	HTTPClient() *http.Client
	Health(ctx context.Context) (bool, error)
	Indexer(ctx context.Context) (nodeclient.IndexerClient, error)
	Info(ctx context.Context) (*api.InfoResponse, error)
	LatestAPI() iotago.API
	Management(ctx context.Context) (nodeclient.ManagementClient, error)
	Name() string
	NetworkMetrics(ctx context.Context) (*api.NetworkMetricsResponse, error)
	NodeSupportsRoute(ctx context.Context, route string) (bool, error)
	OutputByID(ctx context.Context, outputID iotago.OutputID) (iotago.Output, error)
	OutputMetadataByID(ctx context.Context, outputID iotago.OutputID) (*api.OutputMetadata, error)
	OutputWithMetadataByID(ctx context.Context, outputID iotago.OutputID) (iotago.Output, *api.OutputMetadata, error)
	Rewards(ctx context.Context, outputID iotago.OutputID) (*api.ManaRewardsResponse, error)
	Routes(ctx context.Context) (*api.RoutesResponse, error)
	SubmitBlock(ctx context.Context, m *iotago.Block) (iotago.BlockID, error)
	TransactionIncludedBlock(ctx context.Context, txID iotago.TransactionID) (*iotago.Block, error)
	TransactionIncludedBlockMetadata(ctx context.Context, txID iotago.TransactionID) (*api.BlockMetadataResponse, error)
	TransactionMetadata(ctx context.Context, txID iotago.TransactionID) (*api.TransactionMetadataResponse, error)
	Validator(ctx context.Context, accountAddress *iotago.AccountAddress) (*api.ValidatorResponse, error)
	Validators(ctx context.Context, pageSize uint64, cursor ...string) (*api.ValidatorsResponse, error)
	ValidatorsAll(ctx context.Context, maxPages ...int) (validators *api.ValidatorsResponse, allRetrieved bool, err error)
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

func (c *TestSuiteClient) APIForVersion(version iotago.Version) (iotago.API, error) {
	return c.Node.Protocol.APIForVersion(version)
}

func (c *TestSuiteClient) BlockByBlockID(_ context.Context, blockID iotago.BlockID) (*iotago.Block, error) {
	return c.Node.RequestHandler.BlockFromBlockID(blockID)
}

func (c *TestSuiteClient) BlockIssuance(_ context.Context) (*api.IssuanceBlockHeaderResponse, error) {
	return c.Node.RequestHandler.BlockIssuance()
}

func (c *TestSuiteClient) BlockIssuer(_ context.Context) (nodeclient.BlockIssuerClient, error) {
	panic("not implemented")
}

func (c *TestSuiteClient) BlockMetadataByBlockID(_ context.Context, blockID iotago.BlockID) (*api.BlockMetadataResponse, error) {
	return c.Node.RequestHandler.BlockMetadataFromBlockID(blockID)
}

func (c *TestSuiteClient) BlockWithMetadataByBlockID(_ context.Context, blockID iotago.BlockID) (*api.BlockWithMetadataResponse, error) {
	return c.Node.RequestHandler.BlockWithMetadataFromBlockID(blockID)
}

func (c *TestSuiteClient) CommitmentByID(_ context.Context, commitmentID iotago.CommitmentID) (*iotago.Commitment, error) {
	commitment, err := c.Node.RequestHandler.GetCommitmentByID(commitmentID)
	if err != nil {
		return nil, err
	}

	return commitment.Commitment(), nil
}

func (c *TestSuiteClient) CommitmentBySlot(_ context.Context, slot iotago.SlotIndex) (*iotago.Commitment, error) {
	commitment, err := c.Node.RequestHandler.GetCommitmentBySlot(slot)

	return commitment.Commitment(), err
}

func (c *TestSuiteClient) CommitmentUTXOChangesByID(_ context.Context, commitmentID iotago.CommitmentID) (*api.UTXOChangesResponse, error) {
	return c.Node.RequestHandler.GetUTXOChangesByCommitmentID(commitmentID)
}

func (c *TestSuiteClient) CommitmentUTXOChangesBySlot(_ context.Context, slot iotago.SlotIndex) (*api.UTXOChangesResponse, error) {
	return c.Node.RequestHandler.GetUTXOChangesBySlot(slot)
}

func (c *TestSuiteClient) CommitmentUTXOChangesFullByID(_ context.Context, commitmentID iotago.CommitmentID) (*api.UTXOChangesFullResponse, error) {
	return c.Node.RequestHandler.GetUTXOChangesFullByCommitmentID(commitmentID)
}

func (c *TestSuiteClient) CommitmentUTXOChangesFullBySlot(_ context.Context, slot iotago.SlotIndex) (*api.UTXOChangesFullResponse, error) {
	return c.Node.RequestHandler.GetUTXOChangesFullBySlot(slot)
}

func (c *TestSuiteClient) CommittedAPI() iotago.API {
	return c.Node.RequestHandler.CommittedAPI()
}

func (c *TestSuiteClient) Committee(_ context.Context, optEpochIndex ...iotago.EpochIndex) (*api.CommitteeResponse, error) {
	var epoch iotago.EpochIndex
	if len(optEpochIndex) == 0 {
		epoch = c.Node.RequestHandler.CommittedAPI().TimeProvider().CurrentEpoch()
	} else {
		epoch = optEpochIndex[0]
	}

	return c.Node.RequestHandler.SelectedCommittee(epoch)
}

func (c *TestSuiteClient) Congestion(_ context.Context, accountAddress *iotago.AccountAddress, workScore iotago.WorkScore, optCommitmentID ...iotago.CommitmentID) (*api.CongestionResponse, error) {
	var commitmentID iotago.CommitmentID
	if len(optCommitmentID) == 0 {
		// passing empty commitmentID to the handler will result in the latest commitment being used
		commitmentID = iotago.EmptyCommitmentID
	} else {
		commitmentID = optCommitmentID[0]
	}

	return c.Node.RequestHandler.CongestionByAccountAddress(accountAddress, workScore, commitmentID)
}

func (c *TestSuiteClient) Do(_ context.Context, _ string, _ string, _ interface{}, _ interface{}) (*http.Response, error) {
	panic("not implemented")
}

func (c *TestSuiteClient) DoWithRequestHeaderHook(_ context.Context, _ string, _ string, _ nodeclient.RequestHeaderHook, _ interface{}, _ interface{}) (*http.Response, error) {
	panic("not implemented")
}

func (c *TestSuiteClient) EventAPI(_ context.Context) (*nodeclient.EventAPIClient, error) {
	panic("not implemented")
}

func (c *TestSuiteClient) HTTPClient() *http.Client {
	panic("not implemented")
}

func (c *TestSuiteClient) Health(_ context.Context) (bool, error) {
	panic("not implemented")
}

func (c *TestSuiteClient) Indexer(_ context.Context) (nodeclient.IndexerClient, error) {
	panic("not implemented")
}

func (c *TestSuiteClient) Info(_ context.Context) (*api.InfoResponse, error) {
	return &api.InfoResponse{
		Status:             c.Node.RequestHandler.GetNodeStatus(),
		ProtocolParameters: c.Node.RequestHandler.GetProtocolParameters(),
	}, nil
}

func (c *TestSuiteClient) LatestAPI() iotago.API {
	return c.Node.RequestHandler.LatestAPI()
}

func (c *TestSuiteClient) Management(_ context.Context) (nodeclient.ManagementClient, error) {
	panic("not implemented")
}

func (c *TestSuiteClient) NetworkMetrics(_ context.Context) (*api.NetworkMetricsResponse, error) {
	panic("not implemented")
}

func (c *TestSuiteClient) NodeSupportsRoute(_ context.Context, _ string) (bool, error) {
	panic("not implemented")
}

func (c *TestSuiteClient) OutputByID(_ context.Context, outputID iotago.OutputID) (iotago.Output, error) {
	resp, err := c.Node.RequestHandler.OutputFromOutputID(outputID)
	if err != nil {
		return nil, err
	}

	return resp.Output, nil
}

func (c *TestSuiteClient) OutputMetadataByID(_ context.Context, outputID iotago.OutputID) (*api.OutputMetadata, error) {
	return c.Node.RequestHandler.OutputMetadataFromOutputID(outputID)
}

func (c *TestSuiteClient) OutputWithMetadataByID(_ context.Context, outputID iotago.OutputID) (iotago.Output, *api.OutputMetadata, error) {
	resp, err := c.Node.RequestHandler.OutputWithMetadataFromOutputID(outputID)
	if err != nil {
		return nil, nil, err
	}

	return resp.Output, resp.Metadata, nil
}

// TODO: check if we should have slot parameter on this interface like we do on rewards endpoint of API.
func (c *TestSuiteClient) Rewards(_ context.Context, outputID iotago.OutputID) (*api.ManaRewardsResponse, error) {
	return c.Node.RequestHandler.RewardsByOutputID(outputID)
}

func (c *TestSuiteClient) Routes(_ context.Context) (*api.RoutesResponse, error) {
	panic("not implemented")
}

func (c *TestSuiteClient) SubmitBlock(ctx context.Context, block *iotago.Block) (iotago.BlockID, error) {
	return c.Node.RequestHandler.SubmitBlockAndAwaitRetainer(ctx, block)
}

func (c *TestSuiteClient) TransactionIncludedBlock(_ context.Context, txID iotago.TransactionID) (*iotago.Block, error) {
	return c.Node.RequestHandler.BlockFromTransactionID(txID)
}

func (c *TestSuiteClient) TransactionIncludedBlockMetadata(_ context.Context, txID iotago.TransactionID) (*api.BlockMetadataResponse, error) {
	return c.Node.RequestHandler.BlockMetadataFromTransactionID(txID)
}

func (c *TestSuiteClient) TransactionMetadata(_ context.Context, txID iotago.TransactionID) (*api.TransactionMetadataResponse, error) {
	return c.Node.RequestHandler.TransactionMetadataFromTransactionID(txID)
}

func (c *TestSuiteClient) Validator(_ context.Context, accountAddress *iotago.AccountAddress) (*api.ValidatorResponse, error) {
	return c.Node.RequestHandler.ValidatorByAccountAddress(accountAddress)
}

func (c *TestSuiteClient) Validators(_ context.Context, _ uint64, _ ...string) (*api.ValidatorsResponse, error) {
	panic("not implemented")
}

func (c *TestSuiteClient) ValidatorsAll(_ context.Context, _ ...int) (*api.ValidatorsResponse, bool, error) {
	panic("not implemented")
}
