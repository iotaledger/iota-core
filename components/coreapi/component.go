package coreapi

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/components/restapi"
)

const (
	// RouteInfo is the route for getting the node info.
	// GET returns the node info.
	RouteInfo = "/info"
)

func init() {
	Component = &app.Component{
		Name:      "CoreAPIV3",
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
	*restapi.RestRouteManager
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
}
