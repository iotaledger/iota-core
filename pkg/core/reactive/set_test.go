package reactive

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSet(t *testing.T) {
	source1 := NewSet[int]()
	source2 := NewSet[int]()

	inheritedSet := NewSet[int]()
	inheritedSet.InheritFrom(source1, source2)

	source1.Add(setImpl.New(1, 2, 4))
	source2.Add(setImpl.New(7, 9))

	require.True(t, inheritedSet.Get().Has(1))
	require.True(t, inheritedSet.Get().Has(2))
	require.True(t, inheritedSet.Get().Has(4))
	require.True(t, inheritedSet.Get().Has(7))
	require.True(t, inheritedSet.Get().Has(9))

	inheritedSet1 := NewSet[int]()
	inheritedSet1.InheritFrom(source1, source2)

	require.True(t, inheritedSet1.Get().Has(1))
	require.True(t, inheritedSet1.Get().Has(2))
	require.True(t, inheritedSet1.Get().Has(4))
	require.True(t, inheritedSet1.Get().Has(7))
	require.True(t, inheritedSet1.Get().Has(9))
}
