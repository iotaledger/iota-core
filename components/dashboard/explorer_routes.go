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
	"github.com/iotaledger/iota-core/pkg/restapi"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/hexutil"
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

		blockID, err := iotago.SlotIdentifierFromHexString(search)
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
	block, exists := deps.Protocol.MainEngineInstance().Block(blockID)
	if !exists {
		return nil, ierrors.Errorf("model block not found: %s", blockID.ToHex())
	}

	// TODO: metadata instead, or retainer
	cachedBlock, exists := deps.Protocol.MainEngineInstance().BlockCache.Block(blockID)
	if !exists {
		cachedBlock = nil
	}
	// blockMetadata, exists := deps.Retainer.BlockMetadata(blockID)
	// if !exists {
	// 	return nil, ierrors.Wrapf(ErrNotFound, "block metadata %s", blockID.Base58())
	// }

	explorerBlk = createExplorerBlock(block, cachedBlock)

	return
}

func createExplorerBlock(block *model.Block, cachedBlock *blocks.Block) *ExplorerBlock {
	// TODO: fill in missing fields
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
		ID:                  block.ID().ToHex(),
		ProtocolVersion:     iotaBlk.ProtocolVersion,
		NetworkID:           iotaBlk.NetworkID,
		IssuanceTimestamp:   iotaBlk.IssuingTime.Unix(),
		IssuerID:            iotaBlk.IssuerID.String(),
		Signature:           hexutil.EncodeHex(sigBytes),
		StrongParents:       iotaBlk.Block.StrongParentIDs().ToHex(),
		WeakParents:         iotaBlk.Block.WeakParentIDs().ToHex(),
		ShallowLikedParents: iotaBlk.Block.ShallowLikeParentIDs().ToHex(),

		PayloadType: func() iotago.PayloadType {
			if isBasic && basicBlock.Payload != nil {
				return basicBlock.Payload.PayloadType()
			}
			return iotago.PayloadType(0)
		}(),
		TransactionID: func() string {
			if basicBlock.Payload != nil && basicBlock.Payload.PayloadType() == iotago.PayloadTransaction {
				tx := basicBlock.Payload.(*iotago.Transaction)
				id, _ := tx.ID(deps.Protocol.APIForVersion(iotaBlk.ProtocolVersion))

				return id.ToHex()
			}
			return ""
		}(),
		Payload: func() json.RawMessage {
			if basicBlock.Payload != nil && basicBlock.Payload.PayloadType() == iotago.PayloadTransaction {
				tx := NewTransaction(basicBlock.Payload.(*iotago.Transaction))
				bytes, _ := json.Marshal(tx)

				return bytes
			}
			return payloadJSON
		}(),
		CommitmentID: iotaBlk.SlotCommitmentID.ToHex(),
		// TODO: remove from explorer or add link to a separate route
		// Commitment: CommitmentResponse{
		//	Index:            uint64(iotaBlk.SlotCommitmentID.Index()),
		//	PrevID:           iotaBlk.SlotCommitment.PrevID.ToHex(),
		//	RootsID:          iotaBlk.SlotCommitment.RootsID.ToHex(),
		//	CumulativeWeight: iotaBlk.SlotCommitment.CumulativeWeight,
		// },
		LatestConfirmedSlot: uint64(iotaBlk.LatestFinalizedSlot),
	}

	if cachedBlock != nil {
		t.Solid = cachedBlock.IsSolid()
		t.Booked = cachedBlock.IsBooked()
		t.Acceptance = cachedBlock.IsAccepted()
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

	output, err := deps.Protocol.MainEngineInstance().Ledger.Output(outputID)
	if err != nil {
		return err
	}

	block, exists := deps.Protocol.MainEngineInstance().Block(output.BlockID())
	if !exists {
		return ierrors.Errorf("block not found: %s", output.BlockID().ToHex())
	}

	iotaTX, isTX := block.Transaction()
	if !isTX {
		return ierrors.Errorf("payload is not a transaction: %s", output.BlockID().ToHex())
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
	txMetadata, exists := deps.Protocol.MainEngineInstance().Ledger.MemPool().TransactionMetadata(txID)
	if !exists {
		return ierrors.Errorf("tx metadata not found: %s", txID.ToHex())
	}

	conflicts, _ := deps.Protocol.MainEngineInstance().Ledger.ConflictDAG().ConflictingConflicts(txID)

	return httpserver.JSONResponse(c, http.StatusOK, NewTransactionMetadata(txMetadata, conflicts))
}

func getOutput(c echo.Context) error {
	outputID, err := httpserver.ParseOutputIDParam(c, restapipkg.ParameterOutputID)
	if err != nil {
		return err
	}

	output, err := deps.Protocol.MainEngineInstance().Ledger.Output(outputID)
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

	commitment, err := deps.Protocol.MainEngineInstance().Storage.Permanent.Commitments().Load(commitmentID.Index())
	if err != nil {
		return err
	}

	diffs, err := deps.Protocol.MainEngineInstance().Ledger.StateDiffs(commitmentID.Index())
	if err != nil {
		return err
	}

	return httpserver.JSONResponse(c, http.StatusOK, NewSlotDetails(commitment, diffs))
}
