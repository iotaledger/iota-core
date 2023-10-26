package core

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/components/metricstracker"
	"github.com/iotaledger/iota-core/components/protocol"
	"github.com/iotaledger/iota-core/components/restapi"
	"github.com/iotaledger/iota-core/pkg/blockhandler"
	protocolpkg "github.com/iotaledger/iota-core/pkg/protocol"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
)

//nolint:goconst // don't care about the number of constants
const (
	// RouteInfo is the route for getting the node info.
	// GET returns the node info.
	// MIMEApplicationJSON => json.
	// MIMEApplicationVendorIOTASerializerV2 => bytes.
	RouteInfo = "/info"

	// RouteBlockIssuance is the route for getting all needed information for block creation.
	// GET returns the data needed toa attach block.
	// MIMEApplicationJSON => json.
	// MIMEApplicationVendorIOTASerializerV2 => bytes.
	RouteBlockIssuance = "/blocks/issuance"

	// RouteBlock is the route for getting a block by its blockID.
	// GET returns the block based on the given type in the request "Accept" header.
	// MIMEApplicationJSON => json.
	// MIMEApplicationVendorIOTASerializerV2 => bytes.
	RouteBlock = "/blocks/:" + restapipkg.ParameterBlockID

	// RouteBlockMetadata is the route for getting block metadata by its blockID.
	// GET returns block metadata.
	// MIMEApplicationJSON => json.
	// MIMEApplicationVendorIOTASerializerV2 => bytes.
	RouteBlockMetadata = "/blocks/:" + restapipkg.ParameterBlockID + "/metadata"

	// RouteBlocks is the route for sending new blocks.
	// POST creates a single new block and returns the new block ID.
	// The block is parsed based on the given type in the request "Content-Type" header.
	// MIMEApplicationJSON => json.
	// MIMEApplicationVendorIOTASerializerV2 => bytes.
	RouteBlocks = "/blocks"

	// RouteOutput is the route for getting an output by its outputID (transactionHash + outputIndex). This includes the proof, that the output corresponds to the requested outputID.
	// GET returns the output based on the given type in the request "Accept" header.
	// MIMEApplicationJSON => json.
	// MIMEApplicationVendorIOTASerializerV2 => bytes.
	RouteOutput = "/outputs/:" + restapipkg.ParameterOutputID

	// RouteOutputMetadata is the route for getting output metadata by its outputID (transactionHash + outputIndex) without getting the output itself again.
	// GET returns the output metadata.
	// MIMEApplicationJSON => json.
	// MIMEApplicationVendorIOTASerializerV2 => bytes.
	RouteOutputMetadata = "/outputs/:" + restapipkg.ParameterOutputID + "/metadata"

	// RouteOutputWithMetadata is the route for getting output, together with its metadata by its outputID (transactionHash + outputIndex).
	// GET returns the output metadata.
	// MIMEApplicationJSON => json.
	// MIMEApplicationVendorIOTASerializerV2 => bytes.
	RouteOutputWithMetadata = "/outputs/:" + restapipkg.ParameterOutputID + "/full"

	// RouteTransactionsIncludedBlock is the route for getting the block that was first confirmed for a given transaction ID.
	// GET returns the block based on the given type in the request "Accept" header.
	// MIMEApplicationJSON => json.
	// MIMEApplicationVendorIOTASerializerV2 => bytes.
	RouteTransactionsIncludedBlock = "/transactions/:" + restapipkg.ParameterTransactionID + "/included-block"

	// RouteTransactionsIncludedBlockMetadata is the route for getting the block metadata that was first confirmed in the ledger for a given transaction ID.
	// GET returns block metadata (including info about "promotion/reattachment needed").
	// MIMEApplicationJSON => json.
	// MIMEApplicationVendorIOTASerializerV2 => bytes.
	RouteTransactionsIncludedBlockMetadata = "/transactions/:" + restapipkg.ParameterTransactionID + "/included-block/metadata"

	// RouteCommitmentByID is the route for getting a slot commitment by its ID.
	// GET returns the commitment.
	// MIMEApplicationJSON => json.
	// MIMEApplicationVendorIOTASerializerV2 => bytes.
	RouteCommitmentByID = "/commitments/:" + restapipkg.ParameterCommitmentID

	// RouteCommitmentByIDUTXOChanges is the route for getting all UTXO changes of a commitment by its ID.
	// GET returns the output IDs of all UTXO changes.
	// MIMEApplicationJSON => json.
	// MIMEApplicationVendorIOTASerializerV2 => bytes.
	RouteCommitmentByIDUTXOChanges = "/commitments/:" + restapipkg.ParameterCommitmentID + "/utxo-changes"

	// RouteCommitmentByIndex is the route for getting a commitment by its Slot.
	// GET returns the commitment.
	// MIMEApplicationJSON => json.
	// MIMEApplicationVendorIOTASerializerV2 => bytes.
	RouteCommitmentByIndex = "/commitments/by-index/:" + restapipkg.ParameterSlotIndex

	// RouteCommitmentByIndexUTXOChanges is the route for getting all UTXO changes of a commitment by its Slot.
	// GET returns the output IDs of all UTXO changes.
	// MIMEApplicationJSON => json.
	// MIMEApplicationVendorIOTASerializerV2 => bytes.
	RouteCommitmentByIndexUTXOChanges = "/commitments/by-index/:" + restapipkg.ParameterSlotIndex + "/utxo-changes"

	// RouteCongestion is the route for getting the current congestion state and all account related useful details as block issuance credits.
	// GET returns the congestion state related to the specified account.
	// MIMEApplicationJSON => json.
	// MIMEApplicationVendorIOTASerializerV2 => bytes.
	RouteCongestion = "/accounts/:" + restapipkg.ParameterAccountID + "/congestion"

	// RouteValidators is the route for getting informations about the current validators.
	// GET returns the paginated response with the list of validators.
	// MIMEApplicationJSON => json.
	// MIMEApplicationVendorIOTASerializerV2 => bytes.
	RouteValidators = "/validators"

	// RouteValidatorsAccount is the route for getting details about the validator by its accountID.
	// GET returns the validator details.
	// MIMEApplicationJSON => json.
	// MIMEApplicationVendorIOTASerializerV2 => bytes.
	RouteValidatorsAccount = "/validators/:" + restapipkg.ParameterAccountID

	// RouteRewards is the route for getting the rewards for staking or delegation based on staking account or delegation output.
	// Rewards are decayed up to returned epochEnd index.
	// GET returns the rewards.
	// MIMEApplicationJSON => json.
	// MIMEApplicationVendorIOTASerializerV2 => bytes.
	RouteRewards = "/rewards/:" + restapipkg.ParameterOutputID

	// RouteCommittee is the route for getting the current committee.
	// GET returns the committee.
	// MIMEApplicationJSON => json.
	// MIMEApplicationVendorIOTASerializerV2 => bytes.
	RouteCommittee = "/committee"
)

