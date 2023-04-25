package promise

import (
	"sync"
)

type Event struct {
	triggeredCallbacks []func()

	mutex sync.RWMutex
}

func NewEvent() *Event {
	return &Event{
		triggeredCallbacks: make([]func(), 0),
	}
}

func (f *Event) Trigger() {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	for _, callback := range f.triggeredCallbacks {
		callback()
	}
	f.triggeredCallbacks = nil
}

func (f *Event) OnTrigger(callback func()) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.triggeredCallbacks != nil {
		f.triggeredCallbacks = append(f.triggeredCallbacks, callback)
	} else {
		callback()
	}
}

func (f *Event) WasTriggered() bool {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	return f.triggeredCallbacks == nil
}

type Event1[T any] struct {
	triggeredCallbacks []func(T)
	triggerResult      *T

	mutex sync.RWMutex
}

func NewEvent1[T any]() *Event1[T] {
	return &Event1[T]{
		triggeredCallbacks: make([]func(T), 0),
	}
}

func (f *Event1[T]) Trigger(arg T) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	for _, callback := range f.triggeredCallbacks {
		callback(arg)
	}
	f.triggerResult = &arg
	f.triggeredCallbacks = nil
}

func (f *Event1[T]) OnTrigger(callback func(T)) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.triggeredCallbacks != nil {
		f.triggeredCallbacks = append(f.triggeredCallbacks, callback)
	} else {
		callback(*f.triggerResult)
	}
}

func (f *Event1[T]) WasTriggered() bool {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	return f.triggeredCallbacks == nil
}
