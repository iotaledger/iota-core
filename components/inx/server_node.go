package inx

import (
	"context"
	"time"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	inx "github.com/iotaledger/inx/go"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/syncmanager"
)

func inxNodeStatus(status *syncmanager.SyncStatus) *inx.NodeStatus {
	var finalizedCommitment *model.Commitment
	// HasPruned is false when a node just started from a snapshot and keeps data of the LastPrunedEpoch, thus still need
	// to send finalized commitment.
	if !status.HasPruned || status.LatestFinalizedSlot > deps.Protocol.CommittedAPI().TimeProvider().EpochEnd(status.LastPrunedEpoch) {
		var err error
		finalizedCommitment, err = deps.Protocol.Engines.Main.Get().Storage.Commitments().Load(status.LatestFinalizedSlot)
		if err != nil {
			return nil
		}
	}

	return &inx.NodeStatus{
		IsHealthy:                 status.NodeSynced,
		IsBootstrapped:            status.NodeBootstrapped,
		LastAcceptedBlockSlot:     uint32(status.LastAcceptedBlockSlot),
		LastConfirmedBlockSlot:    uint32(status.LastConfirmedBlockSlot),
		LatestCommitment:          inxCommitment(status.LatestCommitment),
		LatestFinalizedCommitment: inxCommitment(finalizedCommitment),
		PruningEpoch:              uint32(status.LastPrunedEpoch),
	}
}

func (s *Server) ReadNodeStatus(context.Context, *inx.NoParams) (*inx.NodeStatus, error) {
	return inxNodeStatus(deps.Protocol.Engines.Main.Get().SyncManager.SyncStatus()), nil
}

func (s *Server) ListenToNodeStatus(req *inx.NodeStatusRequest, srv inx.INX_ListenToNodeStatusServer) error {
	ctx, cancel := context.WithCancel(Component.Daemon().ContextStopped())

	lastSent := time.Time{}
	sendStatus := func(status *inx.NodeStatus) {
		if err := srv.Send(status); err != nil {
			Component.LogErrorf("send error: %v", err)
			cancel()

			return
		}
		lastSent = time.Now()
	}

	var lastUpdateTimer *time.Timer
	coolDownDuration := time.Duration(req.GetCooldownInMilliseconds()) * time.Millisecond
	wp := workerpool.New("ListenToNodeStatus", workerpool.WithWorkerCount(workerCount))

	onUpdate := func(status *syncmanager.SyncStatus) {
		if lastUpdateTimer != nil {
			lastUpdateTimer.Stop()
			lastUpdateTimer = nil
		}

		nodeStatus := inxNodeStatus(status)

		// Use cool-down if the node is syncing
		if coolDownDuration > 0 && !nodeStatus.GetIsHealthy() {
			timeSinceLastSent := time.Since(lastSent)
			if timeSinceLastSent < coolDownDuration {
				lastUpdateTimer = time.AfterFunc(coolDownDuration-timeSinceLastSent, func() {
					sendStatus(nodeStatus)
				})

				return
			}
		}

		sendStatus(nodeStatus)
	}

	wp.Start()
	unhook := deps.Protocol.Events.Engine.SyncManager.UpdatedStatus.Hook(onUpdate, event.WithWorkerPool(wp)).Unhook

	<-ctx.Done()
	unhook()

	// We need to wait until all tasks are done, otherwise we might call
	// "SendMsg" and "CloseSend" in parallel on the grpc stream, which is
	// not safe according to the grpc docs.
	wp.Shutdown()
	wp.ShutdownComplete.Wait()

	return ctx.Err()
}

func (s *Server) ReadNodeConfiguration(context.Context, *inx.NoParams) (*inx.NodeConfiguration, error) {
	protoParams := make([]*inx.RawProtocolParameters, 0)
	provider := deps.Protocol.Engines.Main.Get().Storage.Settings().APIProvider()
	for _, version := range provider.ProtocolEpochVersions() {
		protocolParams := provider.ProtocolParameters(version.Version)
		if protocolParams == nil {
			continue
		}

		rawParams, err := inx.WrapProtocolParameters(version.StartEpoch, protocolParams)
		if err != nil {
			return nil, err
		}
		protoParams = append(protoParams, rawParams)
	}

	return &inx.NodeConfiguration{
		BaseToken: &inx.BaseToken{
			Name:         deps.BaseToken.Name,
			TickerSymbol: deps.BaseToken.TickerSymbol,
			Unit:         deps.BaseToken.Unit,
			Subunit:      deps.BaseToken.Subunit,
			Decimals:     deps.BaseToken.Decimals,
		},
		ProtocolParameters: protoParams,
	}, nil
}
