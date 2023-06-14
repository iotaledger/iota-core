package evilwallet

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_EvilWalletFaucet(t *testing.T) {
	ew := NewEvilWallet()

	initwallet, err := ew.RequestFundsFromFaucet(WithOutputAlias("a"))
	require.NoError(t, err, "request faucet funds failed")

	fmt.Println(initwallet.UnspentOutputs())
}
