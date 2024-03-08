package management

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/iota.go/v4/api"
)

func createSnapshots(_ echo.Context) (*api.CreateSnapshotResponse, error) {
	/*
		if deps.SnapshotManager.IsSnapshotting() || deps.PruningManager.IsPruning() {
			return nil, ierrors.WithMessage(echo.ErrServiceUnavailable, "node is already creating a snapshot or pruning is running")
		}

		request := &createSnapshotsRequest{}
		if err := c.Bind(request); err != nil {
			return nil, ierrors.WithMessagef(httpserver.ErrInvalidParameter, "invalid request, error: %s", err)
		}

		if request.Slot == 0 {
			return nil, ierrors.WithMessage(httpserver.ErrInvalidParameter, "index needs to be specified")
		}

		filePath := filepath.Join(filepath.Dir(deps.SnapshotsFullPath), fmt.Sprintf("full_snapshot_%d.bin", request.Slot))
		if err := deps.SnapshotManager.CreateFullSnapshot(Component.Daemon().ContextStopped(), request.Slot, filePath, false); err != nil {
			return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "creating snapshot failed: %s", err)
		}

		return &createSnapshotsResponse{
			Slot:    request.Slot,
			FilePath: filePath,
		}, nil
	*/

	//nolint:revive,nilnil
	return nil, nil
}
