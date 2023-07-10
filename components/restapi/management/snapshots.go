package management

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/iota.go/v4/nodeclient"
)

func createSnapshots(c echo.Context) (*nodeclient.CreateSnapshotsResponse, error) {
	/*
		if deps.SnapshotManager.IsSnapshotting() || deps.PruningManager.IsPruning() {
			return nil, errors.WithMessage(echo.ErrServiceUnavailable, "node is already creating a snapshot or pruning is running")
		}

		request := &createSnapshotsRequest{}
		if err := c.Bind(request); err != nil {
			return nil, errors.WithMessagef(httpserver.ErrInvalidParameter, "invalid request, error: %s", err)
		}

		if request.Index == 0 {
			return nil, errors.WithMessage(httpserver.ErrInvalidParameter, "index needs to be specified")
		}

		filePath := filepath.Join(filepath.Dir(deps.SnapshotsFullPath), fmt.Sprintf("full_snapshot_%d.bin", request.Index))
		if err := deps.SnapshotManager.CreateFullSnapshot(Component.Daemon().ContextStopped(), request.Index, filePath, false); err != nil {
			return nil, errors.WithMessagef(echo.ErrInternalServerError, "creating snapshot failed: %s", err)
		}

		return &createSnapshotsResponse{
			Index:    request.Index,
			FilePath: filePath,
		}, nil
	*/
	return nil, nil
}
