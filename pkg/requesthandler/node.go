package requesthandler

import (
	"github.com/iotaledger/iota-core/pkg/protocol/engine/clock"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/syncmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (r *RequestHandler) APIProvider() *iotago.EpochBasedProvider {
	return r.protocol.Engines.Main.Get().Storage.Settings().APIProvider()
}

func (r *RequestHandler) Snapshot() *clock.Snapshot {
	return r.protocol.Engines.Main.Get().Clock.Snapshot()
}

func (r *RequestHandler) SyncStatus() *syncmanager.SyncStatus {
	return r.protocol.Engines.Main.Get().SyncManager.SyncStatus()
}
