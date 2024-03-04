package mock

import (
	"context"
	"net/http"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ierrors"
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
	panic("not implemented")
}

func (c *TestClient) APIForSlot(slot iotago.SlotIndex) iotago.API {
	return nil
}

func (c *TestClient) APIForTime(t time.Time) iotago.API {
	return c.Node.Protocol.APIForTime(t)
}

func (c *TestClient) APIForVersion(version iotago.Version) iotago.API {
	api, err := c.Node.Protocol.APIForVersion(version)
	require.NoError(c.Node.Testing, err, "failed to get API for version %s", version)

	return api
}

func (c *TestClient) BlockByBlockID(ctx context.Context, blockID iotago.BlockID) (*iotago.Block, error) {
	loadedBlock, exists := c.Node.Protocol.Engines.Main.Get().Block(blockID)
	if !exists {
		return nil, ierrors.Errorf("block %s does not exist", blockID)
	}

	return loadedBlock.ProtocolBlock(), nil
}

func (c *TestClient) BlockIssuance(ctx context.Context) *api.IssuanceBlockHeaderResponse {
	references := c.Node.Protocol.Engines.Main.Get().TipSelection.SelectTips(iotago.BasicBlockMaxParents)
	require.False(c.Node.Testing, len(references[iotago.StrongParentType]) == 0)

	// get the latest parent block issuing time
	var latestParentBlockIssuingTime time.Time
	for _, parentType := range []iotago.ParentsType{iotago.StrongParentType, iotago.WeakParentType, iotago.ShallowLikeParentType} {
		for _, blockID := range references[parentType] {
			block, exists := c.Node.Protocol.Engines.Main.Get().Block(blockID)
			if !exists {
				continue
			}

			if latestParentBlockIssuingTime.Before(block.ProtocolBlock().Header.IssuingTime) {
				latestParentBlockIssuingTime = block.ProtocolBlock().Header.IssuingTime
			}
		}
	}

	resp := &api.IssuanceBlockHeaderResponse{
		StrongParents:                references[iotago.StrongParentType],
		WeakParents:                  references[iotago.WeakParentType],
		ShallowLikeParents:           references[iotago.ShallowLikeParentType],
		LatestParentBlockIssuingTime: latestParentBlockIssuingTime,
		LatestFinalizedSlot:          c.Node.Protocol.Engines.Main.Get().SyncManager.LatestFinalizedSlot(),
		LatestCommitment:             c.Node.Protocol.Engines.Main.Get().SyncManager.LatestCommitment().Commitment(),
	}

	return resp
}

func (c *TestClient) BlockIssuer(ctx context.Context) nodeclient.BlockIssuerClient {
	panic("not implemented")
}

func (c *TestClient) BlockMetadataByBlockID(ctx context.Context, blockID iotago.BlockID) *api.BlockMetadataResponse {
	panic("not implemented")
}

func (c *TestClient) CommitmentByID(ctx context.Context, commitmentID iotago.CommitmentID) *iotago.Commitment {
	panic("not implemented")
}

func (c *TestClient) CommitmentByIndex(ctx context.Context, slot iotago.SlotIndex) *iotago.Commitment {
	latest := c.Node.Protocol.Engines.Main.Get().SyncManager.LatestCommitment()

	require.True(c.Node.Testing, slot > latest.Slot(), "commitment is from a future slot (%d > %d)", slot, latest.Slot())

	commitment, err := c.Node.Protocol.Engines.Main.Get().Storage.Commitments().Load(slot)
	require.NoError(c.Node.Testing, err, "failed to load commitment, slot: %d", slot)

	return commitment.Commitment()
}

func (c *TestClient) CommitmentUTXOChangesByID(ctx context.Context, commitmentID iotago.CommitmentID) *api.UTXOChangesResponse {
	panic("not implemented")
}

func (c *TestClient) CommitmentUTXOChangesByIndex(ctx context.Context, slot iotago.SlotIndex) *api.UTXOChangesResponse {
	panic("not implemented")
}

func (c *TestClient) CommitmentUTXOChangesFullByID(ctx context.Context, commitmentID iotago.CommitmentID) *api.UTXOChangesFullResponse {
	panic("not implemented")
}

func (c *TestClient) CommitmentUTXOChangesFullByIndex(ctx context.Context, slot iotago.SlotIndex) *api.UTXOChangesFullResponse {
	panic("not implemented")
}

func (c *TestClient) CommittedAPI() iotago.API {
	panic("not implemented")
}

