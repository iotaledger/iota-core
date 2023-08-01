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
	"github.com/iotaledger/iota-core/pkg/blockfactory"
	protocolpkg "github.com/iotaledger/iota-core/pkg/protocol"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
)

const (
	// RouteInfo is the route for getting the node info.
	// GET returns the node info.
	RouteInfo = "/info"

	// RouteBlockIssuance is the route for getting all needed information for block creation.
	// GET returns the data needed toa attach block.
	// MIMEApplicationJSON => json.
	// MIMEVendorIOTASerializer => bytes.
	RouteBlockIssuance = "/blocks/issuance"

	// RouteBlock is the route for getting a block by its blockID.
	// GET returns the block based on the given type in the request "Accept" header.
	// MIMEApplicationJSON => json.
	// MIMEVendorIOTASerializer => bytes.
	RouteBlock = "/blocks/:" + restapipkg.ParameterBlockID

	// RouteBlockMetadata is the route for getting block metadata by its blockID.
	// GET returns block metadata.
	RouteBlockMetadata = "/blocks/:" + restapipkg.ParameterBlockID + "/metadata"

	// RouteBlocks is the route for creating new blocks.
	// POST creates a single new block and returns the new block ID.
	// The block is parsed based on the given type in the request "Content-Type" header.
	// By providing only the protocolVersion and payload transaction user can POST a transaction.
	// MIMEApplicationJSON => json.
	// MIMEVendorIOTASerializer => bytes.
	RouteBlocks = "/blocks"

	// RouteOutput is the route for getting an output by its outputID (transactionHash + outputIndex).
	// GET returns the output based on the given type in the request "Accept" header.
	// MIMEApplicationJSON => json.
	// MIMEVendorIOTASerializer => bytes.
	RouteOutput = "/outputs/:" + restapipkg.ParameterOutputID

	// RouteOutputMetadata is the route for getting output metadata by its outputID (transactionHash + outputIndex) without getting the data again.
	// GET returns the output metadata.
	RouteOutputMetadata = "/outputs/:" + restapipkg.ParameterOutputID + "/metadata"

	// RouteTransactionsIncludedBlock is the route for getting the block that was first confirmed for a given transaction ID.
	// GET returns the block based on the given type in the request "Accept" header.
	// MIMEApplicationJSON => json.
	// MIMEVendorIOTASerializer => bytes.
	RouteTransactionsIncludedBlock = "/transactions/:" + restapipkg.ParameterTransactionID + "/included-block"

	// RouteTransactionsIncludedBlockMetadata is the route for getting the block metadata that was first confirmed in the ledger for a given transaction ID.
	// GET returns block metadata (including info about "promotion/reattachment needed").
	RouteTransactionsIncludedBlockMetadata = "/transactions/:" + restapipkg.ParameterTransactionID + "/included-block/metadata"

	// RouteCommitmentByID is the route for getting a slot commitment by its ID.
	// GET returns the commitment.
	// MIMEApplicationJSON => json.
	// MIMEVendorIOTASerializer => bytes.
	RouteCommitmentByID = "/commitments/:" + restapipkg.ParameterCommitmentID

	// RouteCommitmentByIDUTXOChanges is the route for getting all UTXO changes of a commitment by its ID.
	// GET returns the output IDs of all UTXO changes.
	RouteCommitmentByIDUTXOChanges = "/commitments/:" + restapipkg.ParameterCommitmentID + "/utxo-changes"

	// RouteCommitmentByIndex is the route for getting a commitment by its SlotIndex.
	// GET returns the commitment.
	// MIMEApplicationJSON => json.
	// MIMEVendorIOTASerializer => bytes.
	RouteCommitmentByIndex = "/commitments/by-index/:" + restapipkg.ParameterSlotIndex

	// RouteCommitmentByIndexUTXOChanges is the route for getting all UTXO changes of a commitment by its SlotIndex.
	// GET returns the output IDs of all UTXO changes.
	RouteCommitmentByIndexUTXOChanges = "/commitments/by-index/:" + restapipkg.ParameterSlotIndex + "/utxo-changes"

	// RouteCongestion is the route for getting the current congestion state and all account related useful details as block issuance credits.
	// GET returns the congestion state related to the specified account.
	RouteCongestion = "/accounts/:" + restapipkg.ParameterAccountID + "/congestion"

	// RouteStaking is the route for getting informations about the current stakers.
	// GET returns the stakers.
	RouteStaking = "/staking"

	// RouteStakingAccount is the route for getting an account by its accountID.
	// GET returns the account details.
	RouteStakingAccount = "/staking/:" + restapipkg.ParameterAccountID

	// RouteRewards is the route for getting the rewards for staking or delegation based on staking account or delegation output.
	// GET returns the rewards.
	RouteRewards = "/rewards/:" + restapipkg.ParameterOutputID

	// RouteCommittee is the route for getting the current committee.
	// GET returns the committee.
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
	BlockIssuer      *blockfactory.BlockIssuer `optional:"true"`
	MetricsTracker   *metricstracker.MetricsTracker
	BaseToken        *protocol.BaseToken
}

