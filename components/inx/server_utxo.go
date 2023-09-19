package inx

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	inx "github.com/iotaledger/inx/go"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

func NewLedgerOutput(o *utxoledger.Output) (*inx.LedgerOutput, error) {
	latestCommitment := deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment()

	l := &inx.LedgerOutput{
		OutputId:    inx.NewOutputId(o.OutputID()),
		BlockId:     inx.NewBlockId(o.BlockID()),
		SlotBooked:  uint64(o.SlotBooked()),
		SlotCreated: uint64(o.SlotCreated()),
		Output: &inx.RawOutput{
			Data: o.Bytes(),
		},
	}

	includedSlotIndex := o.SlotBooked()
	if includedSlotIndex <= latestCommitment.Index() {
		includedCommitment, err := deps.Protocol.MainEngineInstance().Storage.Commitments().Load(includedSlotIndex)
		if err != nil {
			return nil, ierrors.Wrapf(err, "failed to load commitment with index: %d", includedSlotIndex)
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
		SlotSpent:          uint64(s.SlotIndexSpent()),
	}

	latestCommitment := deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment()
	spentSlotIndex := s.SlotIndexSpent()
	if spentSlotIndex <= latestCommitment.Index() {
		spentCommitment, err := deps.Protocol.MainEngineInstance().Storage.Commitments().Load(spentSlotIndex)
		if err != nil {
			return nil, ierrors.Wrapf(err, "failed to load commitment with index: %d", spentSlotIndex)
		}
		l.CommitmentIdSpent = inx.NewCommitmentId(spentCommitment.ID())
	}

	return l, nil
}

func NewLedgerUpdateBatchBegin(index iotago.SlotIndex, newOutputsCount int, newSpentsCount int) *inx.LedgerUpdate {
	return &inx.LedgerUpdate{
		Op: &inx.LedgerUpdate_BatchMarker{
			BatchMarker: &inx.LedgerUpdate_Marker{
				Slot:          uint64(index),
				MarkerType:    inx.LedgerUpdate_Marker_BEGIN,
				CreatedCount:  uint32(newOutputsCount),
				ConsumedCount: uint32(newSpentsCount),
			},
		},
	}
}

func NewLedgerUpdateBatchEnd(index iotago.SlotIndex, newOutputsCount int, newSpentsCount int) *inx.LedgerUpdate {
	return &inx.LedgerUpdate{
		Op: &inx.LedgerUpdate_BatchMarker{
			BatchMarker: &inx.LedgerUpdate_Marker{
				Slot:          uint64(index),
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

	sendStateDiffsRange := func(startIndex iotago.SlotIndex, endIndex iotago.SlotIndex) error {
		for currentIndex := startIndex; currentIndex <= endIndex; currentIndex++ {
			stateDiff, err := deps.Protocol.MainEngineInstance().Ledger.SlotDiffs(currentIndex)
			if err != nil {
				return status.Errorf(codes.NotFound, "ledger update for milestoneIndex %d not found", currentIndex)
			}

			if err := createLedgerUpdatePayloadAndSend(stateDiff.Index, stateDiff.Outputs, stateDiff.Spents); err != nil {
				return err
			}
		}

		return nil
	}

	// if a startIndex is given, we send all available milestone diffs including the start index.
	// if an endIndex is given, we send all available milestone diffs up to and including min(ledgerIndex, endIndex).
	// if no startIndex is given, but an endIndex, we don't send previous milestone diffs.
	sendPreviousStateDiffs := func(startIndex iotago.SlotIndex, endIndex iotago.SlotIndex) (iotago.SlotIndex, error) {
		if startIndex == 0 {
			// no need to send previous milestone diffs
			return 0, nil
		}

		latestCommitment := deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment()

		if startIndex > latestCommitment.Index() {
			// no need to send previous milestone diffs
			return 0, nil
		}

		// Stream all available milestone diffs first
		pruningIndex := deps.Protocol.MainEngineInstance().SyncManager.LatestFinalizedSlot()
		if startIndex <= pruningIndex {
			return 0, status.Errorf(codes.InvalidArgument, "given startMilestoneIndex %d is older than the current pruningIndex %d", startIndex, pruningIndex)
		}

		if endIndex == 0 || endIndex > latestCommitment.Index() {
			endIndex = latestCommitment.Index()
		}

		if err := sendStateDiffsRange(startIndex, endIndex); err != nil {
			return 0, err
		}

		return endIndex, nil
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
			Component.LogErrorf("sendMilestoneDiffsRange error: %v", err)

			return err
		}

		return nil
	}

	sendFunc := func(index iotago.SlotIndex, newOutputs utxoledger.Outputs, newSpents utxoledger.Spents) error {
		if err := createLedgerUpdatePayloadAndSend(index, newOutputs, newSpents); err != nil {
			Component.LogErrorf("send error: %v", err)

			return err
		}

		return nil
	}

	var innerErr error
	ctx, cancel := context.WithCancel(Component.Daemon().ContextStopped())

	wp := workerpool.New("ListenToLedgerUpdates", workerCount).Start()

	unhook := deps.Protocol.Events.Engine.Ledger.StateDiffApplied.Hook(func(index iotago.SlotIndex, newOutputs utxoledger.Outputs, newSpents utxoledger.Spents) {
		done, err := handleRangedSend2(index, newOutputs, newSpents, stream, catchUpFunc, sendFunc)
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
