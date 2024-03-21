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
		if deps.Protocol.Engines.Main.Get().SyncManager.IsNodeSynced() {
			return httpserver.JSONResponse(c, http.StatusOK, &api.HealthResponse{
				IsHealthy: true,
			})
		}

		return httpserver.JSONResponse(c, http.StatusServiceUnavailable, &api.HealthResponse{
			IsHealthy: false,
		})
	})

	deps.Echo.GET(api.RouteRoutes, func(c echo.Context) error {
		resp := &RoutesResponse{
			Routes: deps.RestRouteManager.Routes(),
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})
}