func (c *TestClient) Committee(ctx context.Context, optEpochIndex ...iotago.EpochIndex) *api.CommitteeResponse {
	panic("not implemented")
}

func (c *TestClient) Congestion(ctx context.Context, accountAddress *iotago.AccountAddress, workScore iotago.WorkScore, optCommitmentID ...iotago.CommitmentID) *api.CongestionResponse {
	panic("not implemented")
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
	clSnapshot := c.Node.Protocol.Engines.Main.Get().Clock.Snapshot()
	syncStatus := c.Node.Protocol.Engines.Main.Get().SyncManager.SyncStatus()

	return &api.InfoResponse{
		Status: &api.InfoResNodeStatus{
			IsHealthy:                   syncStatus.NodeSynced,
			AcceptedTangleTime:          clSnapshot.AcceptedTime,
			RelativeAcceptedTangleTime:  clSnapshot.RelativeAcceptedTime,
			ConfirmedTangleTime:         clSnapshot.ConfirmedTime,
			RelativeConfirmedTangleTime: clSnapshot.RelativeConfirmedTime,
			LatestCommitmentID:          syncStatus.LatestCommitment.ID(),
			LatestFinalizedSlot:         syncStatus.LatestFinalizedSlot,
			LatestAcceptedBlockSlot:     syncStatus.LastAcceptedBlockSlot,
			LatestConfirmedBlockSlot:    syncStatus.LastConfirmedBlockSlot,
			PruningEpoch:                syncStatus.LastPrunedEpoch,
		},
		ProtocolParameters: c.protocolParameters(),
	}
}

func (c *TestClient) protocolParameters() []*api.InfoResProtocolParameters {
	protoParams := make([]*api.InfoResProtocolParameters, 0)
	provider := c.Node.Protocol.Engines.Main.Get().Storage.Settings().APIProvider()
	for _, version := range provider.ProtocolEpochVersions() {
		protocolParams := provider.ProtocolParameters(version.Version)
		if protocolParams == nil {
			continue
		}

		protoParams = append(protoParams, &api.InfoResProtocolParameters{
			StartEpoch: version.StartEpoch,
			Parameters: protocolParams,
		})
	}

	return protoParams
}

func (c *TestClient) LatestAPI() iotago.API {
	return c.Node.Protocol.LatestAPI()
}

func (c *TestClient) Management(ctx context.Context) nodeclient.ManagementClient {
	panic("not implemented")
}

func (c *TestClient) NodeSupportsRoute(ctx context.Context, route string) bool {
	panic("not implemented")
}

func (c *TestClient) OutputByID(ctx context.Context, outputID iotago.OutputID) iotago.Output {
	panic("not implemented")
}

func (c *TestClient) OutputMetadataByID(ctx context.Context, outputID iotago.OutputID) *api.OutputMetadata {
	panic("not implemented")
}

func (c *TestClient) OutputWithMetadataByID(ctx context.Context, outputID iotago.OutputID) (iotago.Output, *api.OutputMetadata) {
	panic("not implemented")
}

func (c *TestClient) Rewards(ctx context.Context, outputID iotago.OutputID) *api.ManaRewardsResponse {
	panic("not implemented")
}

func (c *TestClient) Routes(ctx context.Context) *api.RoutesResponse {
	panic("not implemented")
}

func (c *TestClient) StakingAccount(ctx context.Context, accountAddress *iotago.AccountAddress) *api.ValidatorResponse {
	panic("not implemented")
}

func (c *TestClient) SubmitBlock(ctx context.Context, block *iotago.Block) (iotago.BlockID, error) {
	return c.Node.BlockHandler.SubmitBlockAndAwaitBooking(ctx, block)
}

func (c *TestClient) TransactionIncludedBlock(ctx context.Context, txID iotago.TransactionID) *iotago.Block {
	panic("not implemented")
}

func (c *TestClient) TransactionIncludedBlockMetadata(ctx context.Context, txID iotago.TransactionID) *api.BlockMetadataResponse {
	panic("not implemented")
}

func (c *TestClient) TransactionMetadata(ctx context.Context, txID iotago.TransactionID) *api.TransactionMetadataResponse {
	panic("not implemented")
}

func (c *TestClient) Validators(ctx context.Context, pageSize uint64, cursor ...string) *api.ValidatorsResponse {
	panic("not implemented")
}

func (c *TestClient) ValidatorsAll(ctx context.Context, maxPages ...int) (validators *api.ValidatorsResponse, allRetrieved bool) {
	panic("not implemented")
}
