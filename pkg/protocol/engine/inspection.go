package engine

import "github.com/iotaledger/hive.go/runtime/inspection"

// Inspect inspects the Engine and its subcomponents.
func (e *Engine) Inspect(session ...inspection.Session) inspection.InspectedObject {
	return inspection.NewInspectedObject(e, func(_ inspection.InspectedObject) {
		// TODO: DUMP ENGINE STRUCTURE IN THE FUTURE
	}, session...)
}
