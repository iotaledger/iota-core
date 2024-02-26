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

func (d *DockerTestFramework) AssertLatestCommitments(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedSlots []iotago.SlotIndex, finishChan chan struct{}) {
	commitmentChan, subInfo := eventClt.CommitmentsLatest()
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertCommitmentsTopics(ctx, commitmentChan, expectedSlots, finishChan)
		fmt.Println("AssertLatestCommitments finished")
	}()
}

func (d *DockerTestFramework) AssertFinalizedCommitments(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedSlots []iotago.SlotIndex, finishChan chan struct{}) {
	commitmentChan, subInfo := eventClt.CommitmentsFinalized()
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertCommitmentsTopics(ctx, commitmentChan, expectedSlots, finishChan)
		fmt.Println("AssertFinalizedCommitments finished")
	}()
}

func (d *DockerTestFramework) AssertBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, finishChan chan struct{}) {
	blksChan, subInfo := eventClt.Blocks()
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertBlocksTopics(ctx, blksChan, expectedBlockIDs, finishChan)
		fmt.Println("AssertBlocks finished")
	}()
}

func (d *DockerTestFramework) AssertBasicBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, finishChan chan struct{}) {
	blksChan, subInfo := eventClt.BlocksBasic()
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertBlocksTopics(ctx, blksChan, expectedBlockIDs, finishChan)
		fmt.Println("AssertBasicBlocks finished")
	}()
}

// AssertValidationBlocks listens to the validation blocks and checks if the block is a validation block and the issuer is in the validators list. The check passes after 10 blocks.
func (d *DockerTestFramework) AssertValidationBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, hrp iotago.NetworkPrefix, validators map[string]struct{}, finishChan chan struct{}) {
	blksChan, subInfo := eventClt.BlocksValidation()
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer fmt.Println("AssertValidationBlocks finished")
		defer subInfo.Close()
		blkIDs := make([]string, 0)
		counter := 0

		// in order to inform that the channel is listened
		finishChan <- struct{}{}

		for {
			select {
			case blk := <-blksChan:
				require.Equal(d.Testing, iotago.BlockBodyTypeValidation, blk.Body.Type())
				_, ok := validators[blk.Header.IssuerID.ToAddress().Bech32(hrp)]
				require.True(d.Testing, ok)

				// The check passes after 10 blocks
				counter++
				if counter == 10 {
					finishChan <- struct{}{}
					return
				}
			case <-ctx.Done():
				fmt.Println("Received blocks:", blkIDs)
				return
			}
		}

	}()
}

func (d *DockerTestFramework) AssertTaggedDataBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, finishChan chan struct{}) {
	blksChan, subInfo := eventClt.BlocksBasicWithTaggedData()
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertBlocksTopics(ctx, blksChan, expectedBlockIDs, finishChan)
		fmt.Println("AssertTaggedDataBlocks finished")
	}()
}

func (d *DockerTestFramework) AssertTaggedDataBlocksByTag(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, tag []byte, finishChan chan struct{}) {
	blksChan, subInfo := eventClt.BlocksBasicWithTaggedDataByTag(tag)
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertBlocksTopics(ctx, blksChan, expectedBlockIDs, finishChan)
		fmt.Println("AssertTaggedDataBlocksByTag finished")
	}()
}

func (d *DockerTestFramework) AssertTransactionBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, finishChan chan struct{}) {
	blksChan, subInfo := eventClt.BlocksBasicWithTransactions()
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertBlocksTopics(ctx, blksChan, expectedBlockIDs, finishChan)
		fmt.Println("AssertTransactionBlocks finished")
	}()
}

func (d *DockerTestFramework) AssertTransactionTaggedDataBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, finishChan chan struct{}) {
	blksChan, subInfo := eventClt.BlocksBasicWithTransactionsWithTaggedData()
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertBlocksTopics(ctx, blksChan, expectedBlockIDs, finishChan)
		fmt.Println("AssertTransactionTaggedDataBlocks finished")
	}()
}

func (d *DockerTestFramework) AssertTransactionBlocksByTag(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, tag []byte, finishChan chan struct{}) {
	blksChan, subInfo := eventClt.BlocksBasicWithTransactionsWithTaggedDataByTag(tag)
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertBlocksTopics(ctx, blksChan, expectedBlockIDs, finishChan)
		fmt.Println("AssertTransactionBlocksByTag finished")
	}()
}

func (d *DockerTestFramework) AssertTransactionMetadataIncludedBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, txID iotago.TransactionID, finishChan chan struct{}) {
	acceptedChan, subInfo := eventClt.BlockMetadataTransactionIncludedBlocksByTransactionID(txID)
	require.Nil(d.Testing, subInfo.Error())
	counter := 0

	go func() {
		defer fmt.Println("AssertTransactionMetadataIncludedBlocks finished")
		defer subInfo.Close()
		// in order to inform that the channel is listened
		finishChan <- struct{}{}

		for {
			select {
			case metadata := <-acceptedChan:
				if txID.Compare(metadata.TransactionMetadata.TransactionID) == 0 {
					counter++
					fmt.Println(metadata.TransactionMetadata.TransactionState)
					// we should get 2 times of the same transaction, one for accepted and one for confirmed
					if counter == 2 {
						finishChan <- struct{}{}
						return
					}
				}

			case <-ctx.Done():

				return
			}
		}

	}()
}

