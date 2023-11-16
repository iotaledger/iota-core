package inx

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	inx "github.com/iotaledger/inx/go"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

func NewLedgerOutput(o *utxoledger.Output) (*inx.LedgerOutput, error) {
	latestCommitment := deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment()

	l := &inx.LedgerOutput{
		OutputId:   inx.NewOutputId(o.OutputID()),
		BlockId:    inx.NewBlockId(o.BlockID()),
		SlotBooked: uint32(o.SlotBooked()),
		Output: &inx.RawOutput{
			Data: o.Bytes(),
		},
	}

	includedSlot := o.SlotBooked()
	if includedSlot > 0 && includedSlot <= latestCommitment.Slot() {
		includedCommitment, err := deps.Protocol.MainEngineInstance().Storage.Commitments().Load(includedSlot)
		if err != nil {
			return nil, ierrors.Wrapf(err, "failed to load commitment with slot: %d", includedSlot)
		}
		l.CommitmentIdIncluded = inx.NewCommitmentId(includedCommitment.ID())
	}

	return l, nil
}

func NewLedgerSpent(s *utxoledger.Spent) (*inx.LedgerSpent, error) {
	output, err := NewLedgerOutput(s.Output())
	if err != nil {
		return nil, err
	}

	l := &inx.LedgerSpent{
		Output:             output,
		TransactionIdSpent: inx.NewTransactionId(s.TransactionIDSpent()),
		SlotSpent:          uint32(s.SlotSpent()),
	}

	latestCommitment := deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment()
	spentSlot := s.SlotSpent()
	if spentSlot > 0 && spentSlot <= latestCommitment.Slot() {
		spentCommitment, err := deps.Protocol.MainEngineInstance().Storage.Commitments().Load(spentSlot)
		if err != nil {
			return nil, ierrors.Wrapf(err, "failed to load commitment with slot: %d", spentSlot)
		}
		l.CommitmentIdSpent = inx.NewCommitmentId(spentCommitment.ID())
	}

	return l, nil
}

func NewLedgerUpdateBatchBegin(slot iotago.SlotIndex, newOutputsCount int, newSpentsCount int) *inx.LedgerUpdate {
	return &inx.LedgerUpdate{
		Op: &inx.LedgerUpdate_BatchMarker{
			BatchMarker: &inx.LedgerUpdate_Marker{
				Slot:          uint32(slot),
				MarkerType:    inx.LedgerUpdate_Marker_BEGIN,
				CreatedCount:  uint32(newOutputsCount),
				ConsumedCount: uint32(newSpentsCount),
			},
		},
	}
}

func NewLedgerUpdateBatchEnd(slot iotago.SlotIndex, newOutputsCount int, newSpentsCount int) *inx.LedgerUpdate {
	return &inx.LedgerUpdate{
		Op: &inx.LedgerUpdate_BatchMarker{
			BatchMarker: &inx.LedgerUpdate_Marker{
				Slot:          uint32(slot),
				MarkerType:    inx.LedgerUpdate_Marker_END,
				CreatedCount:  uint32(newOutputsCount),
				ConsumedCount: uint32(newSpentsCount),
			},
		},
	}
}

func NewLedgerUpdateBatchOperationCreated(output *utxoledger.Output) (*inx.LedgerUpdate, error) {
	o, err := NewLedgerOutput(output)
	if err != nil {
		return nil, err
	}

	return &inx.LedgerUpdate{
		Op: &inx.LedgerUpdate_Created{
			Created: o,
		},
	}, nil
}

func NewLedgerUpdateBatchOperationConsumed(spent *utxoledger.Spent) (*inx.LedgerUpdate, error) {
	s, err := NewLedgerSpent(spent)
	if err != nil {
		return nil, err
	}

	return &inx.LedgerUpdate{
		Op: &inx.LedgerUpdate_Consumed{
			Consumed: s,
		},
	}, nil
}

func (s *Server) ReadOutput(_ context.Context, id *inx.OutputId) (*inx.OutputResponse, error) {
	engine := deps.Protocol.MainEngineInstance()

	latestCommitment := engine.Storage.Settings().LatestCommitment()

	outputID := id.Unwrap()

	output, spent, err := engine.Ledger.OutputOrSpent(outputID)
	if err != nil {
		return nil, err
	}

	if output != nil {
		ledgerOutput, err := NewLedgerOutput(output)
		if err != nil {
			return nil, err
		}

		return &inx.OutputResponse{
			LatestCommitmentId: inx.NewCommitmentId(latestCommitment.ID()),
			Payload: &inx.OutputResponse_Output{
				Output: ledgerOutput,
			},
		}, nil
	}

	ledgerSpent, err := NewLedgerSpent(spent)
	if err != nil {
		return nil, err
	}

	return &inx.OutputResponse{
		LatestCommitmentId: inx.NewCommitmentId(latestCommitment.ID()),
		Payload: &inx.OutputResponse_Spent{
			Spent: ledgerSpent,
		},
	}, nil
}

