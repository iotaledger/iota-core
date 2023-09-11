package protocol

import (
	"context"
	"os"
	"time"

	"github.com/labstack/gommon/bytes"
	"go.uber.org/dig"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/ierrors"
	hivedb "github.com/iotaledger/hive.go/kvstore/database"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network/p2p"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/attestation/slotattestation"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/commitmentfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/blockfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/upgrade/signalingupgradeorchestrator"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1"
	"github.com/iotaledger/iota-core/pkg/storage"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
)

func init() {
	Component = &app.Component{
		Name:             "Protocol",
		DepsFunc:         func(cDeps dependencies) { deps = cDeps },
		Params:           params,
		InitConfigParams: initConfigParams,
		Provide:          provide,
		Configure:        configure,
		Run:              run,
	}
}

var (
	Component *app.Component
	deps      dependencies
)

type dependencies struct {
	dig.In

	Protocol *protocol.Protocol
}

type jsonProtocolParameters struct {
	ProtocolParameters []iotago.ProtocolParameters `serix:"0,mapKey=protocolParameters"`
}

func readProtocolParameters() []iotago.ProtocolParameters {
	fileBytes, err := os.ReadFile(ParamsProtocol.ProtocolParametersPath)
	if err != nil {
		Component.LogInfof("No protocol parameters file (%s) found, skipping import: %s", ParamsProtocol.ProtocolParametersPath, err)
		return nil
	}

	parsedParams := &jsonProtocolParameters{}
	if err := iotago.CommonSerixAPI().JSONDecode(context.Background(), fileBytes, parsedParams); err != nil {
		Component.LogWarnf("Error parsing protocol parameters file (%s): %s", ParamsProtocol.ProtocolParametersPath, err)
		return nil
	}

	return parsedParams.ProtocolParameters
}

func resetProtocolParameters() {
	bytesToWrite, err := iotago.CommonSerixAPI().JSONEncode(context.Background(), jsonProtocolParameters{})
	if err != nil {
		Component.LogInfof("Error writing protocol parameters file (%s): %s", ParamsProtocol.ProtocolParametersPath, err)
		return
	}

	if err := os.WriteFile(ParamsProtocol.ProtocolParametersPath, bytesToWrite, 0600); err != nil {
		Component.LogInfof("Error writing protocol parameters file (%s): %s", ParamsProtocol.ProtocolParametersPath, err)
		return
	}
}

func initConfigParams(c *dig.Container) error {
	type cfgResult struct {
		dig.Out
		DatabaseEngine     hivedb.Engine `name:"databaseEngine"`
		BaseToken          *BaseToken
		ProtocolParameters []iotago.ProtocolParameters
	}

	if err := c.Provide(func() cfgResult {
		dbEngine, err := hivedb.EngineFromStringAllowed(ParamsDatabase.Engine, database.AllowedEnginesDefault)
		if err != nil {
			Component.LogPanic(err)
		}

		return cfgResult{
			DatabaseEngine:     dbEngine,
			BaseToken:          &ParamsProtocol.BaseToken,
			ProtocolParameters: readProtocolParameters(),
		}
	}); err != nil {
		Component.LogPanic(err)
	}

	return nil
}

func provide(c *dig.Container) error {
	type protocolDeps struct {
		dig.In

		DatabaseEngine     hivedb.Engine `name:"databaseEngine"`
		ProtocolParameters []iotago.ProtocolParameters
		P2PManager         *p2p.Manager
	}

	return c.Provide(func(deps protocolDeps) *protocol.Protocol {
		pruningSizeEnabled := ParamsDatabase.Size.Enabled
		pruningTargetDatabaseSizeBytes, err := bytes.Parse(ParamsDatabase.Size.TargetSize)
		if err != nil {
			Component.LogPanicf("parameter %s invalid", Component.App().Config().GetParameterPath(&(ParamsDatabase.Size.TargetSize)))
		}

		if pruningSizeEnabled && pruningTargetDatabaseSizeBytes == 0 {
			Component.LogPanicf("%s has to be specified if %s is enabled", Component.App().Config().GetParameterPath(&(ParamsDatabase.Size.TargetSize)), Component.App().Config().GetParameterPath(&(ParamsDatabase.Size.Enabled)))
		}

		return protocol.New(
			workerpool.NewGroup("Protocol"),
			deps.P2PManager,
			protocol.WithBaseDirectory(ParamsDatabase.Path),
			protocol.WithStorageOptions(
				storage.WithDBEngine(deps.DatabaseEngine),
				storage.WithPruningDelay(iotago.EpochIndex(ParamsDatabase.PruningThreshold)),
				storage.WithPruningSizeEnable(ParamsDatabase.Size.Enabled),
				storage.WithPruningSizeMaxTargetSizeBytes(pruningTargetDatabaseSizeBytes),
				storage.WithPruningSizeReductionPercentage(ParamsDatabase.Size.ReductionPercentage),
				storage.WithPruningSizeCooldownTime(ParamsDatabase.Size.CooldownTime),
				storage.WithBucketManagerOptions(
					prunable.WithMaxOpenDBs(ParamsDatabase.MaxOpenDBs),
				),
			),
			protocol.WithSnapshotPath(ParamsProtocol.Snapshot.Path),
			protocol.WithSybilProtectionProvider(
				sybilprotectionv1.NewProvider(),
			),
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(),
			),
			protocol.WithAttestationProvider(
				slotattestation.NewProvider(),
			),
			protocol.WithFilterProvider(
				blockfilter.NewProvider(
					blockfilter.WithMaxAllowedWallClockDrift(ParamsProtocol.Filter.MaxAllowedClockDrift),
				),
			),
			protocol.WithUpgradeOrchestratorProvider(
				signalingupgradeorchestrator.NewProvider(signalingupgradeorchestrator.WithProtocolParameters(deps.ProtocolParameters...)),
			),
		)
	})
}

