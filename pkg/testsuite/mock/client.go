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
	SubmitBlock(ctx context.Context, m *iotago.Block) iotago.BlockID
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
	return nil
}

func (c *TestClient) APIForSlot(slot iotago.SlotIndex) iotago.API {
	return nil
}

func (c *TestClient) APIForTime(t time.Time) iotago.API {
	return nil
}

func (c *TestClient) APIForVersion(version iotago.Version) iotago.API {
	return nil
}

func (c *TestClient) BlockByBlockID(ctx context.Context, blockID iotago.BlockID) *iotago.Block {
	return nil
}

func (c *TestClient) BlockIssuance(ctx context.Context) *api.IssuanceBlockHeaderResponse {
	return nil
}

func (c *TestClient) BlockIssuer(ctx context.Context) nodeclient.BlockIssuerClient {
	return nil
}

func (c *TestClient) BlockMetadataByBlockID(ctx context.Context, blockID iotago.BlockID) *api.BlockMetadataResponse {
	return nil
}

func (c *TestClient) CommitmentByID(ctx context.Context, commitmentID iotago.CommitmentID) *iotago.Commitment {
	return nil
}

func (c *TestClient) CommitmentByIndex(ctx context.Context, slot iotago.SlotIndex) *iotago.Commitment {
	return nil
}

func (c *TestClient) CommitmentUTXOChangesByID(ctx context.Context, commitmentID iotago.CommitmentID) *api.UTXOChangesResponse {
	return nil
}

func (c *TestClient) CommitmentUTXOChangesByIndex(ctx context.Context, slot iotago.SlotIndex) *api.UTXOChangesResponse {
	return nil
}

func (c *TestClient) CommitmentUTXOChangesFullByID(ctx context.Context, commitmentID iotago.CommitmentID) *api.UTXOChangesFullResponse {
	return nil
}

func (c *TestClient) CommitmentUTXOChangesFullByIndex(ctx context.Context, slot iotago.SlotIndex) *api.UTXOChangesFullResponse {
	return nil
}

func (c *TestClient) CommittedAPI() iotago.API {
	return nil
}

func (c *TestClient) Committee(ctx context.Context, optEpochIndex ...iotago.EpochIndex) *api.CommitteeResponse {
	return nil
}

func (c *TestClient) Congestion(ctx context.Context, accountAddress *iotago.AccountAddress, workScore iotago.WorkScore, optCommitmentID ...iotago.CommitmentID) *api.CongestionResponse {
	return nil
}

func (c *TestClient) Do(ctx context.Context, method string, route string, reqObj interface{}, resObj interface{}) *http.Response {
	return nil
}

func (c *TestClient) DoWithRequestHeaderHook(ctx context.Context, method string, route string, requestHeaderHook nodeclient.RequestHeaderHook, reqObj interface{}, resObj interface{}) *http.Response {
	return nil
}

func (c *TestClient) EventAPI(ctx context.Context) *nodeclient.EventAPIClient {
	return nil
}

func (c *TestClient) HTTPClient() *http.Client {
	return nil
}

func (c *TestClient) Health(ctx context.Context) bool {
	return false
}

func (c *TestClient) Indexer(ctx context.Context) nodeclient.IndexerClient {
	return nil
}

func (c *TestClient) Info(ctx context.Context) *api.InfoResponse {
	return nil
}

func (c *TestClient) LatestAPI() iotago.API {
	return nil
}

func (c *TestClient) Management(ctx context.Context) nodeclient.ManagementClient {
	return nil
}

func (c *TestClient) NodeSupportsRoute(ctx context.Context, route string) bool {
	return false
}

func (c *TestClient) OutputByID(ctx context.Context, outputID iotago.OutputID) iotago.Output {
	return nil
}

func (c *TestClient) OutputMetadataByID(ctx context.Context, outputID iotago.OutputID) *api.OutputMetadata {
	return nil
}

func (c *TestClient) OutputWithMetadataByID(ctx context.Context, outputID iotago.OutputID) (iotago.Output, *api.OutputMetadata) {
	return nil, nil
}

func (c *TestClient) Rewards(ctx context.Context, outputID iotago.OutputID) *api.ManaRewardsResponse {
	return nil
}

func (c *TestClient) Routes(ctx context.Context) *api.RoutesResponse {
	return nil
}

func (c *TestClient) StakingAccount(ctx context.Context, accountAddress *iotago.AccountAddress) *api.ValidatorResponse {
	return nil
}

func (c *TestClient) SubmitBlock(ctx context.Context, m *iotago.Block) iotago.BlockID {
	return iotago.EmptyBlockID
}

func (c *TestClient) TransactionIncludedBlock(ctx context.Context, txID iotago.TransactionID) *iotago.Block {
	return nil
}

func (c *TestClient) TransactionIncludedBlockMetadata(ctx context.Context, txID iotago.TransactionID) *api.BlockMetadataResponse {
	return nil
}

func (c *TestClient) TransactionMetadata(ctx context.Context, txID iotago.TransactionID) *api.TransactionMetadataResponse {
	return nil
}

func (c *TestClient) Validators(ctx context.Context, pageSize uint64, cursor ...string) *api.ValidatorsResponse {
	return nil
}

func (c *TestClient) ValidatorsAll(ctx context.Context, maxPages ...int) (validators *api.ValidatorsResponse, allRetrieved bool) {
	return nil, false
}
