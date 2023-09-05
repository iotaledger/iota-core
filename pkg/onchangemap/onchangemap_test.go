package onchangemap

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type comparableInteger int

func (i comparableInteger) Key() int {
	return int(i)
}

func (i comparableInteger) Equals(other int) bool {
	return int(i) == other
}

func (i comparableInteger) String() string {
	return fmt.Sprintf("my test number %d", i)
}

type testItem struct {
	id    comparableInteger
	value string
}

func newTestItem(id comparableInteger, value string) *testItem {
	return &testItem{
		id:    id,
		value: value,
	}
}

func (i *testItem) ID() comparableInteger {
	return i.id
}

func (i *testItem) Clone() Item[int, comparableInteger] {
	return &testItem{
		id:    i.id,
		value: i.value,
	}
}

func TestOnChangeMap(t *testing.T) {
	var storedItems []*testItem
	var itemAdded *testItem
	var itemModified *testItem
	var itemDeleted *testItem

	onChangeMap := NewOnChangeMap(
		WithChangedCallback[int, comparableInteger](func(items []*testItem) error {
			storedItems = items
			return nil
		}),
		WithItemAddedCallback[int, comparableInteger](func(item *testItem) error {
			itemAdded = item
			return nil
		}),
		WithItemModifiedCallback[int, comparableInteger](func(item *testItem) error {
			itemModified = item
			return nil
		}),
		WithItemDeletedCallback[int, comparableInteger](func(item *testItem) error {
			itemDeleted = item
			return nil
		}),
	)

	require.NotNil(t, onChangeMap)

	item1 := newTestItem(1, "one")
	item2 := newTestItem(2, "two")
	item3 := newTestItem(3, "three")

	require.Nil(t, itemAdded)
	require.Nil(t, itemModified)
	require.Nil(t, itemDeleted)

	// add and get an item
	err := onChangeMap.Add(item1)
	require.NoError(t, err)

	item1Copy, err := onChangeMap.Get(1)
	require.NoError(t, err)

	// get non-existing item
	_, err = onChangeMap.Get(2)
	require.Error(t, err)

	// get all items
	items := onChangeMap.All()

	// compare items
	require.Equal(t, item1.value, item1Copy.value)
	require.Equal(t, len(items), 1)

	// check store on change
	require.Equal(t, len(storedItems), 0)
	err = onChangeMap.ExecuteChangedCallback()
	require.NoError(t, err)
	require.Equal(t, len(storedItems), 0)

	// enable store on changed
	onChangeMap.CallbacksEnabled(true)
	err = onChangeMap.ExecuteChangedCallback()
	require.NoError(t, err)
	require.Equal(t, len(storedItems), 1)

	// add duplicate
	err = onChangeMap.Add(item1)
	require.Error(t, err)
	require.Nil(t, itemAdded)
	require.Nil(t, itemModified)
	require.Nil(t, itemDeleted)
	itemAdded = nil

	// add second item
	err = onChangeMap.Add(item2)
	require.NoError(t, err)
	require.Equal(t, len(storedItems), 2)
	require.NotNil(t, itemAdded)
	require.Nil(t, itemModified)
	require.Nil(t, itemDeleted)
	itemAdded = nil

	// modify non existing item
	_, err = onChangeMap.Modify(3, nil)
	require.Error(t, err)
	require.Nil(t, itemAdded)
	require.Nil(t, itemModified)
	require.Nil(t, itemDeleted)

	// modify existing item
	item2Copy, err := onChangeMap.Modify(2, func(item *testItem) bool {
		item.value = item3.value
		return true
	})
	require.NoError(t, err)
	require.Equal(t, len(storedItems), 2)
	require.Equal(t, item2Copy.value, item3.value)
	require.Nil(t, itemAdded)
	require.NotNil(t, itemModified)
	require.Nil(t, itemDeleted)
	itemModified = nil

	// get modified item
	item2Copy, err = onChangeMap.Get(2)
	require.NoError(t, err)
	require.Equal(t, item2Copy.value, item3.value)

	// delete item
	err = onChangeMap.Delete(2)
	require.NoError(t, err)
	require.Equal(t, len(storedItems), 1)
	require.Nil(t, itemAdded)
	require.Nil(t, itemModified)
	require.NotNil(t, itemDeleted)
	itemDeleted = nil

	// delete non-existing item
	err = onChangeMap.Delete(2)
	require.Error(t, err)
	require.Nil(t, itemAdded)
	require.Nil(t, itemModified)
	require.Nil(t, itemDeleted)
}