func configure() error {
	// check if RestAPI plugin is disabled
	if !Component.App().IsComponentEnabled(restapi.Component.Identifier()) {
		Component.LogPanicf("RestAPI plugin needs to be enabled to use the %s plugin", Component.Name)
	}

	routeGroup := deps.RestRouteManager.AddRoute("core/v3")

	if restapi.ParamsRestAPI.AllowIncompleteBlock {
		AddFeature("allowIncompleteBlock")
	}

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
		// TODO: fill in blockReason, TxState, TxReason.
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
	}, checkNodeSynced(), checkUpcomingUnsupportedProtocolVersion())

	routeGroup.GET(RouteBlockIssuance, func(c echo.Context) error {
		resp, err := blockIssuance(c)
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
		output, err := getOutput(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, output.Output())
	})

	routeGroup.GET(RouteOutputMetadata, func(c echo.Context) error {
		resp, err := getOutputMetadata(c)
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

	routeGroup.GET(RouteStaking, func(c echo.Context) error {
		resp, err := staking()
		if err != nil {
			return err
		}

		return responseByHeader(c, resp)
	}, checkNodeSynced())

	routeGroup.GET(RouteStakingAccount, func(c echo.Context) error {
		resp, err := stakingByAccountID(c)
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
			if !deps.Protocol.SyncManager.IsNodeSynced() {
				return ierrors.Wrap(echo.ErrServiceUnavailable, "node is not synced")
			}

			return next(c)
		}
	}
}

func checkUpcomingUnsupportedProtocolVersion() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// todo update with protocol upgrades support
			// if !deps.ProtocolManager.NextPendingSupported() {
			//	return ierrors.Wrap(echo.ErrServiceUnavailable, "node does not support the upcoming protocol upgrade")
			// }

			return next(c)
		}
	}
}

func responseByHeader(c echo.Context, obj any) error {
	mimeType, err := httpserver.GetAcceptHeaderContentType(c, httpserver.MIMEApplicationVendorIOTASerializerV1, echo.MIMEApplicationJSON)
	if err != nil && err != httpserver.ErrNotAcceptable {
		return err
	}

	switch mimeType {
	// TODO: should this maybe already be V2 ?
	case httpserver.MIMEApplicationVendorIOTASerializerV1:
		// TODO: that should take the API that belongs to the object
		b, err := deps.Protocol.CurrentAPI().Encode(obj)
		if err != nil {
			return err
		}

		return c.Blob(http.StatusOK, httpserver.MIMEApplicationVendorIOTASerializerV1, b)

	// default to echo.MIMEApplicationJSON
	default:
		// TODO: that should take the API that belongs to the object
		j, err := deps.Protocol.CurrentAPI().JSONEncode(obj)
		if err != nil {
			return err
		}

		return c.Blob(http.StatusOK, echo.MIMEApplicationJSON, j)
	}
}
