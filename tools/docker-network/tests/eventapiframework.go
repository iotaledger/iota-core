//go:build dockertests

package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/nodeclient"
)

type EventAPIDockerTestFramework struct {
	Testing *testing.T

	dockerFramework *DockerTestFramework
	DefaultClient   *nodeclient.Client

	finishChan chan struct{}

	optsWaitFor time.Duration
	optsTick    time.Duration
}

func NewEventAPIDockerTestFramework(t *testing.T, dockerFramework *DockerTestFramework) *EventAPIDockerTestFramework {
	return &EventAPIDockerTestFramework{
		Testing:         t,
		dockerFramework: dockerFramework,
		DefaultClient:   dockerFramework.wallet.DefaultClient(),
		finishChan:      make(chan struct{}),
		optsWaitFor:     3 * time.Minute,
		optsTick:        5 * time.Second,
	}
}

func (e *EventAPIDockerTestFramework) ConnectEventAPIClient(ctx context.Context) *nodeclient.EventAPIClient {
	eventClt, err := e.DefaultClient.EventAPI(ctx)
	require.NoError(e.Testing, err)
	err = eventClt.Connect(ctx)
	require.NoError(e.Testing, err)

	return eventClt
}

// SubmitDataBlockStream submits a stream of data blocks to the network for the given duration.
func (e *EventAPIDockerTestFramework) SubmitDataBlockStream(account *mock.AccountData, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	ticker := time.NewTicker(e.optsTick)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			for i := 0; i < 10; i++ {
				blk := e.dockerFramework.CreateTaggedDataBlock(account.ID, []byte("tag"))
				e.dockerFramework.SubmitBlock(context.Background(), blk)
			}
		case <-timer.C:
			return
		}
	}
}

func (e *EventAPIDockerTestFramework) AssertBlockMetadataStateAcceptedBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient) {
	acceptedChan, subInfo := eventClt.BlockMetadataAcceptedBlocks()
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()

		// in order to inform that the channel is listened
		e.finishChan <- struct{}{}

		for {
			select {
			case blk := <-acceptedChan:
				resp, err := eventClt.Client.BlockMetadataByBlockID(ctx, blk.BlockID)
				require.NoError(e.Testing, err)
				// accepted, confirmed are accepted
				require.NotEqualf(e.Testing, api.BlockStatePending, resp.BlockState, "Block %s is pending in BlockMetadataAccepted topic", blk.BlockID.ToHex())

			case <-ctx.Done():
				return
			}
		}
	}()
}

func (e *EventAPIDockerTestFramework) AssertBlockMetadataStateConfirmedBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient) {
	acceptedChan, subInfo := eventClt.BlockMetadataConfirmedBlocks()
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()

		// in order to inform that the channel is listened
		e.finishChan <- struct{}{}

		for {
			select {
			case blk := <-acceptedChan:
				resp, err := eventClt.Client.BlockMetadataByBlockID(ctx, blk.BlockID)
				require.NoError(e.Testing, err)
				require.NotEqualf(e.Testing, api.BlockStatePending, resp.BlockState, "Block %s is pending in BlockMetadataConfirmed endpoint", blk.BlockID.ToHex())
				require.NotEqualf(e.Testing, api.BlockStateAccepted, resp.BlockState, "Block %s is accepted in BlockMetadataConfirmed endpoint", blk.BlockID.ToHex())

			case <-ctx.Done():
				return
			}
		}
	}()
}

func (e *EventAPIDockerTestFramework) AssertLatestCommitments(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedSlots []iotago.SlotIndex) {
	commitmentChan, subInfo := eventClt.CommitmentsLatest()
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		e.assertCommitmentsTopics(ctx, "AssertLatestCommitments", commitmentChan, expectedSlots)
	}()
}

func (e *EventAPIDockerTestFramework) AssertFinalizedCommitments(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedSlots []iotago.SlotIndex) {
	commitmentChan, subInfo := eventClt.CommitmentsFinalized()
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		e.assertCommitmentsTopics(ctx, "AssertFinalizedCommitments", commitmentChan, expectedSlots)
	}()
}

func (e *EventAPIDockerTestFramework) AssertBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block) {
	blksChan, subInfo := eventClt.Blocks()
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		e.assertBlocksTopics(ctx, "AssertBlocks", blksChan, expectedBlockIDs)
	}()
}

func (e *EventAPIDockerTestFramework) AssertBasicBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block) {
	blksChan, subInfo := eventClt.BlocksBasic()
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		e.assertBlocksTopics(ctx, "AssertBasicBlocks", blksChan, expectedBlockIDs)
	}()
}

