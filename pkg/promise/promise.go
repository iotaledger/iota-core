package promise

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/orderedmap"
)

// Promise is a promise that can be resolved or rejected.
type Promise[T any] struct {
	// successCallbacks are called when the promise is resolved successfully.
	successCallbacks *orderedmap.OrderedMap[CallbackID, func(T)]

	// errorCallbacks are called when the promise is rejected.
	errorCallbacks *orderedmap.OrderedMap[CallbackID, func(error)]

	// completeCallbacks are called when the promise is resolved or rejected.
	completeCallbacks *orderedmap.OrderedMap[CallbackID, func()]

	// result is the result of the promise.
	result T

	// err is the error of the promise.
	err error

	// complete is true if the promise is resolved or rejected.
	complete bool

	// mutex is used to synchronize access to the promise.
	mutex sync.RWMutex
}

// New creates a new promise.
func New[T any](optResolver ...func(p *Promise[T])) *Promise[T] {
	p := &Promise[T]{
		successCallbacks:  orderedmap.New[CallbackID, func(T)](),
		errorCallbacks:    orderedmap.New[CallbackID, func(error)](),
		completeCallbacks: orderedmap.New[CallbackID, func()](),
	}

	if len(optResolver) > 0 {
		optResolver[0](p)
	}

	return p
}

// Resolve resolves the promise with the given result.
func (f *Promise[T]) Resolve(result T) *Promise[T] {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.complete {
		return f
	}

	f.successCallbacks.ForEach(func(key CallbackID, callback func(T)) bool {
		callback(result)
		return true
	})

	f.completeCallbacks.ForEach(func(key CallbackID, callback func()) bool {
		callback()
		return true
	})

	f.successCallbacks = nil
	f.errorCallbacks = nil
	f.completeCallbacks = nil
	f.result = result
	f.complete = true

	return f
}

// Reject rejects the promise with the given error.
func (f *Promise[T]) Reject(err error) *Promise[T] {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.complete {
		return f
	}

	f.errorCallbacks.ForEach(func(key CallbackID, callback func(error)) bool {
		callback(err)
		return true
	})

	f.completeCallbacks.ForEach(func(key CallbackID, callback func()) bool {
		callback()
		return true
	})

	f.successCallbacks = nil
	f.errorCallbacks = nil
	f.completeCallbacks = nil
	f.err = err
	f.complete = true

	return f
}

// OnSuccess registers a callback that is called when the promise is resolved.
func (f *Promise[T]) OnSuccess(callback func(result T)) (cancel func()) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.complete {
		if f.err == nil {
			callback(f.result)
		}

		return func() {}
	}

	callbackID := NewCallbackID()
	f.successCallbacks.Set(callbackID, callback)

	return func() {
		f.mutex.Lock()
		defer f.mutex.Unlock()

		if f.successCallbacks != nil {
			f.successCallbacks.Delete(callbackID)
		}
	}
}

// OnError registers a callback that is called when the promise is rejected.
func (f *Promise[T]) OnError(callback func(err error)) func() {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.complete {
		if f.err != nil {
			callback(f.err)
		}

		return func() {}
	}

	callbackID := NewCallbackID()
	f.errorCallbacks.Set(callbackID, callback)

	return func() {
		f.mutex.Lock()
		defer f.mutex.Unlock()

		if f.errorCallbacks != nil {
			f.errorCallbacks.Delete(callbackID)
		}
	}
}

// OnComplete registers a callback that is called when the promise is resolved or rejected.
func (f *Promise[T]) OnComplete(callback func()) func() {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.complete {
		callback()

		return func() {}
	}

	callbackID := NewCallbackID()
	f.completeCallbacks.Set(callbackID, callback)

	return func() {
		f.mutex.Lock()
		defer f.mutex.Unlock()

		f.completeCallbacks.Delete(callbackID)
	}
}

// WasResolved returns true if the promise was resolved.
func (f *Promise[T]) WasResolved() bool {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	return f.complete && f.err == nil
}

// WasRejected returns true if the promise was rejected.
func (f *Promise[T]) WasRejected() bool {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	return f.complete && f.err != nil
}

// WasCompleted returns true if the promise was resolved or rejected.
func (f *Promise[T]) WasCompleted() bool {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	return f.complete
}

// IsEmpty returns true if the promise has no callbacks.
func (f *Promise[T]) IsEmpty() bool {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	return f.successCallbacks.IsEmpty() && f.errorCallbacks.IsEmpty() && f.completeCallbacks.IsEmpty()
}
