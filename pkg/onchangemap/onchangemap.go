package onchangemap

import (
	"fmt"
	"sync"

	"github.com/iotaledger/hive.go/constraints"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/runtime/options"
)

// Item represents an item in the OnChangeMap.
type Item[K comparable, C constraints.ComparableStringer[K]] interface {
	ID() C
	Clone() Item[K, C]
}

// OnChangeMap is a map that executes callbacks if the map or an item is modified,
// in case callbackEnabled is true.
type OnChangeMap[K comparable, C constraints.ComparableStringer[K], I Item[K, C]] struct {
	mutex sync.RWMutex

	m                *shrinkingmap.ShrinkingMap[K, I]
	callbacksEnabled bool

	changedCallback      func([]I) error
	itemAddedCallback    func(I) error
	itemModifiedCallback func(I) error
	itemDeletedCallback  func(I) error
}

// WithChangedCallback is triggered when something in the OnChangeMap is changed (added/modified/deleted).
func WithChangedCallback[K comparable, C constraints.ComparableStringer[K], I Item[K, C]](changedCallback func([]I) error) options.Option[OnChangeMap[K, C, I]] {
	return func(r *OnChangeMap[K, C, I]) {
		r.changedCallback = changedCallback
	}
}

// WithItemAddedCallback is triggered when a new item is added.
func WithItemAddedCallback[K comparable, C constraints.ComparableStringer[K], I Item[K, C]](itemAddedCallback func(I) error) options.Option[OnChangeMap[K, C, I]] {
	return func(r *OnChangeMap[K, C, I]) {
		r.itemAddedCallback = itemAddedCallback
	}
}

// WithItemModifiedCallback is triggered when an item is modified.
func WithItemModifiedCallback[K comparable, C constraints.ComparableStringer[K], I Item[K, C]](itemModifiedCallback func(I) error) options.Option[OnChangeMap[K, C, I]] {
	return func(r *OnChangeMap[K, C, I]) {
		r.itemModifiedCallback = itemModifiedCallback
	}
}

// WithItemDeletedCallback is triggered when an item is deleted.
func WithItemDeletedCallback[K comparable, C constraints.ComparableStringer[K], I Item[K, C]](itemDeletedCallback func(I) error) options.Option[OnChangeMap[K, C, I]] {
	return func(r *OnChangeMap[K, C, I]) {
		r.itemDeletedCallback = itemDeletedCallback
	}
}

// NewOnChangeMap creates a new OnChangeMap.
func NewOnChangeMap[K comparable, C constraints.ComparableStringer[K], I Item[K, C]](opts ...options.Option[OnChangeMap[K, C, I]]) *OnChangeMap[K, C, I] {
	return options.Apply(&OnChangeMap[K, C, I]{
		m:                    shrinkingmap.New[K, I](),
		callbacksEnabled:     false,
		changedCallback:      nil,
		itemAddedCallback:    nil,
		itemModifiedCallback: nil,
		itemDeletedCallback:  nil,
	}, opts)
}

// CallbacksEnabled sets whether executing the callbacks on change is active or not.
func (r *OnChangeMap[K, C, I]) CallbacksEnabled(enabled bool) {
	r.callbacksEnabled = enabled
}

// executeChangedCallback calls the changedCallback if callbackEnabled is true.
func (r *OnChangeMap[K, C, I]) executeChangedCallback() error {
	if !r.callbacksEnabled {
		return nil
	}

	if r.changedCallback != nil {
		if err := r.changedCallback(r.m.Values()); err != nil {
			return fmt.Errorf("failed to execute callback in OnChangeMap: %w", err)
		}
	}

	return nil
}

// executeItemCallback calls the given callback if callbackEnabled is true.
func (r *OnChangeMap[K, C, I]) executeItemCallback(callback func(I) error, item I) error {
	if !r.callbacksEnabled {
		return nil
	}

	if err := r.executeChangedCallback(); err != nil {
		return err
	}

	if callback != nil {
		if err := callback(item); err != nil {
			return fmt.Errorf("failed to execute item callback in OnChangeMap: %w", err)
		}
	}

	return nil
}

// ExecuteChangedCallback calls the changedCallback if callbackEnabled is true.
func (r *OnChangeMap[K, C, I]) ExecuteChangedCallback() error {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return r.executeChangedCallback()
}

// All returns a copy of all items.
func (r *OnChangeMap[K, C, I]) All() map[K]I {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	itemsCopy := make(map[K]I, r.m.Size())
	r.m.ForEach(func(key K, item I) bool {
		//nolint:forcetypeassert // type safety is guaranteed
		itemsCopy[key] = item.Clone().(I)
		return true
	})

	return itemsCopy
}

// Get returns a copy of an item.
func (r *OnChangeMap[K, C, I]) Get(id C) (I, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	item, exists := r.m.Get(id.Key())
	if !exists {
		return *new(I), fmt.Errorf("unable to get item: \"%s\" does not exist in map", id)
	}

	//nolint:forcetypeassert // type safety is guaranteed
	return item.Clone().(I), nil
}

// Add adds an item to the map.
func (r *OnChangeMap[K, C, I]) Add(item I) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.m.Has(item.ID().Key()) {
		return fmt.Errorf("unable to add item: \"%s\" already exists in map", item.ID())
	}

	r.m.Set(item.ID().Key(), item)

	return r.executeItemCallback(r.itemAddedCallback, item)
}

// Modify modifies an item in the map and returns a copy.
func (r *OnChangeMap[K, C, I]) Modify(id C, callback func(item I) bool) (I, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	item, exists := r.m.Get(id.Key())
	if !exists {
		return *new(I), fmt.Errorf("unable to modify item: \"%s\" does not exist in map", id)
	}

	if !callback(item) {
		//nolint:forcetypeassert // type safety is guaranteed
		return item.Clone().(I), nil
	}

	//nolint:forcetypeassert // type safety is guaranteed
	return item.Clone().(I), r.executeItemCallback(r.itemModifiedCallback, item)
}

// Delete removes an item from the map.
func (r *OnChangeMap[K, C, I]) Delete(id C) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	item, exists := r.m.Get(id.Key())
	if !exists {
		return fmt.Errorf("unable to remove item: \"%s\" does not exist in map", id)
	}

	r.m.Delete(id.Key())

	return r.executeItemCallback(r.itemDeletedCallback, item)
}