// AssertValidationBlocks listens to the validation blocks and checks if the block is a validation block and the issuer is in the validators list. The check passes after 10 blocks.
func (e *EventAPIDockerTestFramework) AssertValidationBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, hrp iotago.NetworkPrefix, validators map[string]struct{}) {
	blksChan, subInfo := eventClt.BlocksValidation()
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		blkIDs := make([]string, 0)
		counter := 0

		// in order to inform that the channel is listened
		e.finishChan <- struct{}{}

		for {
			select {
			case blk := <-blksChan:
				require.Equal(e.Testing, iotago.BlockBodyTypeValidation, blk.Body.Type())
				_, ok := validators[blk.Header.IssuerID.ToAddress().Bech32(hrp)]
				require.True(e.Testing, ok)

				// The check passes after 10 blocks
				counter++
				if counter == 10 {
					e.finishChan <- struct{}{}
					return
				}
			case <-ctx.Done():
				fmt.Println("Received blocks:", blkIDs)
				return
			}
		}

	}()
}

func (e *EventAPIDockerTestFramework) AssertTaggedDataBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block) {
	blksChan, subInfo := eventClt.BlocksBasicWithTaggedData()
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		e.assertBlocksTopics(ctx, "AssertTaggedDataBlocks", blksChan, expectedBlockIDs)
	}()
}

func (e *EventAPIDockerTestFramework) AssertTaggedDataBlocksByTag(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, tag []byte) {
	blksChan, subInfo := eventClt.BlocksBasicWithTaggedDataByTag(tag)
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		e.assertBlocksTopics(ctx, "AssertTaggedDataBlocksByTag", blksChan, expectedBlockIDs)
	}()
}

func (e *EventAPIDockerTestFramework) AssertTransactionBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block) {
	blksChan, subInfo := eventClt.BlocksBasicWithTransactions()
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		e.assertBlocksTopics(ctx, "AssertTransactionBlocks", blksChan, expectedBlockIDs)
	}()
}

func (e *EventAPIDockerTestFramework) AssertTransactionTaggedDataBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block) {
	blksChan, subInfo := eventClt.BlocksBasicWithTransactionsWithTaggedData()
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		e.assertBlocksTopics(ctx, "AssertTransactionTaggedDataBlocks", blksChan, expectedBlockIDs)
	}()
}

func (e *EventAPIDockerTestFramework) AssertTransactionBlocksByTag(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block, tag []byte) {
	blksChan, subInfo := eventClt.BlocksBasicWithTransactionsWithTaggedDataByTag(tag)
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		e.assertBlocksTopics(ctx, "AssertTransactionBlocksByTag", blksChan, expectedBlockIDs)
	}()
}

func (e *EventAPIDockerTestFramework) AssertTransactionMetadataIncludedBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, txID iotago.TransactionID) {
	acceptedChan, subInfo := eventClt.BlockMetadataTransactionIncludedBlocksByTransactionID(txID)
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		// in order to inform that the channel is listened
		e.finishChan <- struct{}{}

		for {
			select {
			case <-acceptedChan:
				e.finishChan <- struct{}{}
				return

			case <-ctx.Done():
				fmt.Println("topic AssertTransactionMetadataIncludedBlocks does not get expected BlockMetadata")
				return
			}
		}

	}()
}

func (e *EventAPIDockerTestFramework) AssertTransactionMetadataByTransactionID(ctx context.Context, eventClt *nodeclient.EventAPIClient, txID iotago.TransactionID) {
	acceptedChan, subInfo := eventClt.TransactionMetadataByTransactionID(txID)
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		// in order to inform that the channel is listened
		e.finishChan <- struct{}{}

		for {
			select {
			case metadata := <-acceptedChan:
				if txID.Compare(metadata.TransactionID) == 0 {
					e.finishChan <- struct{}{}
					return
				}

			case <-ctx.Done():
				fmt.Println("topic AssertTransactionMetadataByTransactionID does not get expected transaction metadata")
				return
			}
		}
	}()
}

func (e *EventAPIDockerTestFramework) AssertBlockMetadataAcceptedBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block) {
	acceptedChan, subInfo := eventClt.BlockMetadataAcceptedBlocks()
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		e.assertBlockMetadataTopics(ctx, "AssertBlockMetadataAcceptedBlocks", acceptedChan, expectedBlockIDs)
	}()
}

func (e *EventAPIDockerTestFramework) AssertBlockMetadataConfirmedBlocks(ctx context.Context, eventClt *nodeclient.EventAPIClient, expectedBlockIDs map[string]*iotago.Block) {
	acceptedChan, subInfo := eventClt.BlockMetadataConfirmedBlocks()
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		e.assertBlockMetadataTopics(ctx, "AssertBlockMetadataConfirmedBlocks", acceptedChan, expectedBlockIDs)
	}()
}

