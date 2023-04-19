package dashboard

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"

	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/restapi"

	iotago "github.com/iotaledger/iota.go/v4"
)

// SearchResult defines the struct of the SearchResult.
type SearchResult struct {
	// Block is the *ExplorerBlock.
	Block *ExplorerBlock `json:"block"`
	// Address is the *ExplorerAddress.
	Address *ExplorerAddress `json:"address"`
}

func setupExplorerRoutes(routeGroup *echo.Group) {
	routeGroup.GET("/block/:"+restapi.ParameterBlockID, func(c echo.Context) (err error) {
		blockID, err := httpserver.ParseBlockIDParam(c, restapi.ParameterBlockID)
		if err != nil {
			return errors.Errorf("parse block ID error: %v", err)
		}

		t, err := findBlock(blockID)
		if err != nil {
			return errors.Errorf("find block error: %v", err)
		}

		return c.JSON(http.StatusOK, t)
	})

	// routeGroup.GET("/transaction/:transactionID", ledgerstateAPI.GetTransaction)
	// routeGroup.GET("/transaction/:transactionID/metadata", ledgerstateAPI.GetTransactionMetadata)
	// routeGroup.GET("/transaction/:transactionID/attachments", ledgerstateAPI.GetTransactionAttachments)
	// routeGroup.GET("/output/:outputID", ledgerstateAPI.GetOutput)
	// routeGroup.GET("/output/:outputID/metadata", ledgerstateAPI.GetOutputMetadata)
	// routeGroup.GET("/output/:outputID/consumers", ledgerstateAPI.GetOutputConsumers)
	// routeGroup.GET("/conflict/:conflictID", ledgerstateAPI.GetConflict)
	// routeGroup.GET("/conflict/:conflictID/children", ledgerstateAPI.GetConflictChildren)
	// routeGroup.GET("/conflict/:conflictID/conflicts", ledgerstateAPI.GetConflictConflicts)
	// routeGroup.GET("/conflict/:conflictID/voters", ledgerstateAPI.GetConflictVoters)
	// routeGroup.GET("/slot/:index/blocks", slotAPI.GetBlocks)
	// routeGroup.GET("/slot/commitment/:commitment", slotAPI.GetCommittedSlotByCommitment)
	// routeGroup.GET("/slot/:index/transactions", slotAPI.GetTransactions)
	// routeGroup.GET("/slot/:index/utxos", slotAPI.GetUTXOs)

	routeGroup.GET("/search/:search", func(c echo.Context) error {
		search := c.Param("search")
		result := &SearchResult{}

		blockID, err := iotago.SlotIdentifierFromHexString(search)
		if err != nil {
			return errors.WithMessagef(ErrInvalidParameter, "search ID %s", search)
		}

		blk, err := findBlock(blockID)
		if err != nil {
			return fmt.Errorf("can't find block %s: %w", search, err)
		}
		result.Block = blk

		//addr, err := findAddress(search)
		//if err != nil {
		//	return fmt.Errorf("can't find address %s: %w", search, err)
		//}
		//result.Address = addr

		return c.JSON(http.StatusOK, result)
	})
}

func findBlock(blockID iotago.BlockID) (explorerBlk *ExplorerBlock, err error) {
	block, exists := deps.Protocol.MainEngineInstance().Block(blockID)
	if !exists {
		return nil, errors.Errorf("block not found: %s", blockID.ToHex())
	}

	// blockMetadata, exists := deps.Retainer.BlockMetadata(blockID)
	// if !exists {
	// 	return nil, errors.WithMessagef(ErrNotFound, "block metadata %s", blockID.Base58())
	// }

	explorerBlk = createExplorerBlock(block)

	return
}

func createExplorerBlock(block *blocks.Block) *ExplorerBlock {
	// TODO: fill in missing fields
	iotaBlk := block.Block()

	commitmentID, err := iotaBlk.SlotCommitment.ID()
	if err != nil {
		return nil
	}

	sigBytes, err := iotaBlk.Signature.Encode()
	if err != nil {
		return nil
	}

	t := &ExplorerBlock{
		ID:                  block.ID().ToHex(),
		ProtocolVersion:     iotaBlk.ProtocolVersion,
		NetworkID:           iotaBlk.NetworkID,
		IssuanceTimestamp:   iotaBlk.IssuingTime.Unix(),
		IssuerID:            iotaBlk.IssuerID.String(),
		Signature:           iotago.EncodeHex(sigBytes),
		StrongParents:       iotaBlk.StrongParents.ToHex(),
		WeakParents:         iotaBlk.WeakParents.ToHex(),
		ShallowLikedParents: iotaBlk.ShallowLikeParents.ToHex(),

		PayloadType: func() iotago.PayloadType {
			if iotaBlk.Payload != nil {
				return iotaBlk.Payload.PayloadType()
			}
			return iotago.PayloadType(0)
		}(),
		// Payload:              ProcessPayload(block.Payload()),
		CommitmentID:        commitmentID.ToHex(),
		Commitment:          iotaBlk.SlotCommitment,
		LatestConfirmedSlot: uint64(iotaBlk.LatestConfirmedSlot),
	}

	return t
}
