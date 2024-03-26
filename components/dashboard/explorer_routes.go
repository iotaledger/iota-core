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
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
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
	routeGroup.GET("/block/:"+api.ParameterBlockID, func(c echo.Context) (err error) {
		blockID, err := httpserver.ParseBlockIDParam(c, api.ParameterBlockID)
		if err != nil {
			return ierrors.Wrap(err, "failed to parse block ID")
		}

		t, err := findBlock(blockID)
		if err != nil {
			return err
		}

		return c.JSON(http.StatusOK, t)
	})

	routeGroup.GET("/transaction/:"+api.ParameterTransactionID, getTransaction)
	routeGroup.GET("/transaction/:transactionID/metadata", getTransactionMetadata)
	// routeGroup.GET("/transaction/:transactionID/attachments", ledgerstateAPI.GetTransactionAttachments)
	routeGroup.GET("/output/:"+api.ParameterOutputID, getOutput)
	// routeGroup.GET("/output/:outputID/metadata", ledgerstateAPI.GetOutputMetadata)
	// routeGroup.GET("/output/:outputID/consumers", ledgerstateAPI.GetOutputConsumers)
	routeGroup.GET("/slot/commitment/:"+api.ParameterCommitmentID, getSlotDetailsByID)

	routeGroup.GET("/search/:search", func(c echo.Context) error {
		search := c.Param("search")
		result := &SearchResult{}

		blockID, err := iotago.BlockIDFromHexString(search)
		if err != nil {
			return ierrors.WithMessagef(ErrInvalidParameter, "search ID %s", search)
		}

		blk, err := findBlock(blockID)
		if err != nil {
			return err
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
		return nil, ierrors.Errorf("block %s not found", blockID)
	}

	cachedBlock, _ := deps.Protocol.Engines.Main.Get().BlockCache.Block(blockID)

	blockMetadata, err := deps.Protocol.Engines.Main.Get().BlockRetainer.BlockMetadata(blockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get block metadata for block %s", blockID)
	}

	return createExplorerBlock(block, cachedBlock, blockMetadata), nil
}

func createExplorerBlock(block *model.Block, cachedBlock *blocks.Block, blockMetadata *api.BlockMetadataResponse) *ExplorerBlock {
	iotaBlk := block.ProtocolBlock()

	sigBytes, err := iotaBlk.Signature.Encode()
	if err != nil {
		return nil
	}

	var payloadJSON []byte
	basicBlock, isBasic := block.BasicBlock()
	if isBasic {
		payloadJSON, err = lo.PanicOnErr(deps.Protocol.APIForVersion(iotaBlk.Header.ProtocolVersion)).JSONEncode(basicBlock.Payload)
		if err != nil {
			return nil
		}
	}

	t := &ExplorerBlock{
		ID:                      block.ID().ToHex(),
		NetworkID:               iotaBlk.Header.NetworkID,
		ProtocolVersion:         iotaBlk.Header.ProtocolVersion,
		SolidificationTimestamp: 0,
		IssuanceTimestamp:       iotaBlk.Header.IssuingTime.Unix(),
		SequenceNumber:          0,
		IssuerID:                iotaBlk.Header.IssuerID.ToHex(),
		Signature:               hexutil.EncodeHex(sigBytes),
		StrongParents:           iotaBlk.Body.StrongParentIDs().ToHex(),
		WeakParents:             iotaBlk.Body.WeakParentIDs().ToHex(),
		ShallowLikedParents:     iotaBlk.Body.ShallowLikeParentIDs().ToHex(),

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

				//nolint:errchkjson
				bytes, _ := json.Marshal(txResponse)

				return bytes
			}

			return payloadJSON
		}(),
		TransactionID: func() string {
			if isBasic && basicBlock.Payload != nil && basicBlock.Payload.PayloadType() == iotago.PayloadSignedTransaction {
				tx, _ := basicBlock.Payload.(*iotago.SignedTransaction)

				return tx.MustID().ToHex()
			}

			return ""
		}(),
		CommitmentID:        iotaBlk.Header.SlotCommitmentID.ToHex(),
		LatestConfirmedSlot: uint64(iotaBlk.Header.LatestFinalizedSlot),
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
		t.SpendIDs = lo.Map(cachedBlock.SpenderIDs().ToSlice(), func(spendID iotago.TransactionID) string {
			return spendID.ToHex()
		})
	} else {
		switch blockMetadata.BlockState {
		case api.BlockStateConfirmed, api.BlockStateFinalized:
			t.Solid = true
			t.Booked = true
			t.Acceptance = true
			t.Scheduled = true
			t.Confirmation = true
		case api.BlockStateDropped, api.BlockStateOrphaned:
			t.ObjectivelyInvalid = true
		}
	}

	return t
}

func getTransaction(c echo.Context) error {
	txID, err := httpserver.ParseTransactionIDParam(c, api.ParameterTransactionID)
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
		return ierrors.Errorf("block %s not found", output.BlockID().ToHex())
	}

	iotaTX, isTX := block.SignedTransaction()
	if !isTX {
		return ierrors.Errorf("block payload of block %s is not a signed transaction", output.BlockID().ToHex())
	}

	return httpserver.JSONResponse(c, http.StatusOK, NewTransaction(iotaTX))
}

func getTransactionMetadata(c echo.Context) error {
	txID, err := httpserver.ParseTransactionIDParam(c, api.ParameterTransactionID)
	if err != nil {
		return err
	}

	// Get the first output of that transaction (using index 0)
	outputID := iotago.OutputID{}
	copy(outputID[:], txID[:])
	txMetadata, exists := deps.Protocol.Engines.Main.Get().Ledger.MemPool().TransactionMetadata(txID)
	if !exists {
		return ierrors.Errorf("transaction metadata for transaction %s not found", txID.ToHex())
	}

	conflicts, _ := deps.Protocol.Engines.Main.Get().Ledger.SpendDAG().ConflictingSpenders(txID)

	return httpserver.JSONResponse(c, http.StatusOK, NewTransactionMetadata(txMetadata, conflicts))
}

func getOutput(c echo.Context) error {
	outputID, err := httpserver.ParseOutputIDParam(c, api.ParameterOutputID)
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
	commitmentID, err := httpserver.ParseCommitmentIDParam(c, api.ParameterCommitmentID)
	if err != nil {
		return err
	}

	commitment, err := deps.Protocol.Engines.Main.Get().Storage.Commitments().Load(commitmentID.Slot())
	if err != nil {
		return err
	}

	if commitment.ID() != commitmentID {
		return ierrors.Errorf("commitment in the store for slot %d does not match the given commitmentID (%s != %s)", commitmentID.Slot(), commitment.ID(), commitmentID)
	}

	diffs, err := deps.Protocol.Engines.Main.Get().Ledger.SlotDiffs(commitmentID.Slot())
	if err != nil {
		return err
	}

	return httpserver.JSONResponse(c, http.StatusOK, NewSlotDetails(commitment, diffs))
}