func (e *EventAPIDockerTestFramework) AssertOutput(ctx context.Context, eventClt *nodeclient.EventAPIClient, outputId iotago.OutputID) {
	outputMetadataChan, subInfo := eventClt.OutputWithMetadataByOutputID(outputId)
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		e.assertOutputMetadataTopics(ctx, "AssertOutput", outputMetadataChan, func(resp *api.OutputWithMetadataResponse) bool {
			if outputId.Compare(resp.Metadata.OutputID) == 0 {
				return true
			}
			return false
		})
	}()
}

func (e *EventAPIDockerTestFramework) AssertOutputsWithMetadataByUnlockConditionAndAddress(ctx context.Context, eventClt *nodeclient.EventAPIClient, condition api.EventAPIUnlockCondition, addr iotago.Address) {
	blksChan, subInfo := eventClt.OutputsWithMetadataByUnlockConditionAndAddress(condition, addr)
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		e.assertOutputMetadataTopics(ctx, "AssertOutputsWithMetadataByUnlockConditionAndAddress", blksChan, func(resp *api.OutputWithMetadataResponse) bool {
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
		})
	}()
}

func (e *EventAPIDockerTestFramework) AssertDelegationOutput(ctx context.Context, eventClt *nodeclient.EventAPIClient, delegationId iotago.DelegationID) {
	outputMetadataChan, subInfo := eventClt.OutputsWithMetadataByDelegationID(delegationId)
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		e.assertOutputMetadataTopics(ctx, "AssertDelegationOutput", outputMetadataChan, func(resp *api.OutputWithMetadataResponse) bool {
			if resp.Output.Type() == iotago.OutputDelegation {
				o, ok := resp.Output.(*iotago.DelegationOutput)
				require.True(e.Testing, ok)
				actualDelegationID := o.DelegationID
				if actualDelegationID.Empty() {
					actualDelegationID = iotago.DelegationIDFromOutputID(resp.Metadata.OutputID)
				}

				return delegationId.Matches(actualDelegationID)
			}

			return false
		})
	}()
}

func (e *EventAPIDockerTestFramework) AssertFoundryOutput(ctx context.Context, eventClt *nodeclient.EventAPIClient, foundryId iotago.FoundryID) {
	outputMetadataChan, subInfo := eventClt.OutputsWithMetadataByFoundryID(foundryId)
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		e.assertOutputMetadataTopics(ctx, "AssertFoundryOutput", outputMetadataChan, func(resp *api.OutputWithMetadataResponse) bool {
			if resp.Output.Type() == iotago.OutputFoundry {
				o, ok := resp.Output.(*iotago.FoundryOutput)
				require.True(e.Testing, ok)

				return foundryId.Matches(o.MustFoundryID())
			}

			return false
		})
	}()
}

func (e *EventAPIDockerTestFramework) AssertAccountOutput(ctx context.Context, eventClt *nodeclient.EventAPIClient, accountId iotago.AccountID) {
	outputMetadataChan, subInfo := eventClt.OutputsWithMetadataByAccountID(accountId)
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		e.assertOutputMetadataTopics(ctx, "AssertAccountOutput", outputMetadataChan, func(resp *api.OutputWithMetadataResponse) bool {
			if resp.Output.Type() == iotago.OutputAccount {
				o, ok := resp.Output.(*iotago.AccountOutput)
				require.True(e.Testing, ok)
				actualAccountID := o.AccountID
				if actualAccountID.Empty() {
					actualAccountID = iotago.AccountIDFromOutputID(resp.Metadata.OutputID)
				}

				return accountId.Matches(actualAccountID)
			}

			return false
		})
	}()
}

func (e *EventAPIDockerTestFramework) AssertAnchorOutput(ctx context.Context, eventClt *nodeclient.EventAPIClient, anchorId iotago.AnchorID) {
	outputMetadataChan, subInfo := eventClt.OutputsWithMetadataByAnchorID(anchorId)
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		e.assertOutputMetadataTopics(ctx, "AssertAnchorOutput", outputMetadataChan, func(resp *api.OutputWithMetadataResponse) bool {
			if resp.Output.Type() == iotago.OutputAnchor {
				o, ok := resp.Output.(*iotago.AnchorOutput)
				require.True(e.Testing, ok)
				actualAnchorID := o.AnchorID
				if actualAnchorID.Empty() {
					actualAnchorID = iotago.AnchorIDFromOutputID(resp.Metadata.OutputID)
				}

				return anchorId.Matches(actualAnchorID)
			}

			return false
		})
	}()
}

func (e *EventAPIDockerTestFramework) AssertNFTOutput(ctx context.Context, eventClt *nodeclient.EventAPIClient, nftId iotago.NFTID) {
	outputMetadataChan, subInfo := eventClt.OutputsWithMetadataByNFTID(nftId)
	require.Nil(e.Testing, subInfo.Error())

	go func() {
		defer subInfo.Close()
		e.assertOutputMetadataTopics(ctx, "AssertNFTOutput", outputMetadataChan, func(resp *api.OutputWithMetadataResponse) bool {
			if resp.Output.Type() == iotago.OutputNFT {
				o := resp.Output.(*iotago.NFTOutput)
				actualNFTID := o.NFTID
				if actualNFTID.Empty() {
					actualNFTID = iotago.NFTIDFromOutputID(resp.Metadata.OutputID)
				}

				return nftId.Matches(actualNFTID)
			}

			return false
		})
	}()
}