func configure() error {
	deps.Protocol.Events.Error.Hook(func(err error) {
		Component.LogErrorf("Error in Protocol: %s", err)
	})

	deps.Protocol.Events.Network.Error.Hook(func(err error, id peer.ID) {
		Component.LogErrorf("NetworkError: %s Source: %s", err.Error(), id)
	})

	deps.Protocol.Events.Network.BlockReceived.Hook(func(block *model.Block, source peer.ID) {
		Component.LogDebugf("BlockReceived: %s", block.ID())
	})

	deps.Protocol.Events.Engine.BlockProcessed.Hook(func(blockID iotago.BlockID) {
		Component.LogDebugf("BlockProcessed: %s", blockID)
	})

	deps.Protocol.Events.Engine.AcceptedBlockProcessed.Hook(func(block *blocks.Block) {
		Component.LogDebugf("AcceptedBlockProcessed: %s", block.ID())
	})

	deps.Protocol.Events.Engine.Filter.BlockPreFiltered.Hook(func(event *filter.BlockPreFilteredEvent) {
		Component.LogDebugf("BlockPreFiltered: %s - %s", event.Block.ID(), event.Reason.Error())
	})

	deps.Protocol.Events.Engine.Filter.BlockPreAllowed.Hook(func(blk *model.Block) {
		Component.LogDebugf("BlockPreAllowed: %s - %s", blk.ID())
	})

	deps.Protocol.Events.Engine.CommitmentFilter.BlockAllowed.Hook(func(block *blocks.Block) {
		Component.LogDebugf("CommitmentFilter.BlockAllowed: %s\n", block.ID())
	})

	deps.Protocol.Events.Engine.CommitmentFilter.BlockFiltered.Hook(func(event *commitmentfilter.BlockFilteredEvent) {
		Component.LogWarnf("CommitmentFilter.BlockFiltered: %s - %s\n", event.Block.ID(), event.Reason.Error())
	})

	deps.Protocol.Events.Engine.TipManager.BlockAdded.Hook(func(tip tipmanager.TipMetadata) {
		Component.LogDebugf("BlockAdded to tip pool: %s; is strong: %v; is weak: %v", tip.ID(), tip.IsStrongTip(), tip.IsWeakTip())
	})

	deps.Protocol.Events.Engine.BlockDAG.BlockSolid.Hook(func(block *blocks.Block) {
		Component.LogDebugf("BlockSolid: %s", block.ID())
	})

	deps.Protocol.Events.Engine.BlockDAG.BlockInvalid.Hook(func(block *blocks.Block, err error) {
		Component.LogDebugf("BlockInvalid in blockDAG: %s, error: %v", block.ID(), err.Error())
	})

	deps.Protocol.Events.Engine.Booker.BlockBooked.Hook(func(block *blocks.Block) {
		Component.LogDebugf("BlockBooked: %s", block.ID())
	})

	deps.Protocol.Events.Engine.Booker.BlockInvalid.Hook(func(block *blocks.Block, err error) {
		Component.LogDebugf("BlockInvalid in booker: %s, error: %v", block.ID(), err.Error())
	})

	deps.Protocol.Events.Engine.BlockGadget.BlockPreAccepted.Hook(func(block *blocks.Block) {
		Component.LogDebugf("BlockPreAccepted: %s", block.ID())
	})

	deps.Protocol.Events.Engine.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
		Component.LogDebugf("BlockAccepted: %s", block.ID())
	})

	deps.Protocol.Events.Engine.BlockGadget.BlockPreConfirmed.Hook(func(block *blocks.Block) {
		Component.LogDebugf("BlockPreConfirmed: %s", block.ID())
	})

	deps.Protocol.Events.Engine.BlockGadget.BlockConfirmed.Hook(func(block *blocks.Block) {
		Component.LogDebugf("BlockConfirmed: %s", block.ID())
	})

	deps.Protocol.Events.Engine.Clock.AcceptedTimeUpdated.Hook(func(time time.Time) {
		Component.LogDebugf("AcceptedTimeUpdated: Slot %d @ %s", deps.Protocol.CurrentAPI().TimeProvider().SlotFromTime(time), time)
	})

	deps.Protocol.Events.Engine.Clock.ConfirmedTimeUpdated.Hook(func(time time.Time) {
		Component.LogDebugf("ConfirmedTimeUpdated: Slot %d @ %s", deps.Protocol.CurrentAPI().TimeProvider().SlotFromTime(time), time)
	})

	deps.Protocol.Events.Engine.Notarization.SlotCommitted.Hook(func(details *notarization.SlotCommittedDetails) {
		Component.LogInfof("SlotCommitted: %s - %d", details.Commitment.ID(), details.Commitment.Index())
	})

	deps.Protocol.Events.Engine.SlotGadget.SlotFinalized.Hook(func(index iotago.SlotIndex) {
		Component.LogInfof("SlotFinalized: %d", index)
	})

	deps.Protocol.Events.Engine.Scheduler.BlockScheduled.Hook(func(block *blocks.Block) {
		Component.LogDebugf("BlockScheduled: %s", block.ID())
	})

	deps.Protocol.Events.Engine.Scheduler.BlockDropped.Hook(func(block *blocks.Block, err error) {
		Component.LogDebugf("BlockDropped: %s; reason: %s", block.ID(), err)
	})

	deps.Protocol.Events.Engine.Scheduler.BlockSkipped.Hook(func(block *blocks.Block) {
		Component.LogDebugf("BlockSkipped: %s", block.ID())
	})

	deps.Protocol.Events.ChainManager.RequestCommitment.Hook(func(id iotago.CommitmentID) {
		Component.LogDebugf("RequestCommitment: %s", id)
	})

	deps.Protocol.Events.Network.SlotCommitmentRequestReceived.Hook(func(commitmentID iotago.CommitmentID, id peer.ID) {
		Component.LogDebugf("SlotCommitmentRequestReceived: %s", commitmentID)
	})

	deps.Protocol.Events.Network.SlotCommitmentReceived.Hook(func(commitment *model.Commitment, id peer.ID) {
		Component.LogDebugf("SlotCommitmentReceived: %s", commitment.ID())
	})

	deps.Protocol.Events.Engine.SybilProtection.CommitteeSelected.Hook(func(committee *account.Accounts, epoch iotago.EpochIndex) {
		Component.LogInfof("CommitteeSelected: Epoch %d - %s (reused: %t)", epoch, committee.IDs(), committee.IsReused())
	})

	deps.Protocol.Events.Engine.SybilProtection.RewardsCommitted.Hook(func(epoch iotago.EpochIndex) {
		Component.LogInfof("RewardsCommitted: Epoch %d", epoch)
	})

	deps.Protocol.Events.Engine.Booker.BlockInvalid.Hook(func(block *blocks.Block, err error) {
		Component.LogWarnf("Booker BlockInvalid: Block %s - %s", block.ID(), err.Error())
	})

	deps.Protocol.Events.Engine.SeatManager.OnlineCommitteeSeatAdded.Hook(func(seatIndex account.SeatIndex, account iotago.AccountID) {
		Component.LogWarnf("OnlineCommitteeSeatAdded: %s - %d", account.ToHex(), seatIndex)
	})

	deps.Protocol.Events.Engine.SeatManager.OnlineCommitteeSeatRemoved.Hook(func(seatIndex account.SeatIndex) {
		Component.LogWarnf("OnlineCommitteeSeatRemoved: seatIndex: %d", seatIndex)
	})

	deps.Protocol.Events.Engine.Booker.TransactionInvalid.Hook(func(transaction mempool.TransactionMetadata, reason error) {
		Component.LogWarnf("TransactionInvalid: transaction %s - %s", transaction.ID(), reason.Error())
	})

	return nil
}

func run() error {
	return Component.Daemon().BackgroundWorker(Component.Name, func(ctx context.Context) {
		if err := deps.Protocol.Run(ctx); err != nil {
			if !ierrors.Is(err, context.Canceled) {
				Component.LogErrorfAndExit("Error running the Protocol: %s", err.Error())
			}
		}

		//nolint:contextcheck // context might be canceled
		resetProtocolParameters()

		Component.LogInfo("Gracefully shutting down the Protocol...")
	}, daemon.PriorityProtocol)
}
