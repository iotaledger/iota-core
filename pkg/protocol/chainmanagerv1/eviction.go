package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
)

type EvictionState[Type SlotType] struct {
	evictionEvents *shrinkingmap.ShrinkingMap[Type, reactive.Event]

	lastEvictedSlotIndex reactive.Variable[Type]
}

func NewEvictionState[Type SlotType]() *EvictionState[Type] {
	return &EvictionState[Type]{
		evictionEvents:       shrinkingmap.New[Type, reactive.Event](),
		lastEvictedSlotIndex: reactive.NewVariable[Type](),
	}
}

func (c *EvictionState[Type]) LastEvictedSlot() reactive.Variable[Type] {
	return c.lastEvictedSlotIndex
}

func (c *EvictionState[Type]) EvictionEvent(slot Type) reactive.Event {
	evictionEvent := evictedSlotEvent

	c.lastEvictedSlotIndex.Compute(func(lastEvictedSlotIndex Type) Type {
		if slot > lastEvictedSlotIndex {
			evictionEvent, _ = c.evictionEvents.GetOrCreate(slot, reactive.NewEvent)
		}
		return lastEvictedSlotIndex
	})

	return evictionEvent
}

func (c *EvictionState[Type]) Evict(slot Type) {
	for _, slotEvictedEvent := range c.evict(slot) {
		slotEvictedEvent.Trigger()
	}
}

func (c *EvictionState[Type]) evict(slot Type) (eventsToTrigger []reactive.Event) {
	c.lastEvictedSlotIndex.Compute(func(lastEvictedSlotIndex Type) Type {
		if slot <= lastEvictedSlotIndex {
			return lastEvictedSlotIndex
		}

		for i := lastEvictedSlotIndex + Type(1); i <= slot; i = i + Type(1) {
			if slotEvictedEvent, exists := c.evictionEvents.Get(i); exists {
				eventsToTrigger = append(eventsToTrigger, slotEvictedEvent)
			}
		}

		return slot
	})

	return eventsToTrigger
}

type SlotType interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr | ~float32 | ~float64 | ~string
}

var evictedSlotEvent = reactive.NewEvent()

func init() {
	evictedSlotEvent.Trigger()
}
