//go:build dockertests

package tests

import (
	"context"
	"testing"
	"time"

	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/stretchr/testify/require"
)

func Test_AccountTransitions(t *testing.T) {
	d := NewDockerTestFramework(t,
		WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 4),
			iotago.WithLivenessOptions(10, 10, 2, 4, 8),
			iotago.WithRewardsOptions(8, 8, 10, 2, 1, 384),
			iotago.WithTargetCommitteeSize(4),
		))
	defer d.Stop()

	d.AddValidatorNode("V1", "docker-network-inx-validator-1-1", "http://localhost:8050", "rms1pzg8cqhfxqhq7pt37y8cs4v5u4kcc48lquy2k73ehsdhf5ukhya3y5rx2w6")
	d.AddValidatorNode("V2", "docker-network-inx-validator-2-1", "http://localhost:8060", "rms1pqm4xk8e9ny5w5rxjkvtp249tfhlwvcshyr3pc0665jvp7g3hc875k538hl")
	d.AddValidatorNode("V3", "docker-network-inx-validator-3-1", "http://localhost:8070", "rms1pp4wuuz0y42caz48vv876qfpmffswsvg40zz8v79sy8cp0jfxm4kunflcgt")
	d.AddValidatorNode("V4", "docker-network-inx-validator-4-1", "http://localhost:8040", "rms1pr8cxs3dzu9xh4cduff4dd4cxdthpjkpwmz2244f75m0urslrsvtsshrrjw")
	d.AddNode("node5", "docker-network-node-5-1", "http://localhost:8090")

	err := d.Run()
	require.NoError(t, err)

	err = d.WaitUntilSync()
	require.NoError(t, err)

	// create account1
	account1 := d.CreateAccount()
	// create account2
	account2 := d.CreateAccount()

	// request faucet funds
	fundsAddr, _ := d.getAddress(iotago.AddressEd25519)
	d.RequestFaucetFunds(context.TODO(), fundsAddr)

	// allot 1000 mana from account1 to account2
	d.AllotManaTo(account1, account2, 1000)

	// create native token
	account2 = d.CreateNativeToken(account2)
}
