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

func (d *DockerTestFramework) AssertOutput(ctx context.Context, eventClt *nodeclient.EventAPIClient, outputId iotago.OutputID, finishChan chan struct{}) {
	outputMetadataChan, subInfo := eventClt.OutputWithMetadataByOutputID(outputId)
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertOutputMetadataTopics(ctx, outputMetadataChan, func(resp *api.OutputWithMetadataResponse) bool {
			if outputId.Compare(resp.Metadata.OutputID) == 0 {
				return true
			}
			return false
		}, finishChan)
	}()
}

func (d *DockerTestFramework) AssertDelegationOutput(ctx context.Context, eventClt *nodeclient.EventAPIClient, delegationId iotago.DelegationID, finishChan chan struct{}) {
	outputMetadataChan, subInfo := eventClt.OutputsWithMetadataByDelegationID(delegationId)
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertOutputMetadataTopics(ctx, outputMetadataChan, func(resp *api.OutputWithMetadataResponse) bool {
			if resp.Output.Type() == iotago.OutputDelegation {
				o := resp.Output.(*iotago.DelegationOutput)
				return delegationId.Matches(o.DelegationID)
			}
			return false
		}, finishChan)
	}()
}

func (d *DockerTestFramework) AssertFoundryOutput(ctx context.Context, eventClt *nodeclient.EventAPIClient, foundryId iotago.FoundryID, finishChan chan struct{}) {
	outputMetadataChan, subInfo := eventClt.OutputsWithMetadataByFoundryID(foundryId)
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertOutputMetadataTopics(ctx, outputMetadataChan, func(resp *api.OutputWithMetadataResponse) bool {
			if resp.Output.Type() == iotago.OutputFoundry {
				o := resp.Output.(*iotago.FoundryOutput)
				return foundryId.Matches(o.MustFoundryID())
			}
			return false
		}, finishChan)
	}()
}

func (d *DockerTestFramework) AssertAccountOutput(ctx context.Context, eventClt *nodeclient.EventAPIClient, accountId iotago.AccountID, finishChan chan struct{}) {
	outputMetadataChan, subInfo := eventClt.OutputsWithMetadataByAccountID(accountId)
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertOutputMetadataTopics(ctx, outputMetadataChan, func(resp *api.OutputWithMetadataResponse) bool {
			if resp.Output.Type() == iotago.OutputAccount {
				o := resp.Output.(*iotago.AccountOutput)
				return accountId.Matches(o.AccountID)
			}
			return false
		}, finishChan)
	}()
}

func (d *DockerTestFramework) AssertAnchorOutput(ctx context.Context, eventClt *nodeclient.EventAPIClient, anchorId iotago.AnchorID, finishChan chan struct{}) {
	outputMetadataChan, subInfo := eventClt.OutputsWithMetadataByAnchorID(anchorId)
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertOutputMetadataTopics(ctx, outputMetadataChan, func(resp *api.OutputWithMetadataResponse) bool {
			if resp.Output.Type() == iotago.OutputAnchor {
				o := resp.Output.(*iotago.AnchorOutput)
				return anchorId.Matches(o.AnchorID)
			}
			return false
		}, finishChan)
	}()
}

func (d *DockerTestFramework) AssertNFTOutput(ctx context.Context, eventClt *nodeclient.EventAPIClient, nftId iotago.NFTID, finishChan chan struct{}) {
	outputMetadataChan, subInfo := eventClt.OutputsWithMetadataByNFTID(nftId)
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertOutputMetadataTopics(ctx, outputMetadataChan, func(resp *api.OutputWithMetadataResponse) bool {
			if resp.Output.Type() == iotago.OutputNFT {
				o := resp.Output.(*iotago.NFTOutput)
				return nftId.Matches(o.NFTID)
			}
			return false
		}, finishChan)
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

func (d *DockerTestFramework) assertOutputMetadataTopics(ctx context.Context, receivedChan <-chan *api.OutputWithMetadataResponse, matchFunc func(*api.OutputWithMetadataResponse) bool, finishChan chan struct{}) {
	// in order to inform that the channel is listened
	finishChan <- struct{}{}

	for {
		select {
		case outputMetadata := <-receivedChan:
			if matchFunc(outputMetadata) {
				finishChan <- struct{}{}
				return
			}
		case <-ctx.Done():
			fmt.Println("Output Metadata related topics does not get expected contents")
			return
		}
	}
}
