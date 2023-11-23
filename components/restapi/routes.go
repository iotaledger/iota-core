package restapi

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota.go/v4/api"
)

type RoutesResponse struct {
	Routes []string `json:"routes"`
}

func setupRoutes() {

	deps.Echo.GET(api.RouteHealth, func(c echo.Context) error {
		if deps.Protocol.MainEngineInstance().SyncManager.IsNodeSynced() {
			return c.NoContent(http.StatusOK)
		}

		return c.NoContent(http.StatusServiceUnavailable)
	})

	deps.Echo.GET(api.RouteRoutes, func(c echo.Context) error {
		resp := &RoutesResponse{
			Routes: deps.RestRouteManager.Routes(),
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})
}
