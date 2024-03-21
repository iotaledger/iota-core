package management

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/network/p2p"
	"github.com/iotaledger/iota-core/pkg/protocol"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	"github.com/iotaledger/iota.go/v4/api"
)

func init() {
	Component = &app.Component{
		Name:      "ManagementAPIV1",
		DepsFunc:  func(cDeps dependencies) { deps = cDeps },
		Configure: configure,
	}
}

var (
	Component *app.Component
	deps      dependencies
)

type dependencies struct {
	dig.In

	RestRouteManager     *restapipkg.RestRouteManager
	Protocol             *protocol.Protocol
	PeeringConfigManager *p2p.ConfigManager
	NetworkManager       network.Manager
}

func configure() error {
	routeGroup := deps.RestRouteManager.AddRoute(api.ManagementPluginName)

	routeGroup.GET(api.EndpointWithEchoParameters(api.ManagementEndpointPeer), func(c echo.Context) error {
		resp, err := getPeer(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp, http.StatusOK)
	})

	routeGroup.DELETE(api.EndpointWithEchoParameters(api.ManagementEndpointPeer), func(c echo.Context) error {
		if err := removePeer(c); err != nil {
			return err
		}

		return c.NoContent(http.StatusNoContent)
	})

	routeGroup.GET(api.ManagementEndpointPeers, func(c echo.Context) error {
		return responseByHeader(c, listPeers(), http.StatusOK)
	})

	routeGroup.POST(api.ManagementEndpointPeers, func(c echo.Context) error {
		resp, err := addPeer(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp, http.StatusOK)
	})

	routeGroup.POST(api.ManagementEndpointDatabasePrune, func(c echo.Context) error {
		resp, err := pruneDatabase(c)
		if err != nil {
			return err
		}

		return responseByHeader(c, resp, http.StatusOK)
	})

	// routeGroup.POST(api.ManagementEndpointSnapshotsCreate, func(c echo.Context) error {
	//	resp, err := createSnapshots(c)
	//	if err != nil {
	//		return err
	//	}
	//
	//	return responseByHeader(c, resp, http.StatusOK)
	// })

	return nil
}

func responseByHeader(c echo.Context, obj any, httpStatusCode ...int) error {
	return httpserver.SendResponseByHeader(c, deps.Protocol.CommittedAPI(), obj, httpStatusCode...)
}
