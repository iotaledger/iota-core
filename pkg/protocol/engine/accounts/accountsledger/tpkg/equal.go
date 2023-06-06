package tpkg

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
)

func EqualAccountData(t *testing.T, expected *accounts.AccountData, actual *accounts.AccountData) {
	require.Equal(t, expected.ID(), actual.ID())
	require.Equal(t, expected.BlockIssuanceCredits(), actual.BlockIssuanceCredits())
	require.Equal(t, expected.PubKeys().Size(), actual.PubKeys().Size())
	actual.PubKeys().Equal(expected.PubKeys())
}