func (e *EventAPIDockerTestFramework) assertCommitmentsTopics(ctx context.Context, callerName string, receivedChan <-chan *iotago.Commitment, expectedSlots []iotago.SlotIndex) {
	maxSlot := lo.Max(expectedSlots...)
	slots := make([]iotago.SlotIndex, 0)

	for {
		select {
		case commitment := <-receivedChan:
			slots = append(slots, commitment.Slot)
			if commitment.Slot == maxSlot {
				// make sure the commitment is in increasing order.
				require.IsIncreasing(e.Testing, slots)
				// we will receive more slots than expected, so we need to trim the slice
				slots = slots[len(slots)-len(expectedSlots):]

				require.ElementsMatch(e.Testing, expectedSlots, slots)
				e.finishChan <- struct{}{}
				return
			}
		case <-ctx.Done():
			fmt.Println("topic", callerName, "does not get expected commitments, received slots:", slots)

			return
		}
	}
}

func (e *EventAPIDockerTestFramework) assertBlocksTopics(ctx context.Context, callerName string, receivedChan <-chan *iotago.Block, expectedBlocks map[string]*iotago.Block) {
	expectedBlockIDsSlice := lo.Keys(expectedBlocks)
	blkIDs := make([]string, 0)

	// in order to inform that the channel is listened
	e.finishChan <- struct{}{}

	for {
		select {
		case blk := <-receivedChan:
			_, ok := expectedBlocks[blk.MustID().ToHex()]
			if ok {
				blkIDs = append(blkIDs, blk.MustID().ToHex())

				if len(blkIDs) == len(expectedBlocks) {
					require.ElementsMatch(e.Testing, expectedBlockIDsSlice, blkIDs)
					e.finishChan <- struct{}{}
					return
				}
			}
		case <-ctx.Done():
			fmt.Println("topic", callerName, "does not get expected Blocks, received blocks:", blkIDs)

			return
		}
	}
}

func (e *EventAPIDockerTestFramework) assertBlockMetadataTopics(ctx context.Context, callerName string, receivedChan <-chan *api.BlockMetadataResponse, expectedBlocks map[string]*iotago.Block) {
	expectedBlockIDsSlice := lo.Keys(expectedBlocks)
	blkIDs := make([]string, 0)

	// in order to inform that the channel is listened
	e.finishChan <- struct{}{}

	for {
		select {
		case blk := <-receivedChan:
			id := blk.BlockID.ToHex()
			_, ok := expectedBlocks[id]
			if ok {
				blkIDs = append(blkIDs, id)

				if len(blkIDs) == len(expectedBlocks) {
					require.ElementsMatch(e.Testing, expectedBlockIDsSlice, blkIDs)
					e.finishChan <- struct{}{}
					return
				}
			}
		case <-ctx.Done():
			fmt.Println("topic", callerName, "does not get expected BlockMetadata, received blocks:", blkIDs)

			return
		}
	}
}

func (e *EventAPIDockerTestFramework) assertOutputMetadataTopics(ctx context.Context, callerName string, receivedChan <-chan *api.OutputWithMetadataResponse, matchFunc func(*api.OutputWithMetadataResponse) bool) {
	// in order to inform that the channel is listened
	e.finishChan <- struct{}{}

	for {
		select {
		case outputMetadata := <-receivedChan:
			if matchFunc(outputMetadata) {
				e.finishChan <- struct{}{}
				return
			}
		case <-ctx.Done():
			fmt.Println("topic", callerName, "does not get expected outputs")

			return
		}
	}
}

func (e *EventAPIDockerTestFramework) AwaitEventAPITopics(t *testing.T, cancleFunc context.CancelFunc, numOfTopics int) error {
	counter := 0
	timer := time.NewTimer(e.optsWaitFor)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			cancleFunc()
			return ierrors.New("Timeout, did not receive signals from all topics")
		case <-e.finishChan:
			counter++
			if counter == numOfTopics {
				fmt.Println("Received all signals from topics")
				return nil
			}
		}
	}
}

func WithEventAPIWaitFor(waitFor time.Duration) options.Option[EventAPIDockerTestFramework] {
	return func(d *EventAPIDockerTestFramework) {
		d.optsWaitFor = waitFor
	}
}

func WithEventAPITick(tick time.Duration) options.Option[EventAPIDockerTestFramework] {
	return func(d *EventAPIDockerTestFramework) {
		d.optsTick = tick
	}
}
