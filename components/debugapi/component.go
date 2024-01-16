package debugapi

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	hivedb "github.com/iotaledger/hive.go/kvstore/database"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/components/restapi"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

const (
	RouteValidators    = "/validators"
	RouteBlockMetadata = "/blocks/:" + api.ParameterBlockID + "/metadata"

	RouteChainManagerAllChainsDot      = "/all-chains"
	RouteChainManagerAllChainsRendered = "/all-chains/rendered"

	RouteCommitmentBySlotBlockIDs = "/commitments/by-slot/:" + api.ParameterSlot + "/blocks"

	RouteCommitmentBySlotTransactionIDs = "/commitments/by-slot/:" + api.ParameterSlot + "/transactions"
)

const (
	debugPrefixHealth byte = iota
	debugPrefixBlocks
)

func init() {
	Component = &app.Component{
		Name:      "DebugAPIV3",
		DepsFunc:  func(cDeps dependencies) { deps = cDeps },
		Configure: configure,
		Params:    params,
		IsEnabled: func(c *dig.Container) bool {
			return restapi.ParamsRestAPI.Enabled && ParamsDebugAPI.Enabled
		},
	}
}

var (
	Component *app.Component
	deps      dependencies

	blocksPerSlot         *shrinkingmap.ShrinkingMap[iotago.SlotIndex, []*blocks.Block]
	blocksPrunableStorage *prunable.BucketManager
)

type dependencies struct {
	dig.In

	Protocol         *protocol.Protocol
	AppInfo          *app.Info
	RestRouteManager *restapipkg.RestRouteManager
}

func configure() error {
	// check if RestAPI plugin is disabled
	if !Component.App().IsComponentEnabled(restapi.Component.Identifier()) {
		Component.LogPanic("RestAPI plugin needs to be enabled to use the DebugAPIV3 plugin")
	}

	blocksPerSlot = shrinkingmap.New[iotago.SlotIndex, []*blocks.Block]()
	blocksPrunableStorage = prunable.NewBucketManager(database.Config{
		Engine:    hivedb.EngineRocksDB,
		Directory: ParamsDebugAPI.Database.Path,

		Version:      1,
		PrefixHealth: []byte{debugPrefixHealth},
	}, func(err error) {
		Component.LogWarnf(">> DebugAPI Error: %s\n", err)
	}, prunable.WithMaxOpenDBs(ParamsDebugAPI.Database.MaxOpenDBs),
	)

	routeGroup := deps.RestRouteManager.AddRoute("debug/v2")

	deps.Protocol.Events.Engine.BlockDAG.BlockAttached.Hook(func(block *blocks.Block) {
		blocksPerSlot.Set(block.ID().Slot(), append(lo.Return1(blocksPerSlot.GetOrCreate(block.ID().Slot(), func() []*blocks.Block {
			return make([]*blocks.Block, 0)
		})), block))
	})

	deps.Protocol.Events.Engine.SlotGadget.SlotFinalized.Hook(func(index iotago.SlotIndex) {
		epoch := deps.Protocol.APIForSlot(index).TimeProvider().EpochFromSlot(index)
		if epoch < iotago.EpochIndex(ParamsDebugAPI.Database.Pruning.Threshold) {
			return
		}

		lastPruned, hasPruned := blocksPrunableStorage.LastPrunedEpoch()
		if hasPruned {
			lastPruned++
		}

		for i := lastPruned; i < epoch-iotago.EpochIndex(ParamsDebugAPI.Database.Pruning.Threshold); i++ {
			if err := blocksPrunableStorage.Prune(i); err != nil {
				Component.LogWarnf(">> DebugAPI Error: %s\n", err)
			}
		}

	}, event.WithWorkerPool(workerpool.NewGroup("DebugAPI").CreatePool("PruneDebugAPI", workerpool.WithWorkerCount(1))))

	deps.Protocol.Events.Engine.Notarization.SlotCommitted.Hook(func(scd *notarization.SlotCommittedDetails) {
		if err := storeTransactionsPerSlot(scd); err != nil {
			Component.LogWarnf(">> DebugAPI Error: %s\n", err)
		}
	})

	deps.Protocol.Events.Engine.EvictionState.SlotEvicted.Hook(func(index iotago.SlotIndex) {
		blocksInSlot, exists := blocksPerSlot.Get(index)
		if !exists {
			return
		}

		for _, block := range blocksInSlot {
			if block.ProtocolBlock() == nil {
				Component.LogInfof("block is a root block", block.ID())
				continue
			}

			epoch := deps.Protocol.APIForSlot(block.ID().Slot()).TimeProvider().EpochFromSlot(block.ID().Slot())
			blockStore, err := blocksPrunableStorage.Get(epoch, []byte{debugPrefixBlocks})
			if err != nil {
				panic(err)
			}

			err = blockStore.Set(lo.PanicOnErr(block.ID().Bytes()), lo.PanicOnErr(json.Marshal(BlockMetadataResponseFromBlock(block))))
			if err != nil {
				panic(err)
			}
		}

		blocksPerSlot.Delete(index)
	})

	routeGroup.GET(RouteBlockMetadata, func(c echo.Context) error {
		blockID, err := httpserver.ParseBlockIDParam(c, api.ParameterBlockID)
		if err != nil {
			return err
		}

		if block, exists := deps.Protocol.Engines.Main.Get().BlockCache.Block(blockID); exists && block.ProtocolBlock() != nil {
			response := BlockMetadataResponseFromBlock(block)

			return httpserver.JSONResponse(c, http.StatusOK, response)

		}

		epoch := deps.Protocol.APIForSlot(blockID.Slot()).TimeProvider().EpochFromSlot(blockID.Slot())
		blockStore, err := blocksPrunableStorage.Get(epoch, []byte{debugPrefixBlocks})
		if err != nil {
			return c.String(http.StatusInternalServerError, err.Error())
		}

		blockJSON, err := blockStore.Get(lo.PanicOnErr(blockID.Bytes()))
		if err != nil {
			return c.String(http.StatusInternalServerError, err.Error())
		}

		return c.Blob(http.StatusOK, echo.MIMEApplicationJSONCharsetUTF8, blockJSON)
	})

	routeGroup.GET(RouteValidators, func(c echo.Context) error {
		resp, err := validatorsSummary()
		if err != nil {
			return err
		}

		j, err := deps.Protocol.CommittedAPI().JSONEncode(resp)
		if err != nil {
			return err
		}

		return c.Blob(http.StatusOK, echo.MIMEApplicationJSON, j)
	})

	routeGroup.GET(RouteChainManagerAllChainsDot, func(c echo.Context) error {
		resp, err := chainManagerAllChainsDot()
		if err != nil {
			return err
		}

		return c.String(http.StatusOK, resp)
	})

	routeGroup.GET(RouteChainManagerAllChainsRendered, func(c echo.Context) error {
		renderedBytes, err := chainManagerAllChainsRendered()
		if err != nil {
			return err
		}

		return c.Blob(http.StatusOK, "image/png", renderedBytes)
	})

	routeGroup.GET(RouteCommitmentBySlotBlockIDs, func(c echo.Context) error {
		slot, err := httpserver.ParseSlotParam(c, api.ParameterSlot)
		if err != nil {
			return err
		}

		resp, err := getSlotBlockIDs(slot)
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	routeGroup.GET(RouteCommitmentBySlotTransactionIDs, func(c echo.Context) error {
		slot, err := httpserver.ParseSlotParam(c, api.ParameterSlot)
		if err != nil {
			return err
		}

		resp, err := getSlotTransactionIDs(slot)
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	return nil
}