func init() {
	Component = &app.Component{
		Name:      "CoreAPIV3",
		DepsFunc:  func(cDeps dependencies) { deps = cDeps },
		Configure: configure,
		IsEnabled: func(c *dig.Container) bool {
			return restapi.ParamsRestAPI.Enabled
		},
	}
}

var (
	Component *app.Component
	deps      dependencies

	features = []string{}
)

type dependencies struct {
	dig.In

	AppInfo          *app.Info
	RestRouteManager *restapipkg.RestRouteManager
	Protocol         *protocolpkg.Protocol
	BlockHandler     *blockhandler.BlockHandler
	MetricsTracker   *metricstracker.MetricsTracker
	BaseToken        *protocol.BaseToken
}

func configure() error {
	// check if RestAPI plugin is disabled
	if !Component.App().IsComponentEnabled(restapi.Component.Identifier()) {
		Component.LogPanicf("RestAPI plugin needs to be enabled to use the %s plugin", Component.Name)
	}

	routeGroup := deps.RestRouteManager.AddRoute("core/v3")

	routeGroup.GET(RouteInfo, func(c echo.Context) error {
		resp := info()

		return responseByHeader(c, resp)
	})

	routeGroup.GET(RouteBlock, func(c echo.Context) error {
		block, err := blockByID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, block.ProtocolBlock())
	})

	routeGroup.GET(RouteBlockMetadata, func(c echo.Context) error {
		resp, err := blockMetadataByID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.POST(RouteBlocks, func(c echo.Context) error {
		resp, err := sendBlock(c)
		if err != nil {
			return err
		}
		c.Response().Header().Set(echo.HeaderLocation, resp.BlockID.ToHex())

		return httpserver.JSONResponse(c, http.StatusCreated, resp)
	}, checkNodeSynced())

	routeGroup.GET(RouteBlockIssuance, func(c echo.Context) error {
		index, _ := httpserver.ParseSlotQueryParam(c, restapipkg.ParameterSlotIndex)

		resp, err := blockIssuanceBySlot(index)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(RouteCommitmentByID, func(c echo.Context) error {
		index, err := indexByCommitmentID(c)
		if err != nil {
			return err
		}

		commitment, err := getCommitmentDetails(index)
		if err != nil {
			return err
		}

		return responseByHeader(c, commitment)
	})

	routeGroup.GET(RouteCommitmentByIDUTXOChanges, func(c echo.Context) error {
		index, err := indexByCommitmentID(c)
		if err != nil {
			return err
		}

		resp, err := getUTXOChanges(index)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	})

	routeGroup.GET(RouteCommitmentByIndex, func(c echo.Context) error {
		index, err := httpserver.ParseSlotParam(c, restapipkg.ParameterSlotIndex)
		if err != nil {
			return err
		}

		resp, err := getCommitmentDetails(index)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	})

	routeGroup.GET(RouteCommitmentByIndexUTXOChanges, func(c echo.Context) error {
		index, err := httpserver.ParseSlotParam(c, restapipkg.ParameterSlotIndex)
		if err != nil {
			return err
		}

		resp, err := getUTXOChanges(index)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	})

	routeGroup.GET(RouteOutput, func(c echo.Context) error {
		resp, err := getOutput(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	})

	routeGroup.GET(RouteOutputMetadata, func(c echo.Context) error {
		resp, err := getOutputMetadata(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	})

	routeGroup.GET(RouteOutputWithMetadata, func(c echo.Context) error {
		resp, err := getOutputWithMetadata(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	})

	routeGroup.GET(RouteTransactionsIncludedBlock, func(c echo.Context) error {
		block, err := blockByTransactionID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, block.ProtocolBlock())
	})

	routeGroup.GET(RouteTransactionsIncludedBlockMetadata, func(c echo.Context) error {
		resp, err := blockMetadataFromTransactionID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(RouteCongestion, func(c echo.Context) error {
		resp, err := congestionForAccountID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(RouteValidators, func(c echo.Context) error {
		resp, err := validators(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(RouteValidatorsAccount, func(c echo.Context) error {
		resp, err := validatorByAccountID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(RouteRewards, func(c echo.Context) error {
		resp, err := rewardsByOutputID(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(RouteCommittee, func(c echo.Context) error {
		resp := selectedCommittee(c)

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	return nil
}

// AddFeature adds a feature to the RouteInfo endpoint.
func AddFeature(feature string) {
	features = append(features, strings.ToLower(feature))
}

func checkNodeSynced() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if !deps.Protocol.MainEngineInstance().SyncManager.IsNodeSynced() {
				return ierrors.Wrap(echo.ErrServiceUnavailable, "node is not synced")
			}

			return next(c)
		}
	}
}

func responseByHeader(c echo.Context, obj any) error {
	// TODO: that should take the API that belongs to the object
	return httpserver.SendResponseByHeader(c, deps.Protocol.CommittedAPI(), obj)
}