func (s *Server) ReadUnspentOutputs(_ *inx.NoParams, srv inx.INX_ReadUnspentOutputsServer) error {
	engine := deps.Protocol.MainEngineInstance()
	latestCommitment := engine.Storage.Settings().LatestCommitment()

	var innerErr error
	err := engine.Ledger.ForEachUnspentOutput(func(output *utxoledger.Output) bool {
		ledgerOutput, err := NewLedgerOutput(output)
		if err != nil {
			innerErr = err

			return false
		}

		payload := &inx.UnspentOutput{
			LatestCommitmentId: inx.NewCommitmentId(latestCommitment.ID()),
			Output:             ledgerOutput,
		}

		if err := srv.Send(payload); err != nil {
			innerErr = fmt.Errorf("send error: %w", err)

			return false
		}

		return true
	})
	if innerErr != nil {
		return innerErr
	}

	return err
}

func (s *Server) ListenToLedgerUpdates(req *inx.SlotRangeRequest, srv inx.INX_ListenToLedgerUpdatesServer) error {
	createLedgerUpdatePayloadAndSend := func(slot iotago.SlotIndex, outputs utxoledger.Outputs, spents utxoledger.Spents) error {
		// Send Begin
		if err := srv.Send(NewLedgerUpdateBatchBegin(slot, len(outputs), len(spents))); err != nil {
			return fmt.Errorf("send error: %w", err)
		}

		// Send consumed
		for _, spent := range spents {
			payload, err := NewLedgerUpdateBatchOperationConsumed(spent)
			if err != nil {
				return err
			}

			if err := srv.Send(payload); err != nil {
				return fmt.Errorf("send error: %w", err)
			}
		}

		// Send created
		for _, output := range outputs {
			payload, err := NewLedgerUpdateBatchOperationCreated(output)
			if err != nil {
				return err
			}

			if err := srv.Send(payload); err != nil {
				return fmt.Errorf("send error: %w", err)
			}
		}

		// Send End
		if err := srv.Send(NewLedgerUpdateBatchEnd(slot, len(outputs), len(spents))); err != nil {
			return fmt.Errorf("send error: %w", err)
		}

		return nil
	}

	sendStateDiffsRange := func(startSlot iotago.SlotIndex, endSlot iotago.SlotIndex) error {
		for currentSlot := startSlot; currentSlot <= endSlot; currentSlot++ {
			stateDiff, err := deps.Protocol.MainEngineInstance().Ledger.SlotDiffs(currentSlot)
			if err != nil {
				return status.Errorf(codes.NotFound, "ledger update for slot %d not found", currentSlot)
			}

			if err := createLedgerUpdatePayloadAndSend(stateDiff.Slot, stateDiff.Outputs, stateDiff.Spents); err != nil {
				return err
			}
		}

		return nil
	}

	// if a startSlot is given, we send all available state diffs including the start slot.
	// if an endSlot is given, we send all available state diffs up to and including min(ledgerSlot, endSlot).
	// if no startSlot is given, but an endSlot we don't send previous state diffs.
	sendPreviousStateDiffs := func(startSlot iotago.SlotIndex, endSlot iotago.SlotIndex) (iotago.SlotIndex, error) {
		if startSlot == 0 {
			// no need to send previous state diffs
			return 0, nil
		}

		latestCommitment := deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment()

		if startSlot > latestCommitment.Slot() {
			// no need to send previous state diffs
			return 0, nil
		}

		// Stream all available milestone diffs first
		prunedEpoch, hasPruned := deps.Protocol.MainEngineInstance().SyncManager.LastPrunedEpoch()
		if hasPruned && startSlot <= deps.Protocol.CommittedAPI().TimeProvider().EpochEnd(prunedEpoch) {
			return 0, status.Errorf(codes.InvalidArgument, "given startSlot %d is older than the current pruningSlot %d", startSlot, deps.Protocol.CommittedAPI().TimeProvider().EpochEnd(prunedEpoch))
		}

		if endSlot == 0 || endSlot > latestCommitment.Slot() {
			endSlot = latestCommitment.Slot()
		}

		if err := sendStateDiffsRange(startSlot, endSlot); err != nil {
			return 0, err
		}

		return endSlot, nil
	}

	stream := &streamRange{
		start: iotago.SlotIndex(req.GetStartSlot()),
		end:   iotago.SlotIndex(req.GetEndSlot()),
	}

	var err error
	stream.lastSent, err = sendPreviousStateDiffs(stream.start, stream.end)
	if err != nil {
		return err
	}

	if stream.isBounded() && stream.lastSent >= stream.end {
		// We are done sending, so close the stream
		return nil
	}

	catchUpFunc := func(start iotago.SlotIndex, end iotago.SlotIndex) error {
		if err := sendStateDiffsRange(start, end); err != nil {
			Component.LogErrorf("sendStateDiffsRange error: %v", err)

			return err
		}

		return nil
	}

	sendFunc := func(slot iotago.SlotIndex, newOutputs utxoledger.Outputs, newSpents utxoledger.Spents) error {
		if err := createLedgerUpdatePayloadAndSend(slot, newOutputs, newSpents); err != nil {
			Component.LogErrorf("send error: %v", err)

			return err
		}

		return nil
	}

	var innerErr error
	ctx, cancel := context.WithCancel(Component.Daemon().ContextStopped())

	wp := workerpool.New("ListenToLedgerUpdates", workerpool.WithWorkerCount(workerCount)).Start()

	unhook := deps.Protocol.Events.Engine.Ledger.StateDiffApplied.Hook(func(slot iotago.SlotIndex, newOutputs utxoledger.Outputs, newSpents utxoledger.Spents) {
		done, err := handleRangedSend2(slot, newOutputs, newSpents, stream, catchUpFunc, sendFunc)
		switch {
		case err != nil:
			innerErr = err
			cancel()

		case done:
			cancel()
		}
	}).Unhook

	<-ctx.Done()
	unhook()

	// We need to wait until all tasks are done, otherwise we might call
	// "SendMsg" and "CloseSend" in parallel on the grpc stream, which is
	// not safe according to the grpc docs.
	wp.Shutdown()
	wp.ShutdownComplete.Wait()

	return innerErr
}

