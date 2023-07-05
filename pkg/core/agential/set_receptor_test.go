package agential

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds/advancedset"
)

func TestSet(t *testing.T) {
	source1 := NewSetReceptor[int]()
	source2 := NewSetReceptor[int]()

	inheritedSet := NewSetReceptor[int]()
	inheritedSet.InheritFrom(source1, source2)

	source1.Add(advancedset.New(1, 2, 4))
	source2.Add(advancedset.New(7, 9))

	require.True(t, inheritedSet.Has(1))
	require.True(t, inheritedSet.Has(2))
	require.True(t, inheritedSet.Has(4))
	require.True(t, inheritedSet.Has(7))
	require.True(t, inheritedSet.Has(9))

	inheritedSet1 := NewSetReceptor[int]()
	inheritedSet1.InheritFrom(source1, source2)

	require.True(t, inheritedSet1.Has(1))
	require.True(t, inheritedSet1.Has(2))
	require.True(t, inheritedSet1.Has(4))
	require.True(t, inheritedSet1.Has(7))
	require.True(t, inheritedSet1.Has(9))
}
