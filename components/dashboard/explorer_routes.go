package dashboard

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	"github.com/iotaledger/iota-core/pkg/retainer"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/hexutil"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

// SearchResult defines the struct of the SearchResult.
type SearchResult struct {
	// Block is the *ExplorerBlock.
	Block *ExplorerBlock `json:"block"`
	// Address is the *ExplorerAddress.
	Address *ExplorerAddress `json:"address"`
}

func setupExplorerRoutes(routeGroup *echo.Group) {
	routeGroup.GET("/block/:"+restapipkg.ParameterBlockID, func(c echo.Context) (err error) {
		blockID, err := httpserver.ParseBlockIDParam(c, restapipkg.ParameterBlockID)
		if err != nil {
			return ierrors.Errorf("parse block ID error: %w", err)
		}

		t, err := findBlock(blockID)
		if err != nil {
			return ierrors.Errorf("find block error: %w", err)
		}

		return c.JSON(http.StatusOK, t)
	})

	routeGroup.GET("/transaction/:"+restapipkg.ParameterTransactionID, getTransaction)
	routeGroup.GET("/transaction/:transactionID/metadata", getTransactionMetadata)
	// routeGroup.GET("/transaction/:transactionID/attachments", ledgerstateAPI.GetTransactionAttachments)
	routeGroup.GET("/output/:"+restapipkg.ParameterOutputID, getOutput)
	// routeGroup.GET("/output/:outputID/metadata", ledgerstateAPI.GetOutputMetadata)
	// routeGroup.GET("/output/:outputID/consumers", ledgerstateAPI.GetOutputConsumers)
	// routeGroup.GET("/conflict/:conflictID", ledgerstateAPI.GetConflict)
	// routeGroup.GET("/conflict/:conflictID/children", ledgerstateAPI.GetConflictChildren)
	// routeGroup.GET("/conflict/:conflictID/conflicts", ledgerstateAPI.GetConflictConflicts)
	// routeGroup.GET("/conflict/:conflictID/voters", ledgerstateAPI.GetConflictVoters)
	routeGroup.GET("/slot/commitment/:"+restapipkg.ParameterCommitmentID, getSlotDetailsByID)

	routeGroup.GET("/search/:search", func(c echo.Context) error {
		search := c.Param("search")
		result := &SearchResult{}

		blockID, err := iotago.BlockIDFromHexString(search)
		if err != nil {
			return ierrors.Wrapf(ErrInvalidParameter, "search ID %s", search)
		}

		blk, err := findBlock(blockID)
		if err != nil {
			return ierrors.Errorf("can't find block %s: %w", search, err)
		}
		result.Block = blk

		// addr, err := findAddress(search)
		// if err != nil {
		//	return ierrors.Errorf("can't find address %s: %w", search, err)
		// }
		// result.Address = addr

		return c.JSON(http.StatusOK, result)
	})
}

func findBlock(blockID iotago.BlockID) (explorerBlk *ExplorerBlock, err error) {
	block, exists := deps.Protocol.Engines.Main.Get().Block(blockID)
	if !exists {
		return nil, ierrors.Errorf("block not found: %s", blockID.ToHex())
	}

	cachedBlock, _ := deps.Protocol.Engines.Main.Get().BlockCache.Block(blockID)

	blockMetadata, err := deps.Protocol.Engines.Main.Get().Retainer.BlockMetadata(blockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "block metadata %s", blockID.ToHex())
	}

	return createExplorerBlock(block, cachedBlock, blockMetadata), nil
}