func (d *DockerTestFramework) AssertTransactionMetadataByTransactionID(ctx context.Context, eventClt *nodeclient.EventAPIClient, txID iotago.TransactionID, finishChan chan struct{}) {
	acceptedChan, subInfo := eventClt.TransactionMetadataByTransactionID(txID)
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer fmt.Println("AssertTransactionMetadataByTransactionID finished")
		defer subInfo.Close()
		// in order to inform that the channel is listened
		finishChan <- struct{}{}

		for {
			select {
			case metadata := <-acceptedChan:
				if txID.Compare(metadata.TransactionID) == 0 {
					fmt.Println(metadata.TransactionState)
					finishChan <- struct{}{}
					return
				}

			case <-ctx.Done():
				return
			}
		}
	}()
}

func (d *DockerTestFramework) AssertBlockMetadataAcceptedBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, finishChan chan struct{}) {
	acceptedChan, subInfo := eventClt.BlockMetadataAcceptedBlocks()
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertBlockMetadataTopics(ctx, acceptedChan, expectedBlockIDs, finishChan)
		fmt.Println("AssertBlockMetadataAcceptedBlocks finished")
	}()
}

func (d *DockerTestFramework) AssertBlockMetadataConfirmedBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, finishChan chan struct{}) {
	acceptedChan, subInfo := eventClt.BlockMetadataConfirmedBlocks()
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertBlockMetadataTopics(ctx, acceptedChan, expectedBlockIDs, finishChan)
		fmt.Println("AssertBlockMetadataConfirmedBlocks finished")
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
		fmt.Println("AssertOutput finished")
	}()
}

func (d *DockerTestFramework) AssertOutputsWithMetadataByUnlockConditionAndAddress(ctx context.Context, eventClt *nodeclient.EventAPIClient, condition api.EventAPIUnlockCondition, addr iotago.Address, finishChan chan struct{}) {
	blksChan, subInfo := eventClt.OutputsWithMetadataByUnlockConditionAndAddress(condition, addr)
	require.Nil(d.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		d.assertOutputMetadataTopics(ctx, blksChan, func(resp *api.OutputWithMetadataResponse) bool {
			unlock := resp.Output.UnlockConditionSet()
			switch condition {
			case api.EventAPIUnlockConditionAny:
				return true
			case api.EventAPIUnlockConditionAddress:
				if unlock.Address() != nil && addr.Equal(unlock.Address().Address) {
					return true
				}
			case api.EventAPIUnlockConditionStorageReturn:
				if unlock.HasStorageDepositReturnCondition() && addr.Equal(unlock.StorageDepositReturn().ReturnAddress) {
					return true
				}
			case api.EventAPIUnlockConditionExpiration:
				if unlock.HasExpirationCondition() && addr.Equal(unlock.Expiration().ReturnAddress) {
					return true
				}
			case api.EventAPIUnlockConditionStateController:
				if unlock.StateControllerAddress() != nil && addr.Equal(unlock.StateControllerAddress().Address) {
					return true
				}
			case api.EventAPIUnlockConditionGovernor:
				if unlock.GovernorAddress() != nil && addr.Equal(unlock.GovernorAddress().Address) {
					return true
				}
			case api.EventAPIUnlockConditionImmutableAccount:
				if unlock.ImmutableAccount() != nil && addr.Equal(unlock.ImmutableAccount().Address) {
					return true
				}
			}
			return false
		}, finishChan)
		fmt.Println("AssertOutputsWithMetadataByUnlockConditionAndAddress finished")
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
		fmt.Println("AssertDelegationOutput finished")
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
		fmt.Println("AssertFoundryOutput finished")
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
		fmt.Println("AssertAccountOutput finished")
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
		fmt.Println("AssertAnchorOutput finished")
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
		fmt.Println("AssertNFTOutput finished")
	}()
}

func (d *DockerTestFramework) assertCommitmentsTopics(ctx context.Context, receivedChan <-chan *iotago.Commitment, expectedSlots []iotago.SlotIndex, finishChan chan struct{}) {
	maxSlot := lo.Max(expectedSlots...)
	slots := make([]iotago.SlotIndex, 0)

	for {
		select {
		case commitment := <-receivedChan:
			slots = append(slots, commitment.Slot)
			if commitment.Slot == maxSlot {
				// make sure the commitment is in increasing order.
				require.IsIncreasing(d.Testing, slots)
				// we will receive more slots than expected, so we need to trim the slice
				slots = slots[len(slots)-len(expectedSlots):]

				require.ElementsMatch(d.Testing, expectedSlots, slots)
				finishChan <- struct{}{}
				return
			}
		case <-ctx.Done():
			fmt.Println("Received slots:", slots)
			return
		}
	}
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
