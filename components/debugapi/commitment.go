package debugapi

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/inx-app/pkg/httpserver"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
)

func chainManagerAllChains(c echo.Context) (iotago.SlotIndex, error) {
	commitmentID, err := httpserver.ParseCommitmentIDParam(c, restapipkg.ParameterCommitmentID)
	if err != nil {
		return iotago.SlotIndex(0), err
	}

	return commitmentID.Index(), nil
}
