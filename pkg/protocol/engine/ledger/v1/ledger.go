package ledgerv1

import (
	"golang.org/x/xerrors"
	"iota-core/pkg/core/promise"
	"iota-core/pkg/protocol/engine/ledger"

	"github.com/iotaledger/hive.go/runtime/workerpool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Ledger struct {
	ioWorker *workerpool.WorkerPool
}

func New(workers *workerpool.Group) *Ledger {
	return &Ledger{
		ioWorker: workers.CreatePool("io-worker", 1),
	}
}

func (m *Ledger) ResolveState(id iotago.OutputID) *promise.Promise[ledger.State] {
	p := promise.New[ledger.State]()

	m.ioWorker.Submit(func() {
		if output, exists := m.loadOutput(id); exists {
			p.Resolve(output)
		} else {
			p.Reject(xerrors.Errorf("output %s not found: %w", id, ledger.ErrStateNotFound))
		}
	})

	return p
}

func (m *Ledger) loadOutput(id iotago.OutputID) (ledger.State, bool) {
	// TODO
	return nil, false
}