func (s *Server) ListenToAcceptedTransactions(_ *inx.NoParams, srv inx.INX_ListenToAcceptedTransactionsServer) error {
	ctx, cancel := context.WithCancel(Component.Daemon().ContextStopped())

	wp := workerpool.New("ListenToAcceptedTransactions", workerpool.WithWorkerCount(workerCount)).Start()

	unhook := deps.Protocol.Events.Engine.Booker.TransactionAccepted.Hook(func(transactionMetadata mempool.TransactionMetadata) {
		slot := transactionMetadata.EarliestIncludedAttachment().Slot()

		var consumed []*inx.LedgerSpent
		if err := transactionMetadata.Inputs().ForEach(func(stateMetadata mempool.StateMetadata) error {
			spentOutput, ok := stateMetadata.State().(*utxoledger.Output)
			if !ok {
				// not an Output, so we don't need to send it (could be MockedState, Commitment, BlockIssuanceCreditInput, RewardInput, etc.)
				return nil
			}

			inxSpent, err := NewLedgerSpent(utxoledger.NewSpent(spentOutput, transactionMetadata.ID(), slot))
			if err != nil {
				return err
			}
			consumed = append(consumed, inxSpent)

			return nil
		}); err != nil {
			Component.LogErrorf("error creating payload: %v", err)
			cancel()

			return
		}

		var created []*inx.LedgerOutput
		if err := transactionMetadata.Outputs().ForEach(func(stateMetadata mempool.StateMetadata) error {
			output, ok := stateMetadata.State().(*utxoledger.Output)
			if !ok {
				// not an Output, so we don't need to send it (could be MockedState, Commitment, BlockIssuanceCreditInput, RewardInput, etc.)
				return nil
			}

			inxOutput, err := NewLedgerOutput(output)
			if err != nil {
				return err
			}
			created = append(created, inxOutput)

			return nil
		}); err != nil {
			Component.LogErrorf("error creating payload: %v", err)
			cancel()

			return
		}

		payload := &inx.AcceptedTransaction{
			TransactionId: inx.NewTransactionId(transactionMetadata.ID()),
			Slot:          uint32(slot),
			Consumed:      consumed,
			Created:       created,
		}

		if ctx.Err() != nil {
			// context is done, so we don't need to send the payload
			return
		}

		if err := srv.Send(payload); err != nil {
			Component.LogErrorf("send error: %v", err)
			cancel()
		}
	}, event.WithWorkerPool(wp)).Unhook

	<-ctx.Done()
	unhook()

	// We need to wait until all tasks are done, otherwise we might call
	// "SendMsg" and "CloseSend" in parallel on the grpc stream, which is
	// not safe according to the grpc docs.
	wp.Shutdown()
	wp.ShutdownComplete.Wait()

	return ctx.Err()
}
