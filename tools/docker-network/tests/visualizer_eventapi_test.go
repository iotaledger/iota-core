//go:build dockertests

package tests

import (
	"context"
	"testing"
	"time"

	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/stretchr/testify/require"
)

var eventAPIVisualizerTests = map[string]func(t *testing.T, e *EventAPIDockerTestFramework){
	"Test_BlockMetadataMatched": test_BlockMetadata,
}

func Test_VisualizerTopics(t *testing.T) {
	d := NewDockerTestFramework(t,
		WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(5, time.Now().Unix(), 10, 4),
			iotago.WithLivenessOptions(10, 10, 2, 4, 8),
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

	e := NewEventAPIDockerTestFramework(t, d)

	for name, test := range eventAPIVisualizerTests {
		t.Run(name, func(t *testing.T) {
			test(t, e)
		})
	}
}

func test_BlockMetadata(t *testing.T, e *EventAPIDockerTestFramework) {
	// get event API client ready
	ctx, cancel := context.WithCancel(context.Background())
	eventClt := e.ConnectEventAPIClient(ctx)
	defer eventClt.Close()

	{
		account := e.dockerFramework.CreateAccount()

		assertions := []func(){
			func() { e.AssertBlockMetadataStateAcceptedBlocks(ctx, eventClt) },
			func() { e.AssertBlockMetadataStateConfirmedBlocks(ctx, eventClt) },
		}

		totalTopics := len(assertions)
		for _, assertion := range assertions {
			assertion()
		}

		// wait until all topics starts listening
		err := e.AwaitEventAPITopics(t, cancel, totalTopics)
		require.NoError(t, err)

		// issue blocks
		e.SubmitDataBlockStream(account, 5*time.Minute)

		// time's up, cancel the context
		time.Sleep(2 * time.Minute)
		cancel()
	}
}
