package promise

import (
	"sync"
)

// Promise is a promise that can be resolved or rejected.
type Promise[T any] struct {
	// successCallbacks are called when the promise is resolved successfully.
	successCallbacks []func(T)

	// errorCallbacks are called when the promise is rejected.
	errorCallbacks []func(error)

	// completeCallbacks are called when the promise is resolved or rejected.
	completeCallbacks []func()

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
func New[T any]() *Promise[T] {
	return &Promise[T]{
		successCallbacks:  make([]func(T), 0),
		errorCallbacks:    make([]func(error), 0),
		completeCallbacks: make([]func(), 0),
	}
}

// Resolve resolves the promise with the given result.
func (f *Promise[T]) Resolve(result T) *Promise[T] {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.complete {
		return f
	}

	f.result = result
	f.complete = true

	for _, callback := range f.successCallbacks {
		callback(result)
	}

	for _, callback := range f.completeCallbacks {
		callback()
	}

	return f
}

// Reject rejects the promise with the given error.
func (f *Promise[T]) Reject(err error) *Promise[T] {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.complete {
		return f
	}

	f.err = err
	f.complete = true

	for _, callback := range f.errorCallbacks {
		callback(err)
	}

	for _, callback := range f.completeCallbacks {
		callback()
	}

	return f
}

// OnSuccess registers a callback that is called when the promise is resolved.
func (f *Promise[T]) OnSuccess(callback func(result T)) *Promise[T] {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if !f.complete {
		f.successCallbacks = append(f.successCallbacks, callback)
	} else if f.err == nil {
		callback(f.result)
	}

	return f
}

// OnError registers a callback that is called when the promise is rejected.
func (f *Promise[T]) OnError(callback func(err error)) *Promise[T] {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if !f.complete {
		f.errorCallbacks = append(f.errorCallbacks, callback)
	} else if f.err != nil {
		callback(f.err)
	}

	return f
}

// OnComplete registers a callback that is called when the promise is resolved or rejected.
func (f *Promise[T]) OnComplete(callback func()) *Promise[T] {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if !f.complete {
		f.completeCallbacks = append(f.completeCallbacks, callback)
	} else {
		callback()
	}

	return f
}
