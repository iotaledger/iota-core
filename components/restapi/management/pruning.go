package management

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

func pruneDatabase(_ echo.Context) (*apimodels.PruneDatabaseResponse, error) {
	/*
		if deps.SnapshotManager.IsSnapshotting() || deps.PruningManager.IsPruning() {
			return nil, errors.WithMessage(echo.ErrServiceUnavailable, "node is already creating a snapshot or pruning is running")
		}

		request := &pruneDatabaseRequest{}
		if err := c.Bind(request); err != nil {
			return nil, errors.WithMessagef(httpserver.ErrInvalidParameter, "invalid request, error: %s", err)
		}

		if (request.Index == nil && request.Depth == nil && request.TargetDatabaseSize == nil) ||
			(request.Index != nil && request.Depth != nil) ||
			(request.Index != nil && request.TargetDatabaseSize != nil) ||
			(request.Depth != nil && request.TargetDatabaseSize != nil) {
			return nil, errors.WithMessage(httpserver.ErrInvalidParameter, "either index, depth or size has to be specified")
		}

		var err error
		var targetIndex iotago.SlotIndex

		if request.Index != nil {
			targetIndex, err = deps.PruningManager.PruneDatabaseByTargetIndex(Component.Daemon().ContextStopped(), *request.Index)
			if err != nil {
				return nil, errors.WithMessagef(echo.ErrInternalServerError, "pruning database failed: %s", err)
			}
		}

		if request.Depth != nil {
			targetIndex, err = deps.PruningManager.PruneDatabaseByDepth(Component.Daemon().ContextStopped(), *request.Depth)
			if err != nil {
				return nil, errors.WithMessagef(echo.ErrInternalServerError, "pruning database failed: %s", err)
			}
		}

		if request.TargetDatabaseSize != nil {
			pruningTargetDatabaseSizeBytes, err := bytes.Parse(*request.TargetDatabaseSize)
			if err != nil {
				return nil, errors.WithMessagef(echo.ErrInternalServerError, "pruning database failed: %s", err)
			}

			targetIndex, err = deps.PruningManager.PruneDatabaseBySize(Component.Daemon().ContextStopped(), pruningTargetDatabaseSizeBytes)
			if err != nil {
				return nil, errors.WithMessagef(echo.ErrInternalServerError, "pruning database failed: %s", err)
			}
		}

		return &pruneDatabaseResponse{
			Index: targetIndex,
		}, nil
	*/

	//nolint:revive,nilnil
	return nil, nil
}
