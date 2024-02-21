//go:build dockertests

package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/stretchr/testify/require"
)

func Test_EventAPI_BasicTaggedDataBlocks(t *testing.T) {
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

	// get event API client ready
	clt := d.Node("V1").Client
	ctx, cancel := context.WithCancel(context.Background())
	eventClt, err := clt.EventAPI(ctx)
	require.NoError(t, err)
	err = eventClt.Connect(ctx)
	require.NoError(t, err)

	// create an account to issue blocks
	account := d.CreateAccount()

	// prepare data blocks to send
	expectedBlocks := make(map[string]*iotago.Block, 0)
	for i := 0; i < 10; i++ {
		blk := d.CreateTaggedDataBlock(account, []byte("tag"))
		expectedBlocks[blk.MustID().ToHex()] = blk
	}
	finish := make(chan struct{})
	totalTopics := 6

	// register to `blocks` topic
	d.AssertBlocks(ctx, eventClt, expectedBlocks, finish)

	// register to `blocks/basic` topic
	d.AssertBasicBlocks(ctx, eventClt, expectedBlocks, finish)

	// register to `blocks/basic/tagged-data` topic
	d.AssertTaggedDataBlocks(ctx, eventClt, expectedBlocks, finish)

	// register to `blocks/basic/transaction/tagged-data/{tag}` topic
	d.AssertTaggedDataBlocksByTag(ctx, eventClt, expectedBlocks, []byte("tag"), finish)

	// register to `block-metadata/accepted` topic
	d.AssertBlockMetadataAcceptedBlocks(ctx, eventClt, expectedBlocks, finish)

	// register to `block-metadata/confirmed` topic
	d.AssertBlockMetadataConfirmedBlocks(ctx, eventClt, expectedBlocks, finish)

	// wait until all topics starts listening
	err = AwaitEventAPITopics(t, d.optsWaitFor, cancel, finish, totalTopics)
	require.NoError(t, err)

	// issue blocks
	go func() {
		for _, blk := range expectedBlocks {
			fmt.Println("submitting a block")
			d.SubmitBlock(context.Background(), blk)
		}
	}()

	// wait until all topics receives all expected objects
	err = AwaitEventAPITopics(t, d.optsWaitFor, cancel, finish, totalTopics)
	require.NoError(t, err)
}
