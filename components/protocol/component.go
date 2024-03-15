package protocol

import (
	"context"
	"os"

	"github.com/labstack/gommon/bytes"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/db"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/attestation/slotattestation"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/presolidfilter/presolidblockfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/upgrade/signalingupgradeorchestrator"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1"
	"github.com/iotaledger/iota-core/pkg/retainer/txretainer"
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
	ProtocolParameters []iotago.ProtocolParameters `serix:""`
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
		DatabaseEngine     db.Engine `name:"databaseEngine"`
		BaseToken          *BaseToken
		ProtocolParameters []iotago.ProtocolParameters
	}

	if err := c.Provide(func() cfgResult {
		dbEngine, err := db.EngineFromStringAllowed(ParamsDatabase.Engine, database.AllowedEnginesDefault)
		if err != nil {
			Component.LogPanic(err.Error())
		}

		return cfgResult{
			DatabaseEngine:     dbEngine,
			BaseToken:          &ParamsProtocol.BaseToken,
			ProtocolParameters: readProtocolParameters(),
		}
	}); err != nil {
		Component.LogPanic(err.Error())
	}

	return nil
}

func provide(c *dig.Container) error {
	type protocolDeps struct {
		dig.In

		DatabaseEngine     db.Engine `name:"databaseEngine"`
		ProtocolParameters []iotago.ProtocolParameters
		NetworkManager     network.Manager
	}

	return c.Provide(func(deps protocolDeps) *protocol.Protocol {
		pruningSizeEnabled := ParamsDatabase.Pruning.Size.Enabled
		pruningTargetDatabaseSizeBytes, err := bytes.Parse(ParamsDatabase.Pruning.Size.TargetSize)
		if err != nil {
			Component.LogPanicf("parameter %s invalid", Component.App().Config().GetParameterPath(&(ParamsDatabase.Pruning.Size.TargetSize)))
		}

		if pruningSizeEnabled && pruningTargetDatabaseSizeBytes == 0 {
			Component.LogPanicf("%s has to be specified if %s is enabled", Component.App().Config().GetParameterPath(&(ParamsDatabase.Pruning.Size.TargetSize)), Component.App().Config().GetParameterPath(&(ParamsDatabase.Pruning.Size.Enabled)))
		}

		return protocol.New(
			Component.Logger,
			workerpool.NewGroup("Protocol"),
			deps.NetworkManager,
			protocol.WithBaseDirectory(ParamsDatabase.Path),
			protocol.WithStorageOptions(
				storage.WithDBEngine(deps.DatabaseEngine),
				storage.WithPruningDelay(iotago.EpochIndex(ParamsDatabase.Pruning.Threshold)),
				storage.WithPruningSizeEnable(ParamsDatabase.Pruning.Size.Enabled),
				storage.WithPruningSizeMaxTargetSizeBytes(pruningTargetDatabaseSizeBytes),
				storage.WithPruningSizeReductionPercentage(ParamsDatabase.Pruning.Size.ReductionPercentage),
				storage.WithPruningSizeCooldownTime(ParamsDatabase.Pruning.Size.CooldownTime),
				storage.WithBucketManagerOptions(
					prunable.WithMaxOpenDBs(ParamsDatabase.MaxOpenDBs),
				),
			),
			protocol.WithSnapshotPath(ParamsProtocol.Snapshot.Path),
			protocol.WithCommitmentCheck(ParamsProtocol.CommitmentCheck),
			protocol.WithMaxAllowedWallClockDrift(ParamsProtocol.Filter.MaxAllowedClockDrift),
			protocol.WithPreSolidFilterProvider(
				presolidblockfilter.NewProvider(),
			),
			protocol.WithSybilProtectionProvider(
				sybilprotectionv1.NewProvider(),
			),
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(),
			),
			protocol.WithAttestationProvider(
				slotattestation.NewProvider(),
			),
			protocol.WithTransactionRetainerProvider(
				txretainer.NewProvider(
					txretainer.WithDebugStoreErrorMessages(ParamsRetainer.DebugStoreErrorMessages),
				),
			),
			protocol.WithUpgradeOrchestratorProvider(
				signalingupgradeorchestrator.NewProvider(signalingupgradeorchestrator.WithProtocolParameters(deps.ProtocolParameters...)),
			),
		)
	})
}

func configure() error {
	deps.Protocol.Network.OnBlockReceived(func(block *model.Block, _ peer.ID) {
		Component.LogDebugf("BlockReceived: %s", block.ID())
	})
	deps.Protocol.Network.OnCommitmentRequestReceived(func(commitmentID iotago.CommitmentID, _ peer.ID) {
		Component.LogDebugf("SlotCommitmentRequestReceived: %s", commitmentID)
	})

	deps.Protocol.Network.OnCommitmentReceived(func(commitment *model.Commitment, _ peer.ID) {
		Component.LogDebugf("SlotCommitmentReceived: %s", commitment.ID())
	})

	return nil
}

func run() error {
	return Component.Daemon().BackgroundWorker(Component.Name, func(ctx context.Context) {
		if err := deps.Protocol.Run(ctx); err != nil {
			if !ierrors.Is(err, context.Canceled) {
				Component.LogFatal("Error running the Protocol: %s", err.Error())
			}
		}

		//nolint:contextcheck // context might be canceled
		resetProtocolParameters()

		Component.LogInfo("Gracefully shutting down the Protocol...")
	}, daemon.PriorityProtocol)
}
