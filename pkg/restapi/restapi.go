package restapi

import (
	"github.com/labstack/echo/v4"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
)

const (
	// ParameterBlockID is used to identify a block by its ID.
	ParameterBlockID = "blockID"

	// ParameterTransactionID is used to identify a transaction by its ID.
	ParameterTransactionID = "transactionID"

	// ParameterOutputID is used to identify an output by its ID.
	ParameterOutputID = "outputID"

	// ParameterSlotIndex is used to identify a slot by index.
	ParameterSlotIndex = "slotIndex"

	// ParameterCommitmentID is used to identify a slot commitment by its ID.
	ParameterCommitmentID = "commitmentID"

	// ParameterAccountID is used to identify an account by its ID.
	ParameterAccountID = "accountID"

	// ParameterPeerID is used to identify a peer.
	ParameterPeerID = "peerID"
)

func ParsePeerIDParam(c echo.Context) (peer.ID, error) {
	peerID, err := peer.Decode(c.Param(ParameterPeerID))
	if err != nil {
		return "", ierrors.Wrapf(httpserver.ErrInvalidParameter, "invalid peerID, error: %s", err)
	}

	return peerID, nil
}
