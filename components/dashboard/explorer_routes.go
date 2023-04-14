package dashboard

import (
	"net/http"

	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/restapi"
	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"

	iotago "github.com/iotaledger/iota.go/v4"
)

func setupExplorerRoutes(routeGroup *echo.Group) {
	routeGroup.GET("/block/:"+restapi.ParameterBlockID, func(c echo.Context) (err error) {
		blockID, err := httpserver.ParseBlockIDParam(c, restapi.ParameterBlockID)
		Component.LogWarnf("parse blockID failed, %v", err)
		if err != nil {
			return errors.Errorf("parse block ID error: %v", err)
		}

		t, err := findBlock(blockID)
		if err != nil {
			return errors.Errorf("find block error: %v", err)
		}

		return c.JSON(http.StatusOK, t)
	})

	// routeGroup.GET("/address/:id", func(c echo.Context) error {
	// 	addr, err := findAddress(c.Param("id"))
	// 	if err != nil {
	// 		return err
	// 	}
	// 	return c.JSON(http.StatusOK, addr)
	// })

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

	// routeGroup.GET("/search/:search", func(c echo.Context) error {
	// 	search := c.Param("search")
	// 	result := &SearchResult{}

	// 	switch strings.Contains(search, ":") {
	// 	case true:
	// 		var blockID models.BlockID
	// 		err := blockID.FromBase58(search)
	// 		if err != nil {
	// 			return errors.WithMessagef(ErrInvalidParameter, "search ID %s", search)
	// 		}

	// 		blk, err := findBlock(blockID)
	// 		if err != nil {
	// 			return fmt.Errorf("can't find block %s: %w", search, err)
	// 		}
	// 		result.Block = blk

	// 	case false:
	// 		addr, err := findAddress(search)
	// 		if err != nil {
	// 			return fmt.Errorf("can't find address %s: %w", search, err)
	// 		}
	// 		result.Address = addr
	// 	}

	// 	return c.JSON(http.StatusOK, result)
	// })
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

	explorerBlk = createExplorerBlock(block.Block())

	return
}

func createExplorerBlock(block *iotago.Block) *ExplorerBlock {
	// TODO: fill in missing fields
	blkID, err := block.ID(deps.Protocol.API().SlotTimeProvider())
	if err != nil {
		return nil
	}

	commitmentID, err := block.SlotCommitment.ID()
	if err != nil {
		return nil
	}

	sigBytes, err := block.Signature.Encode()
	if err != nil {
		return nil
	}

	t := &ExplorerBlock{
		ID:                  blkID.Identifier().ToHex(),
		ProtocolVersion:     block.ProtocolVersion,
		NetworkID:           block.NetworkID,
		IssuanceTimestamp:   block.IssuingTime.Unix(),
		IssuerID:            block.IssuerID.String(),
		Signature:           iotago.EncodeHex(sigBytes),
		StrongParents:       block.StrongParents.ToHex(),
		WeakParents:         block.WeakParents.ToHex(),
		ShallowLikedParents: block.ShallowLikeParents.ToHex(),

		PayloadType: block.Payload.PayloadType(),
		// Payload:              ProcessPayload(block.Payload()),
		CommitmentID:        commitmentID.ToHex(),
		Commitment:          block.SlotCommitment,
		LatestConfirmedSlot: uint64(block.LatestConfirmedSlot),
	}

	return t
}
