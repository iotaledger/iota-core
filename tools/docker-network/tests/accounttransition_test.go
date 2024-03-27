//go:build dockertests

package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	iotago "github.com/iotaledger/iota.go/v4"
)

// Test_AccountTransitions follows the account state transition flow described in:
// 1. Create account1.
// 2. Create account2.
// 3. account1 requests faucet funds then allots 1000 mana to account2.
// 4. account2 requests faucet funds then creates native tokens.
func Test_AccountTransitions(t *testing.T) {
	d := NewDockerTestFramework(t,
		WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 4),
			iotago.WithLivenessOptions(10, 10, 2, 4, 8),
			iotago.WithRewardsOptions(8, 10, 2, 384),
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

	d.WaitUntilNetworkReady()

	// create account1
	fmt.Println("Creating account1")
	wallet1, _ := d.CreateAccount()

	// create account2
	fmt.Println("Creating account2")
	wallet2, _ := d.CreateAccount()

	// allot 1000 mana from account1 to account2
	fmt.Println("Allotting mana from account1 to account2")
	d.AllotManaTo(wallet1, wallet2.BlockIssuer.AccountData, 1000)

	// create native token
	fmt.Println("Creating native token")
	d.CreateNativeToken(wallet1, 5_000_000, 10_000_000_000)
}
