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
	for _, callback := range func() (callbacks []func()) {
		f.mutex.Lock()
		defer f.mutex.Unlock()

		if callbacks = f.triggeredCallbacks; callbacks != nil {
			f.triggeredCallbacks = nil
		}

		return callbacks
	}() {
		callback()
	}
}

func (f *Event) OnTrigger(callback func()) {
	if func() (triggeredAlready bool) {
		f.mutex.Lock()
		defer f.mutex.Unlock()

		if triggeredAlready = f.triggeredCallbacks == nil; !triggeredAlready {
			f.triggeredCallbacks = append(f.triggeredCallbacks, callback)
		}

		return triggeredAlready
	}() {
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
	for _, callback := range func() (callbacks []func(T)) {
		f.mutex.Lock()
		defer f.mutex.Unlock()

		if callbacks = f.triggeredCallbacks; callbacks != nil {
			f.triggeredCallbacks = nil
			f.triggerResult = &arg
		}

		return callbacks
	}() {
		callback(arg)
	}
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
