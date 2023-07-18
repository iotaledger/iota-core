package inx

import (
	"context"
	"time"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	inx "github.com/iotaledger/inx/go"
	"github.com/iotaledger/iota-core/pkg/protocol/syncmanager"
)

func inxNodeStatus(status *syncmanager.SyncStatus) *inx.NodeStatus {
	return &inx.NodeStatus{
		IsHealthy:              status.NodeSynced, //TODO: check if we have peers
		IsSynced:               status.NodeSynced,
		IsAlmostSynced:         status.NodeSynced, //TODO: check if we are close to sync
		LastAcceptedBlockSlot:  uint64(status.LastAcceptedBlockSlot),
		LastConfirmedBlockSlot: uint64(status.LastConfirmedBlockSlot),
		LatestCommitment:       inxCommitment(status.LatestCommitment),
		LatestFinalizedSlot:    uint64(status.LatestFinalizedSlot),
	}
}

func (s *Server) ReadNodeStatus(context.Context, *inx.NoParams) (*inx.NodeStatus, error) {
	return inxNodeStatus(deps.Protocol.SyncManager.SyncStatus()), nil
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
	wp := workerpool.New("ListenToNodeStatus", workerCount)

	onUpdate := func(status *syncmanager.SyncStatus) {
		if lastUpdateTimer != nil {
			lastUpdateTimer.Stop()
			lastUpdateTimer = nil
		}

		nodeStatus := inxNodeStatus(status)

		// Use cool-down if the node is syncing
		if coolDownDuration > 0 && !nodeStatus.IsAlmostSynced {
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
	unhook := deps.Protocol.Events.SyncManager.UpdatedStatus.Hook(onUpdate, event.WithWorkerPool(wp)).Unhook

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
	var protoParams []*inx.RawProtocolParameters
	for _, version := range deps.Protocol.MainEngineInstance().Storage.Settings().ProtocolVersions() {
		apiForVersion, err := deps.Protocol.APIForVersion(version.Version)
		if err != nil {
			return nil, err
		}

		rawParams, err := inx.WrapProtocolParameters(version.StartEpoch, apiForVersion.ProtocolParameters())
		if err != nil {
			return nil, err
		}
		protoParams = append(protoParams, rawParams)
	}

	return &inx.NodeConfiguration{
		BaseToken: &inx.BaseToken{
			//Name:            deps.BaseToken.Name,
			//TickerSymbol:    deps.BaseToken.TickerSymbol,
			//Unit:            deps.BaseToken.Unit,
			//Subunit:         deps.BaseToken.Subunit,
			//Decimals:        deps.BaseToken.Decimals,
			//UseMetricPrefix: deps.BaseToken.UseMetricPrefix,
		},
		ProtocolParameters: protoParams,
	}, nil
}
