package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	iotago "github.com/iotaledger/iota.go/v4"
)

type SlotEviction struct {
	evictionEvents *shrinkingmap.ShrinkingMap[iotago.SlotIndex, reactive.Event]

	lastEvictedSlotIndex reactive.Variable[iotago.SlotIndex]
}

func NewSlotEviction() *SlotEviction {
	return &SlotEviction{
		evictionEvents:       shrinkingmap.New[iotago.SlotIndex, reactive.Event](),
		lastEvictedSlotIndex: reactive.NewVariable[iotago.SlotIndex](),
	}
}

func (c *SlotEviction) LastEvictedSlotIndex() reactive.Variable[iotago.SlotIndex] {
	return c.lastEvictedSlotIndex
}

func (c *SlotEviction) EvictedEvent(index iotago.SlotIndex) reactive.Event {
	slotEvictedEvent := defaultTriggeredEvent

	c.lastEvictedSlotIndex.Compute(func(lastEvictedSlotIndex iotago.SlotIndex) iotago.SlotIndex {
		if index > lastEvictedSlotIndex {
			slotEvictedEvent, _ = c.evictionEvents.GetOrCreate(index, reactive.NewEvent)
		}

		return lastEvictedSlotIndex
	})

	return slotEvictedEvent
}

func (c *SlotEviction) Evict(slotIndex iotago.SlotIndex) {
	slotEvictedEventsToTrigger := make([]reactive.Event, 0)

	c.lastEvictedSlotIndex.Compute(func(lastEvictedSlotIndex iotago.SlotIndex) iotago.SlotIndex {
		if slotIndex <= lastEvictedSlotIndex {
			return lastEvictedSlotIndex
		}

		for i := lastEvictedSlotIndex + 1; i <= slotIndex; i++ {
			if slotEvictedEvent, exists := c.evictionEvents.Get(i); exists {
				slotEvictedEventsToTrigger = append(slotEvictedEventsToTrigger, slotEvictedEvent)
			}
		}

		return slotIndex
	})

	for _, slotEvictedEvent := range slotEvictedEventsToTrigger {
		slotEvictedEvent.Trigger()
	}
}

var defaultTriggeredEvent = reactive.NewEvent()

func init() {
	defaultTriggeredEvent.Trigger()
}
