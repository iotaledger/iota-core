package management

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/components/restapi"
	"github.com/iotaledger/iota-core/pkg/protocol"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	"github.com/iotaledger/iota.go/v4/api"
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

	routeGroup := deps.RestRouteManager.AddRoute(api.ManagementPluginName)

	routeGroup.GET(api.EndpointWithEchoParameters(api.ManagementEndpointPeer), func(c echo.Context) error {
		resp, err := getPeer(c)
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	routeGroup.DELETE(api.EndpointWithEchoParameters(api.ManagementEndpointPeer), func(c echo.Context) error {
		if err := removePeer(c); err != nil {
			return err
		}

		return c.NoContent(http.StatusNoContent)
	})

	routeGroup.GET(api.ManagementEndpointPeers, func(c echo.Context) error {
		resp, err := listPeers(c)
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	routeGroup.POST(api.ManagementEndpointPeers, func(c echo.Context) error {
		resp, err := addPeer(c, Component.Logger())
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	routeGroup.POST(api.ManagementEndpointDatabasePrune, func(c echo.Context) error {
		resp, err := pruneDatabase(c)
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	routeGroup.POST(api.ManagementEndpointSnapshotsCreate, func(c echo.Context) error {
		resp, err := createSnapshots(c)
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	return nil
}
