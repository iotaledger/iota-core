package management

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/components/restapi"
	"github.com/iotaledger/iota-core/pkg/protocol"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	"github.com/iotaledger/iota.go/v4/nodeclient"
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
	Protocol         *protocol.Protocol
}

func configure() error {
	// check if RestAPI plugin is disabled
	if !Component.App().IsComponentEnabled(restapi.Component.Identifier()) {
		Component.LogPanicf("RestAPI plugin needs to be enabled to use the %s plugin", Component.Name)
	}

	formatEndpoint := func(endpoint string, parameter string) string {
		return fmt.Sprintf(endpoint, ":"+parameter)
	}

	routeGroup := deps.RestRouteManager.AddRoute(nodeclient.ManagementPluginName)

	routeGroup.GET(formatEndpoint(nodeclient.ManagementEndpointPeer, restapipkg.ParameterPeerID), func(c echo.Context) error {
		resp, err := getPeer(c)
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	routeGroup.DELETE(formatEndpoint(nodeclient.ManagementEndpointPeer, restapipkg.ParameterPeerID), func(c echo.Context) error {
		if err := removePeer(c); err != nil {
			return err
		}

		return c.NoContent(http.StatusNoContent)
	})

	routeGroup.GET(nodeclient.ManagementEndpointPeers, func(c echo.Context) error {
		resp, err := listPeers(c)
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	routeGroup.POST(nodeclient.ManagementEndpointPeers, func(c echo.Context) error {
		resp, err := addPeer(c, Component.Logger())
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	routeGroup.POST(nodeclient.ManagementEndpointDatabasePrune, func(c echo.Context) error {
		resp, err := pruneDatabase(c)
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	routeGroup.POST(nodeclient.ManagementEndpointSnapshotsCreate, func(c echo.Context) error {
		resp, err := createSnapshots(c)
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	return nil
}
