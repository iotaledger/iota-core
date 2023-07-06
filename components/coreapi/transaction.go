package coreapi

import (
	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"

	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient"
)

func blockIDByTransactionID(c echo.Context) (iotago.BlockID, error) {
	txID, err := httpserver.ParseTransactionIDParam(c, restapipkg.ParameterTransactionID)
	if err != nil {
		return iotago.EmptyBlockID(), err
	}

	// Get the first output of that transaction (using index 0)
	outputID := iotago.OutputID{}
	copy(outputID[:], txID[:])

	output, err := deps.Protocol.MainEngineInstance().Ledger.Output(outputID.UTXOInput())
	if err != nil {
		return iotago.EmptyBlockID(), err
	}

	return output.BlockID(), nil
}

func blockByTransactionID(c echo.Context) (*model.Block, error) {
	blockID, err := blockIDByTransactionID(c)
	if err != nil {
		return nil, err
	}

	block, exists := deps.Protocol.MainEngineInstance().Block(blockID)
	if !exists {
		return nil, errors.Errorf("block not found: %s", blockID.ToHex())
	}

	return block, nil
}

func blockMetadataFromTransactionID(c echo.Context) (*nodeclient.BlockMetadataResponse, error) {
	block, err := blockByTransactionID(c)
	if err != nil {
		return nil, err
	}

	txState := txStatePending.String()
	txMetadata, exist := deps.Protocol.MainEngineInstance().Ledger.TransactionMetadataByAttachment(block.ID())
	if exist {
		txState = resolveTxState(txMetadata)
	}

	// TODO: fill in blockReason, TxReason.

	bmResponse := &nodeclient.BlockMetadataResponse{
		BlockID:            block.ID().ToHex(),
		StrongParents:      block.ProtocolBlock().Block.StrongParentIDs().ToHex(),
		WeakParents:        block.ProtocolBlock().Block.WeakParentIDs().ToHex(),
		ShallowLikeParents: block.ProtocolBlock().Block.ShallowLikeParentIDs().ToHex(),
		BlockState:         blockStatePending.String(),
		TxState:            txState,
	}

	return bmResponse, nil
}

func resolveTxState(metadata mempool.TransactionMetadata) string {
	var state txState

	if metadata.IsAccepted() {
		state = txStateConfirmed
	} else if metadata.IsCommitted() {
		state = txStateFinalized
	} else {
		state = txStatePending
	}

	return state.String()
}
