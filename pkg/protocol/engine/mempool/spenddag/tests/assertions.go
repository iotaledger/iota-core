package tests

import (
	"github.com/stretchr/testify/require"
)

// Assertions provides a set of assertions for the ConflictDAG.
type Assertions struct {
	f *Framework
}

// Children asserts that the given conflict has the given children.
func (a *Assertions) Children(conflictAlias string, childAliases ...string) {
	childIDs, exists := a.f.Instance.SpendChildren(a.f.SpendID(conflictAlias))
	require.True(a.f.test, exists, "Conflict %s does not exist", conflictAlias)

	require.Equal(a.f.test, len(childAliases), childIDs.Size(), "Conflict %s has wrong number of children", conflictAlias)
	for _, childAlias := range childAliases {
		require.True(a.f.test, childIDs.Has(a.f.SpendID(childAlias)), "Conflict %s does not have child %s", conflictAlias, childAlias)
	}
}

// Parents asserts that the given conflict has the given parents.
func (a *Assertions) Parents(conflictAlias string, parentAliases ...string) {
	parents, exists := a.f.Instance.SpendParents(a.f.SpendID(conflictAlias))
	require.True(a.f.test, exists, "Conflict %s does not exist", conflictAlias)

	require.Equal(a.f.test, len(parentAliases), parents.Size(), "Conflict %s has wrong number of parents", conflictAlias)
	for _, parentAlias := range parentAliases {
		require.True(a.f.test, parents.Has(a.f.SpendID(parentAlias)), "Conflict %s does not have parent %s", conflictAlias, parentAlias)
	}
}

// LikedInstead asserts that the given conflicts return the given LikedInstead conflicts.
func (a *Assertions) LikedInstead(conflictAliases []string, likedInsteadAliases ...string) {
	likedInsteadConflicts := a.f.LikedInstead(conflictAliases...)

	require.Equal(a.f.test, len(likedInsteadAliases), likedInsteadConflicts.Size(), "LikedInstead returns wrong number of conflicts %d instead of %d", likedInsteadConflicts.Size(), len(likedInsteadAliases))
}

// ConflictSetMembers asserts that the given resource has the given conflict set members.
func (a *Assertions) ConflictSetMembers(resourceAlias string, conflictAliases ...string) {
	conflictSetMembers, exists := a.f.Instance.ConflictSetMembers(a.f.ResourceID(resourceAlias))
	require.True(a.f.test, exists, "Resource %s does not exist", resourceAlias)

	require.Equal(a.f.test, len(conflictAliases), conflictSetMembers.Size(), "Resource %s has wrong number of parents", resourceAlias)
	for _, conflictAlias := range conflictAliases {
		require.True(a.f.test, conflictSetMembers.Has(a.f.SpendID(conflictAlias)), "Resource %s does not have parent %s", resourceAlias, conflictAlias)
	}
}

// ConflictSets asserts that the given conflict has the given conflict sets.
func (a *Assertions) ConflictSets(conflictAlias string, resourceAliases ...string) {
	conflictSets, exists := a.f.Instance.ConflictSets(a.f.SpendID(conflictAlias))
	require.True(a.f.test, exists, "Conflict %s does not exist", conflictAlias)

	require.Equal(a.f.test, len(resourceAliases), conflictSets.Size(), "Conflict %s has wrong number of conflict sets", conflictAlias)
	for _, resourceAlias := range resourceAliases {
		require.True(a.f.test, conflictSets.Has(a.f.ResourceID(resourceAlias)), "Conflict %s does not have conflict set %s", conflictAlias, resourceAlias)
	}
}

// Pending asserts that the given conflicts are pending.
func (a *Assertions) Pending(aliases ...string) {
	for _, alias := range aliases {
		require.True(a.f.test, a.f.Instance.AcceptanceState(a.f.SpendIDs(alias)).IsPending(), "Conflict %s is not pending", alias)
	}
}

// Accepted asserts that the given conflicts are accepted.
func (a *Assertions) Accepted(aliases ...string) {
	for _, alias := range aliases {
		require.True(a.f.test, a.f.Instance.AcceptanceState(a.f.SpendIDs(alias)).IsAccepted(), "Conflict %s is not accepted", alias)
	}
}

// Rejected asserts that the given conflicts are rejected.
func (a *Assertions) Rejected(aliases ...string) {
	for _, alias := range aliases {
		require.True(a.f.test, a.f.Instance.AcceptanceState(a.f.SpendIDs(alias)).IsRejected(), "Conflict %s is not rejected", alias)
	}
}

// ValidatorWeight asserts that the given conflict has the given validator weight.
func (a *Assertions) ValidatorWeight(conflictAlias string, weight int64) {
	require.Equal(a.f.test, weight, a.f.Instance.SpendWeight(a.f.SpendID(conflictAlias)), "ValidatorWeight is %s instead of % for conflict %s", a.f.Instance.SpendWeight(a.f.SpendID(conflictAlias)), weight, conflictAlias)
}
