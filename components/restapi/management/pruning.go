package management

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/bytes"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

func pruneDatabase(c echo.Context) (*apimodels.PruneDatabaseResponse, error) {
	if deps.Protocol.MainEngineInstance().Storage.IsPruning() {
		return nil, ierrors.Wrapf(echo.ErrServiceUnavailable, "node is already pruning")
	}

	request := &apimodels.PruneDatabaseRequest{}
	if err := c.Bind(request); err != nil {
		return nil, ierrors.Wrapf(httpserver.ErrInvalidParameter, "invalid request, error: %s", err)
	}

	// only allow one type of pruning at a time
	if (request.Epoch == 0 && request.Depth == 0 && request.TargetDatabaseSize == "") ||
		(request.Epoch != 0 && request.Depth != 0) ||
		(request.Epoch != 0 && request.TargetDatabaseSize != "") ||
		(request.Depth != 0 && request.TargetDatabaseSize != "") {
		return nil, ierrors.Wrapf(httpserver.ErrInvalidParameter, "either epoch, depth or size has to be specified")
	}

	var err error

	if request.Epoch != 0 {
		err = deps.Protocol.MainEngineInstance().Storage.PruneByEpochIndex(request.Epoch)
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "pruning database failed: %s", err)
		}
	}

	if request.Depth != 0 {
		_, _, err := deps.Protocol.MainEngineInstance().Storage.PruneByDepth(request.Depth)
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "pruning database failed: %s", err)
		}
	}

	if request.TargetDatabaseSize != "" {
		pruningTargetDatabaseSizeBytes, err := bytes.Parse(request.TargetDatabaseSize)
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "pruning database failed: %s", err)
		}

		err = deps.Protocol.MainEngineInstance().Storage.PruneBySize(pruningTargetDatabaseSizeBytes)
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "pruning database failed: %s", err)
		}
	}

	targetEpoch, hasPruned := deps.Protocol.MainEngineInstance().Storage.LastPrunedEpoch()
	if hasPruned {
		targetEpoch++
	}

	return &apimodels.PruneDatabaseResponse{
		Epoch: targetEpoch,
	}, nil
}
