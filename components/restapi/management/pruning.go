package management

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/bytes"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

func pruneDatabase(c echo.Context) (*apimodels.PruneDatabaseResponse, error) {
	if deps.Protocol.MainEngine().Storage.IsPruning() {
		return nil, ierrors.Wrapf(echo.ErrServiceUnavailable, "node is already pruning")
	}

	request := &apimodels.PruneDatabaseRequest{}
	if err := c.Bind(request); err != nil {
		return nil, ierrors.Wrapf(httpserver.ErrInvalidParameter, "invalid request, error: %s", err)
	}

	// only allow one type of pruning at a time
	if (request.Index == 0 && request.Depth == 0 && request.TargetDatabaseSize == "") ||
		(request.Index != 0 && request.Depth != 0) ||
		(request.Index != 0 && request.TargetDatabaseSize != "") ||
		(request.Depth != 0 && request.TargetDatabaseSize != "") {
		return nil, ierrors.Wrapf(httpserver.ErrInvalidParameter, "either index, depth or size has to be specified")
	}

	var err error

	if request.Index != 0 {
		err = deps.Protocol.MainEngine().Storage.PruneByEpochIndex(request.Index)
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "pruning database failed: %s", err)
		}
	}

	if request.Depth != 0 {
		_, _, err := deps.Protocol.MainEngine().Storage.PruneByDepth(request.Depth)
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "pruning database failed: %s", err)
		}
	}

	if request.TargetDatabaseSize != "" {
		pruningTargetDatabaseSizeBytes, err := bytes.Parse(request.TargetDatabaseSize)
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "pruning database failed: %s", err)
		}

		err = deps.Protocol.MainEngine().Storage.PruneBySize(pruningTargetDatabaseSizeBytes)
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "pruning database failed: %s", err)
		}
	}

	targetIndex, hasPruned := deps.Protocol.MainEngine().Storage.LastPrunedEpoch()
	if hasPruned {
		targetIndex++
	}

	return &apimodels.PruneDatabaseResponse{
		Index: targetIndex,
	}, nil
}
