package protocol

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	iotago "github.com/iotaledger/iota.go/v4"
)

// triggerEventIfCommitmentBelowThreshold triggers the given event if the given commitment is below the given threshold.
func triggerEventIfCommitmentBelowThreshold(event func(*Commitment) reactive.Event, commitment *Commitment, chainThreshold func(*Chain) reactive.Variable[iotago.SlotIndex]) {
	// only monitor the threshold after the parent event was triggered (minimize listeners to same threshold)
	event(commitment.parent.Get()).OnTrigger(func() {
		// since events only trigger once, we unsubscribe from the threshold after the trigger condition is met
		chainThreshold(commitment.chain.Get()).OnUpdateOnce(func(_, _ iotago.SlotIndex) {
			event(commitment).Trigger()
		}, func(_, slotIndex iotago.SlotIndex) bool {
			return commitment.Index() < slotIndex
		})
	})
}
