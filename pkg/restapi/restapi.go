package restapi

import (
	"github.com/labstack/echo/v4"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota.go/v4/api"
)

const (
	// QueryParameterPageSize is used to specify the page size.
	QueryParameterPageSize = "pageSize"

	// QueryParameterCursor is used to specify the the point from which the response should continue for paginater results.
	QueryParameterCursor = "cursor"
)

func ParsePeerIDParam(c echo.Context) (peer.ID, error) {
	peerID, err := peer.Decode(c.Param(api.ParameterPeerID))
	if err != nil {
		return "", ierrors.Wrapf(httpserver.ErrInvalidParameter, "invalid peerID, error: %s", err)
	}

	return peerID, nil
}
