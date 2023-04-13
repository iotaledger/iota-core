package coreapi

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/components/restapi"
	"github.com/iotaledger/iota-core/pkg/protocol"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
)

const (
	// RouteInfo is the route for getting the node info.
	// GET returns the node info.
	RouteInfo = "/info"

	// RouteBlockIssuance is the route for getting all needed information for block creation.
	// GET returns the data needed toa attach block.
	RouteBlockIssuance = "/issuance/block-header"

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

	// RouteTransactionsIncludedBlock is the route for getting the block that was included in the ledger for a given transaction ID.
	// GET returns the block based on the given type in the request "Accept" header.
	// MIMEApplicationJSON => json.
	// MIMEVendorIOTASerializer => bytes.
	RouteTransactionsIncludedBlock = "/transactions/:" + restapipkg.ParameterTransactionID + "/included-block"

	// RouteTransactionsIncludedBlockMetadata is the route for getting the block metadata that was included in the ledger for a given transaction ID.
	// GET returns block metadata (including info about "promotion/reattachment needed").
	RouteTransactionsIncludedBlockMetadata = "/transactions/:" + restapipkg.ParameterTransactionID + "/included-block/metadata"

	// RouteCommitmentByID is the route for getting a slot commitment by its ID.
	// GET returns the commitment.
	// MIMEApplicationJSON => json.
	// MIMEVendorIOTASerializer => bytes.
	RouteCommitmentByID = "/commitment/:" + restapipkg.ParameterCommitmentID

	// RouteCommitmentByIDUTXOChanges is the route for getting all UTXO changes of a milestone by its ID.
	// GET returns the output IDs of all UTXO changes.
	RouteCommitmentByIDUTXOChanges = "/commitment/:" + restapipkg.ParameterCommitmentID + "/utxo-changes"

	// RouteSlotByIndex is the route for getting a milestone by its milestoneIndex.
	// GET returns the milestone.
	// MIMEApplicationJSON => json.
	// MIMEVendorIOTASerializer => bytes.
	RouteSlotByIndex = "/commitment/by-index/:" + restapipkg.ParameterSlotIndex

	// RouteSlotByIndexUTXOChanges is the route for getting all UTXO changes of a milestone by its milestoneIndex.
	// GET returns the output IDs of all UTXO changes.
	RouteSlotByIndexUTXOChanges = "/commitment/by-index/:" + restapipkg.ParameterSlotIndex + "/utxo-changes"

	// RouteAccountsByAcciuntID is the route for getting an account by its accountID.
	// GET returns the account details.
	RouteAccountsByAcciuntID = "/accounts/:" + restapipkg.ParameterAccountID

	// RouteAccountMana is the route for getting an account mana by its accountID.
	// GET returns the account mana details.
	RouteAccountMana = "/accounts/:" + restapipkg.ParameterAccountID + "/mana"

	// RoutePeer is the route for getting peers by their peerID.
	// GET returns the peer
	// DELETE deletes the peer.
	RoutePeer = "/peers/:" + restapipkg.ParameterPeerID

	// RoutePeers is the route for getting all peers of the node.
	// GET returns a list of all peers.
	// POST adds a new peer.
	RoutePeers = "/peers"

	// RouteControlDatabasePrune is the control route to manually prune the database.
	// POST prunes the database.
	RouteControlDatabasePrune = "/control/database/prune"

	// RouteControlSnapshotsCreate is the control route to manually create a snapshot files.
	// POST creates a full snapshot.
	RouteControlSnapshotsCreate = "/control/snapshots/create"
)

func init() {
	Component = &app.Component{
		Name:      "CoreAPIV3",
		DepsFunc:  func(cDeps dependencies) { deps = cDeps },
		Configure: configure,
		IsEnabled: func() bool {
			return restapi.ParamsRestAPI.Enabled
		},
	}
}

var (
	Component *app.Component
	deps      dependencies
)

type dependencies struct {
	dig.In

	Protocol         *protocol.Protocol
	AppInfo          *app.Info
	RestRouteManager *restapi.RestRouteManager
}

func configure() error {
	routeGroup := deps.RestRouteManager.AddRoute("core/v3")

	routeGroup.GET(RouteInfo, func(c echo.Context) error {
		resp, err := info()
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	routeGroup.GET(RouteBlock, func(c echo.Context) error {
		mimeType, err := httpserver.GetAcceptHeaderContentType(c, httpserver.MIMEApplicationVendorIOTASerializerV1, echo.MIMEApplicationJSON)
		if err != nil && err != httpserver.ErrNotAcceptable {
			return err
		}

		switch mimeType {
		case httpserver.MIMEApplicationVendorIOTASerializerV1:
			resp, err := blockBytesByID(c)
			if err != nil {
				return err
			}

			return c.Blob(http.StatusOK, httpserver.MIMEApplicationVendorIOTASerializerV1, resp)

		default:
			// default to echo.MIMEApplicationJSON
			resp, err := blockByID(c)
			if err != nil {
				return err
			}

			return httpserver.JSONResponse(c, http.StatusOK, resp)
		}
	})

	routeGroup.GET(RouteBlockMetadata, func(c echo.Context) error {
		resp, err := blockMetadataByID(c)
		if err != nil {
			return err
		}
		return httpserver.JSONResponse(c, http.StatusOK, resp)
	}, checkNodeAlmostSynced())

	routeGroup.POST(RouteBlocks, func(c echo.Context) error {
		resp, err := sendBlock(c)
		if err != nil {
			return err
		}
		c.Response().Header().Set(echo.HeaderLocation, resp.BlockID)

		return httpserver.JSONResponse(c, http.StatusCreated, resp)
	}, checkNodeAlmostSynced(), checkUpcomingUnsupportedProtocolVersion())

	routeGroup.GET(RouteBlockIssuance, func(c echo.Context) error {
		resp, err := blockIssuance(c)
		if err != nil {
			return err
		}
		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	return nil
}

func checkNodeAlmostSynced() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// todo update with sync status
			//if !deps.SyncManager.IsNodeAlmostSynced() {
			//	return errors.WithMessage(echo.ErrServiceUnavailable, "node is not synced")
			//}

			return next(c)
		}
	}
}

func checkUpcomingUnsupportedProtocolVersion() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// todo update with protocol upgrades support
			//if !deps.ProtocolManager.NextPendingSupported() {
			//	return errors.WithMessage(echo.ErrServiceUnavailable, "node does not support the upcoming protocol upgrade")
			//}

			return next(c)
		}
	}
}
