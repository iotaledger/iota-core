package management

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/components/restapi"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
)

const (
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
		Name:      "ManagementAPIV1",
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
)

type dependencies struct {
	dig.In

	RestRouteManager *restapipkg.RestRouteManager
}

func configure() error {
	// check if RestAPI plugin is disabled
	if !Component.App().IsComponentEnabled(restapi.Component.Identifier()) {
		Component.LogPanicf("RestAPI plugin needs to be enabled to use the %s plugin", Component.Name)
	}

	routeGroup := deps.RestRouteManager.AddRoute("management/v1")

	routeGroup.GET(RoutePeer, func(c echo.Context) error {
		resp, err := getPeer(c)
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	routeGroup.DELETE(RoutePeer, func(c echo.Context) error {
		if err := removePeer(c); err != nil {
			return err
		}

		return c.NoContent(http.StatusNoContent)
	})

	routeGroup.GET(RoutePeers, func(c echo.Context) error {
		resp, err := listPeers(c)
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	routeGroup.POST(RoutePeers, func(c echo.Context) error {
		resp, err := addPeer(c, Component.Logger())
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	routeGroup.POST(RouteControlDatabasePrune, func(c echo.Context) error {
		resp, err := pruneDatabase(c)
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	routeGroup.POST(RouteControlSnapshotsCreate, func(c echo.Context) error {
		resp, err := createSnapshots(c)
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	return nil
}
