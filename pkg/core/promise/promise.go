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
func (p *Promise[T]) Resolve(result T) *Promise[T] {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.complete {
		return p
	}

	p.successCallbacks.ForEach(func(key CallbackID, callback func(T)) bool {
		callback(result)
		return true
	})

	p.completeCallbacks.ForEach(func(key CallbackID, callback func()) bool {
		callback()
		return true
	})

	p.successCallbacks = nil
	p.errorCallbacks = nil
	p.completeCallbacks = nil
	p.result = result
	p.complete = true

	return p
}

// Reject rejects the promise with the given error.
func (p *Promise[T]) Reject(err error) *Promise[T] {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.complete {
		return p
	}

	p.errorCallbacks.ForEach(func(key CallbackID, callback func(error)) bool {
		callback(err)
		return true
	})

	p.completeCallbacks.ForEach(func(key CallbackID, callback func()) bool {
		callback()
		return true
	})

	p.successCallbacks = nil
	p.errorCallbacks = nil
	p.completeCallbacks = nil
	p.err = err
	p.complete = true

	return p
}

// OnSuccess registers a callback that is called when the promise is resolved.
func (p *Promise[T]) OnSuccess(callback func(result T)) (cancel func()) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.complete {
		if p.err == nil {
			callback(p.result)
		}

		return func() {}
	}

	callbackID := NewCallbackID()
	p.successCallbacks.Set(callbackID, callback)

	return func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		if p.successCallbacks != nil {
			p.successCallbacks.Delete(callbackID)
		}
	}
}

// OnError registers a callback that is called when the promise is rejected.
func (p *Promise[T]) OnError(callback func(err error)) func() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.complete {
		if p.err != nil {
			callback(p.err)
		}

		return func() {}
	}

	callbackID := NewCallbackID()
	p.errorCallbacks.Set(callbackID, callback)

	return func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		if p.errorCallbacks != nil {
			p.errorCallbacks.Delete(callbackID)
		}
	}
}

func (p *Promise[T]) WaitComplete() {
	var waitRequestComplete sync.WaitGroup

	waitRequestComplete.Add(1)
	p.OnComplete(waitRequestComplete.Done)

	waitRequestComplete.Wait()
}

// OnComplete registers a callback that is called when the promise is resolved or rejected.
func (p *Promise[T]) OnComplete(callback func()) func() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.complete {
		callback()

		return func() {}
	}

	callbackID := NewCallbackID()
	p.completeCallbacks.Set(callbackID, callback)

	return func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		p.completeCallbacks.Delete(callbackID)
	}
}

// WasResolved returns true if the promise was resolved.
func (p *Promise[T]) WasResolved() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.complete && p.err == nil
}

// WasRejected returns true if the promise was rejected.
func (p *Promise[T]) WasRejected() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.complete && p.err != nil
}

// WasCompleted returns true if the promise was resolved or rejected.
func (p *Promise[T]) WasCompleted() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.complete
}

// IsEmpty returns true if the promise has no updateCallbacks.
func (p *Promise[T]) IsEmpty() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.successCallbacks.IsEmpty() && p.errorCallbacks.IsEmpty() && p.completeCallbacks.IsEmpty()
}
