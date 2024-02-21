//go:build dockertests

package tests

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/nodeclient"
)

func (d *DockerTestFramework) AssertBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, finishChan chan struct{}) {
	blksChan, subInfo := eventClt.Blocks()
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertBlocksTopics(ctx, blksChan, expectedBlockIDs, finishChan)
	}()
}

func (d *DockerTestFramework) AssertBasicBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, finishChan chan struct{}) {
	blksChan, subInfo := eventClt.BlocksBasic()
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertBlocksTopics(ctx, blksChan, expectedBlockIDs, finishChan)
	}()
}

func (d *DockerTestFramework) AssertValidationBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, finishChan chan struct{}) {
	blksChan, subInfo := eventClt.BlocksValidation()
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertBlocksTopics(ctx, blksChan, expectedBlockIDs, finishChan)
	}()
}

func (d *DockerTestFramework) AssertTaggedDataBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, finishChan chan struct{}) {
	blksChan, subInfo := eventClt.BlocksBasicWithTaggedData()
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertBlocksTopics(ctx, blksChan, expectedBlockIDs, finishChan)
	}()
}

func (d *DockerTestFramework) AssertTaggedDataBlocksByTag(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, tag []byte, finishChan chan struct{}) {
	blksChan, subInfo := eventClt.BlocksBasicWithTaggedDataByTag(tag)
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertBlocksTopics(ctx, blksChan, expectedBlockIDs, finishChan)
	}()
}

func (d *DockerTestFramework) AssertTransactionBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, finishChan chan struct{}) {
	blksChan, subInfo := eventClt.BlocksBasicWithTransactions()
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertBlocksTopics(ctx, blksChan, expectedBlockIDs, finishChan)
	}()
}

func (d *DockerTestFramework) AssertTransactionTaggedDataBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, finishChan chan struct{}) {
	blksChan, subInfo := eventClt.BlocksBasicWithTransactionsWithTaggedData()
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertBlocksTopics(ctx, blksChan, expectedBlockIDs, finishChan)
	}()
}

func (d *DockerTestFramework) AssertTransactionBlocksByTag(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, tag []byte, finishChan chan struct{}) {
	blksChan, subInfo := eventClt.BlocksBasicWithTransactionsWithTaggedDataByTag(tag)
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertBlocksTopics(ctx, blksChan, expectedBlockIDs, finishChan)
	}()
}

func (d *DockerTestFramework) AssertBlockMetadataAcceptedBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, finishChan chan struct{}) {
	acceptedChan, subInfo := eventClt.BlockMetadataAcceptedBlocks()
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertBlockMetadataTopics(ctx, acceptedChan, expectedBlockIDs, finishChan)
	}()
}

func (d *DockerTestFramework) AssertBlockMetadataConfirmedBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, finishChan chan struct{}) {
	acceptedChan, subInfo := eventClt.BlockMetadataConfirmedBlocks()
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertBlockMetadataTopics(ctx, acceptedChan, expectedBlockIDs, finishChan)
	}()
}

func (d *DockerTestFramework) assertBlocksTopics(ctx context.Context, receivedChan <-chan *iotago.Block, expectedBlocks map[string]*iotago.Block, finishChan chan struct{}) {
	expectedBlockIDsSlice := lo.Keys(expectedBlocks)
	blkIDs := make([]string, 0)

	// in order to inform that the channel is listened
	finishChan <- struct{}{}

	for {
		select {
		case blk := <-receivedChan:
			_, ok := expectedBlocks[blk.MustID().ToHex()]
			if ok {
				blkIDs = append(blkIDs, blk.MustID().ToHex())

				if len(blkIDs) == len(expectedBlocks) {
					require.ElementsMatch(d.Testing, expectedBlockIDsSlice, blkIDs)
					finishChan <- struct{}{}
					return
				}
			}
		case <-ctx.Done():
			fmt.Println("Received blocks:", blkIDs)
			return
		}
	}
}

func (d *DockerTestFramework) assertBlockMetadataTopics(ctx context.Context, receivedChan <-chan *api.BlockMetadataResponse, expectedBlocks map[string]*iotago.Block, finishChan chan struct{}) {
	expectedBlockIDsSlice := lo.Keys(expectedBlocks)
	blkIDs := make([]string, 0)

	// in order to inform that the channel is listened
	finishChan <- struct{}{}

	for {
		select {
		case blk := <-receivedChan:
			id := blk.BlockID.ToHex()
			_, ok := expectedBlocks[id]
			if ok {
				blkIDs = append(blkIDs, id)

				if len(blkIDs) == len(expectedBlocks) {
					require.ElementsMatch(d.Testing, expectedBlockIDsSlice, blkIDs)
					finishChan <- struct{}{}
					return
				}
			}
		case <-ctx.Done():
			fmt.Println("Received blocks:", blkIDs)
			return
		}
	}
}
