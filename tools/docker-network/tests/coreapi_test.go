package tests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient"
	"github.com/stretchr/testify/require"
)

func Test_ValidatorsAPI(t *testing.T) {
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
	hrp := d.wallet.DefaultClient().CommittedAPI().ProtocolParameters().Bech32HRP()

	// Create registered validators
	var wg sync.WaitGroup
	clt := d.wallet.DefaultClient()
	status := d.NodeStatus("V1")
	currentEpoch := clt.CommittedAPI().TimeProvider().EpochFromSlot(status.LatestAcceptedBlockSlot)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			account := d.CreateAccount(WithStakingFeature(100, 1, 0))

			// issue candidacy payload in the next epoch (currentEpoch + 1), in order to issue it before epochNearingThreshold
			d.AwaitCommitment(clt.CommittedAPI().TimeProvider().EpochEnd(currentEpoch))
			blkID := d.IssueCandidacyPayloadFromAccount(account.ID)
			fmt.Println("Candidacy payload:", blkID.ToHex(), blkID.Slot())
			d.AwaitCommitment(blkID.Slot())
		}()
	}
	wg.Wait()

	expectedValidators := d.AccountsFromNodes(d.Nodes()...)
	for _, v := range d.wallet.Accounts() {
		expectedValidators = append(expectedValidators, v.Address.Bech32(hrp))
	}
	// get all validators of currentEpoch+1 with pageSize 10
	actualValidators := getAllValidatorsOnEpoch(t, clt, 0, 10)
	require.ElementsMatch(t, expectedValidators, actualValidators)

	// wait until currentEpoch+3 and check the results again
	targetSlot := clt.CommittedAPI().TimeProvider().EpochEnd(currentEpoch + 2)
	d.AwaitCommitment(targetSlot)
	actualValidators = getAllValidatorsOnEpoch(t, clt, currentEpoch+1, 10)
	require.ElementsMatch(t, expectedValidators, actualValidators)
}

func getAllValidatorsOnEpoch(t *testing.T, clt *nodeclient.Client, epoch iotago.EpochIndex, pageSize uint64) []string {
	actualValidators := make([]string, 0)
	cursor := ""
	if epoch != 0 {
		cursor = fmt.Sprintf("%d,%d", epoch, 0)
	}

	for {
		resp, err := clt.Validators(context.Background(), pageSize, cursor)
		require.NoError(t, err)

		for _, v := range resp.Validators {
			actualValidators = append(actualValidators, v.AddressBech32)
		}

		cursor = resp.Cursor
		if cursor == "" {
			break
		}
	}

	return actualValidators
}