func createExplorerBlock(block *model.Block, cachedBlock *blocks.Block, metadata *retainer.BlockMetadata) *ExplorerBlock {
	iotaBlk := block.ProtocolBlock()

	sigBytes, err := iotaBlk.Signature.Encode()
	if err != nil {
		return nil
	}

	var payloadJSON []byte
	basicBlock, isBasic := block.BasicBlock()
	if isBasic {
		payloadJSON, err = lo.PanicOnErr(deps.Protocol.APIForVersion(iotaBlk.ProtocolVersion)).JSONEncode(basicBlock.Payload)
		if err != nil {
			return nil
		}
	}

	t := &ExplorerBlock{
		ID:                      block.ID().ToHex(),
		NetworkID:               iotaBlk.NetworkID,
		ProtocolVersion:         iotaBlk.ProtocolVersion,
		SolidificationTimestamp: 0,
		IssuanceTimestamp:       iotaBlk.IssuingTime.Unix(),
		SequenceNumber:          0,
		IssuerID:                iotaBlk.IssuerID.ToHex(),
		Signature:               hexutil.EncodeHex(sigBytes),
		StrongParents:           iotaBlk.Block.StrongParentIDs().ToHex(),
		WeakParents:             iotaBlk.Block.WeakParentIDs().ToHex(),
		ShallowLikedParents:     iotaBlk.Block.ShallowLikeParentIDs().ToHex(),

		PayloadType: func() iotago.PayloadType {
			if isBasic && basicBlock.Payload != nil {
				return basicBlock.Payload.PayloadType()
			}

			return iotago.PayloadType(0)
		}(),
		Payload: func() json.RawMessage {
			if isBasic && basicBlock.Payload != nil && basicBlock.Payload.PayloadType() == iotago.PayloadSignedTransaction {
				tx, _ := basicBlock.Payload.(*iotago.SignedTransaction)
				txResponse := NewTransaction(tx)
				bytes, _ := json.Marshal(txResponse)

				return bytes
			}

			return payloadJSON
		}(),
		TransactionID: func() string {
			if isBasic && basicBlock.Payload != nil && basicBlock.Payload.PayloadType() == iotago.PayloadSignedTransaction {
				tx, _ := basicBlock.Payload.(*iotago.SignedTransaction)
				id, _ := tx.ID()

				return id.ToHex()
			}

			return ""
		}(),
		CommitmentID:        iotaBlk.SlotCommitmentID.ToHex(),
		LatestConfirmedSlot: uint64(iotaBlk.LatestFinalizedSlot),
	}

	if cachedBlock != nil {
		t.Solid = cachedBlock.IsSolid()
		t.Booked = cachedBlock.IsBooked()
		t.Acceptance = cachedBlock.IsAccepted()
		t.Confirmation = cachedBlock.IsConfirmed()
		t.Scheduled = cachedBlock.IsScheduled()
		t.ObjectivelyInvalid = cachedBlock.IsInvalid()
		t.StrongChildren = lo.Map(cachedBlock.StrongChildren(), func(childBlock *blocks.Block) string {
			return childBlock.ID().ToHex()
		})
		t.WeakChildren = lo.Map(cachedBlock.WeakChildren(), func(childBlock *blocks.Block) string {
			return childBlock.ID().ToHex()
		})
		t.LikedInsteadChildren = lo.Map(cachedBlock.ShallowLikeChildren(), func(childBlock *blocks.Block) string {
			return childBlock.ID().ToHex()
		})
		t.ConflictIDs = lo.Map(cachedBlock.ConflictIDs().ToSlice(), func(conflictID iotago.TransactionID) string {
			return conflictID.ToHex()
		})
	} else {
		switch metadata.BlockState {
		case apimodels.BlockStateConfirmed, apimodels.BlockStateFinalized:
			t.Solid = true
			t.Booked = true
			t.Acceptance = true
			t.Scheduled = true
			t.Confirmation = true
		case apimodels.BlockStateFailed, apimodels.BlockStateRejected:
			t.ObjectivelyInvalid = true
		}
	}

	return t
}

func getTransaction(c echo.Context) error {
	txID, err := httpserver.ParseTransactionIDParam(c, restapipkg.ParameterTransactionID)
	if err != nil {
		return err
	}

	// Get the first output of that transaction (using index 0)
	outputID := iotago.OutputID{}
	copy(outputID[:], txID[:])

	output, err := deps.Protocol.Engines.Main.Get().Ledger.Output(outputID)
	if err != nil {
		return err
	}

	block, exists := deps.Protocol.Engines.Main.Get().Block(output.BlockID())
	if !exists {
		return ierrors.Errorf("block not found: %s", output.BlockID().ToHex())
	}

	iotaTX, isTX := block.SignedTransaction()
	if !isTX {
		return ierrors.Errorf("payload is not a signed transaction: %s", output.BlockID().ToHex())
	}

	return httpserver.JSONResponse(c, http.StatusOK, NewTransaction(iotaTX))
}

func getTransactionMetadata(c echo.Context) error {
	txID, err := httpserver.ParseTransactionIDParam(c, restapipkg.ParameterTransactionID)
	if err != nil {
		return err
	}

	// Get the first output of that transaction (using index 0)
	outputID := iotago.OutputID{}
	copy(outputID[:], txID[:])
	txMetadata, exists := deps.Protocol.Engines.Main.Get().Ledger.MemPool().TransactionMetadata(txID)
	if !exists {
		return ierrors.Errorf("tx metadata not found: %s", txID.ToHex())
	}

	conflicts, _ := deps.Protocol.Engines.Main.Get().Ledger.ConflictDAG().ConflictingConflicts(txID)

	return httpserver.JSONResponse(c, http.StatusOK, NewTransactionMetadata(txMetadata, conflicts))
}

func getOutput(c echo.Context) error {
	outputID, err := httpserver.ParseOutputIDParam(c, restapipkg.ParameterOutputID)
	if err != nil {
		return err
	}

	output, err := deps.Protocol.Engines.Main.Get().Ledger.Output(outputID)
	if err != nil {
		return err
	}

	return httpserver.JSONResponse(c, http.StatusOK, NewOutputFromLedgerstateOutput(output))
}

func getSlotDetailsByID(c echo.Context) error {
	commitmentID, err := httpserver.ParseCommitmentIDParam(c, restapipkg.ParameterCommitmentID)
	if err != nil {
		return err
	}

	commitment, err := deps.Protocol.Engines.Main.Get().Storage.Commitments().Load(commitmentID.Slot())
	if err != nil {
		return err
	}

	diffs, err := deps.Protocol.Engines.Main.Get().Ledger.SlotDiffs(commitmentID.Slot())
	if err != nil {
		return err
	}

	return httpserver.JSONResponse(c, http.StatusOK, NewSlotDetails(commitment, diffs))
}
