package tests

import (
	"github.com/stretchr/testify/require"
)

// Assertions provides a set of assertions for the SpendDAG.
type Assertions struct {
	f *Framework
}

// Children asserts that the given spender has the given children.
func (a *Assertions) Children(spendAlias string, childAliases ...string) {
	childIDs, exists := a.f.Instance.SpenderChildren(a.f.SpenderID(spendAlias))
	require.True(a.f.test, exists, "Spender %s does not exist", spendAlias)

	require.Equal(a.f.test, len(childAliases), childIDs.Size(), "Spend %s has wrong number of children", spendAlias)
	for _, childAlias := range childAliases {
		require.True(a.f.test, childIDs.Has(a.f.SpenderID(childAlias)), "Spend %s does not have child %s", spendAlias, childAlias)
	}
}

// Parents asserts that the given spend has the given parents.
func (a *Assertions) Parents(spendAlias string, parentAliases ...string) {
	parents, exists := a.f.Instance.SpenderParents(a.f.SpenderID(spendAlias))
	require.True(a.f.test, exists, "Spend %s does not exist", spendAlias)

	require.Equal(a.f.test, len(parentAliases), parents.Size(), "Spend %s has wrong number of parents", spendAlias)
	for _, parentAlias := range parentAliases {
		require.True(a.f.test, parents.Has(a.f.SpenderID(parentAlias)), "Spend %s does not have parent %s", spendAlias, parentAlias)
	}
}

// LikedInstead asserts that the given spenders return the given LikedInstead spenders.
func (a *Assertions) LikedInstead(spendAliases []string, likedInsteadAliases ...string) {
	likedInsteadSpenders := a.f.LikedInstead(spendAliases...)

	require.Equal(a.f.test, len(likedInsteadAliases), likedInsteadSpenders.Size(), "LikedInstead returns wrong number of spenders %d instead of %d", likedInsteadSpenders.Size(), len(likedInsteadAliases))
}

// SpendSetMembers asserts that the given resource has the given spend set members.
func (a *Assertions) SpendSetMembers(resourceAlias string, spendAliases ...string) {
	spendSetMembers, exists := a.f.Instance.SpendSetMembers(a.f.ResourceID(resourceAlias))
	require.True(a.f.test, exists, "Resource %s does not exist", resourceAlias)

	require.Equal(a.f.test, len(spendAliases), spendSetMembers.Size(), "Resource %s has wrong number of parents", resourceAlias)
	for _, spendAlias := range spendAliases {
		require.True(a.f.test, spendSetMembers.Has(a.f.SpenderID(spendAlias)), "Resource %s does not have parent %s", resourceAlias, spendAlias)
	}
}

// SpendSets asserts that the given spender has the given spend sets.
func (a *Assertions) SpendSets(spenderAlias string, resourceAliases ...string) {
	spendSets, exists := a.f.Instance.SpendSets(a.f.SpenderID(spenderAlias))
	require.True(a.f.test, exists, "Spender %s does not exist", spenderAlias)

	require.Equal(a.f.test, len(resourceAliases), spendSets.Size(), "Spender %s has wrong number of conflict sets", spenderAlias)
	for _, resourceAlias := range resourceAliases {
		require.True(a.f.test, spendSets.Has(a.f.ResourceID(resourceAlias)), "Spender %s does not have conflict set %s", spenderAlias, resourceAlias)
	}
}

// Pending asserts that the given spenders are pending.
func (a *Assertions) Pending(aliases ...string) {
	for _, alias := range aliases {
		require.True(a.f.test, a.f.Instance.AcceptanceState(a.f.SpenderIDs(alias)).IsPending(), "Spender %s is not pending", alias)
	}
}

// Accepted asserts that the given spenders are accepted.
func (a *Assertions) Accepted(aliases ...string) {
	for _, alias := range aliases {
		require.True(a.f.test, a.f.Instance.AcceptanceState(a.f.SpenderIDs(alias)).IsAccepted(), "Spender %s is not accepted", alias)
	}
}

// Rejected asserts that the given spenders are rejected.
func (a *Assertions) Rejected(aliases ...string) {
	for _, alias := range aliases {
		require.True(a.f.test, a.f.Instance.AcceptanceState(a.f.SpenderIDs(alias)).IsRejected(), "Spender %s is not rejected", alias)
	}
}

// ValidatorWeight asserts that the given spend has the given validator weight.
func (a *Assertions) ValidatorWeight(spendAlias string, weight int64) {
	require.Equal(a.f.test, weight, a.f.Instance.SpenderWeight(a.f.SpenderID(spendAlias)), "ValidatorWeight is %s instead of % for spender %s", a.f.Instance.SpenderWeight(a.f.SpenderID(spendAlias)), weight, spendAlias)
}
