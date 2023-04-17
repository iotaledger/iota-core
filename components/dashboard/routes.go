package dashboard

import (
	"embed"
	"io"
	"io/fs"
	"net/http"

	"github.com/labstack/echo/v4"
)

//go:embed frontend/build frontend/src/assets
var staticFS embed.FS

const (
	build  = "frontend/build"
	assets = "frontend/src/assets"
)

// ErrInvalidParameter defines the invalid parameter error.
var ErrInvalidParameter = echo.NewHTTPError(http.StatusBadRequest, "invalid parameter")

func indexRoute(e echo.Context) error {

	index, err := staticFS.Open(build + "/index.html")
	if err != nil {
		return err
	}
	defer index.Close()

	indexHTML, err := io.ReadAll(index)
	if err != nil {
		return err
	}

	return e.HTMLBlob(http.StatusOK, indexHTML)
}

func setupRoutes(e *echo.Echo) {
	staticfsys := fs.FS(staticFS)

	appfs, _ := fs.Sub(staticfsys, build)
	dirEntries, _ := staticFS.ReadDir(build)
	for _, de := range dirEntries {
		e.GET("/app/"+de.Name(), echo.WrapHandler(http.StripPrefix("/app", http.FileServer(http.FS(appfs)))))
	}

	assetsfs, _ := fs.Sub(staticfsys, assets)
	dirEntries, _ = staticFS.ReadDir(assets)
	for _, de := range dirEntries {
		e.GET("/assets/"+de.Name(), echo.WrapHandler(http.StripPrefix("/assets", http.FileServer(http.FS(assetsfs)))))
	}

	e.GET("/ws", websocketRoute)
	e.GET("/", indexRoute)

	// used to route into the dashboard index
	e.GET("*", indexRoute)

	apiRoutes := e.Group("/api")

	setupExplorerRoutes(apiRoutes)
}
