//go:build dockertests

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func (d *DockerTestFramework) CheckAccountStatus(ctx context.Context, blkID iotago.BlockID, txID iotago.TransactionID, creationOutputID iotago.OutputID, accountAddress *iotago.AccountAddress, checkIndexer ...bool) {
	// request by blockID if provided, otherwise use txID
	// we take the slot from the blockID in case the tx is created earlier than the block.
	clt := d.wallet.DefaultClient()
	slot := blkID.Slot()

	if blkID == iotago.EmptyBlockID {
		blkMetadata, err := clt.TransactionIncludedBlockMetadata(ctx, txID)
		require.NoError(d.Testing, err)

		blkID = blkMetadata.BlockID
		slot = blkMetadata.BlockID.Slot()
	}

	d.AwaitTransactionPayloadAccepted(ctx, txID)

	// wait for the account to be committed
	d.AwaitCommitment(slot)

	// Check the indexer
	if len(checkIndexer) > 0 && checkIndexer[0] {
		indexerClt, err := d.wallet.DefaultClient().Indexer(ctx)
		require.NoError(d.Testing, err)

		_, _, _, err = indexerClt.Account(ctx, accountAddress)
		require.NoError(d.Testing, err)
	}

	// check if the creation output exists
	_, err := clt.OutputByID(ctx, creationOutputID)
	require.NoError(d.Testing, err)
}

func (d *DockerTestFramework) AssertIndexerAccount(account *mock.AccountData) {
	d.Eventually(func() error {
		ctx := context.TODO()
		indexerClt, err := d.wallet.DefaultClient().Indexer(ctx)
		if err != nil {
			return err
		}

		outputID, output, _, err := indexerClt.Account(ctx, account.Address)
		if err != nil {
			return err
		}

		assert.EqualValues(d.fakeTesting, account.OutputID, *outputID)
		assert.EqualValues(d.fakeTesting, account.Output, output)

		return nil
	})
}

func (d *DockerTestFramework) AssertIndexerFoundry(foundryID iotago.FoundryID) {
	d.Eventually(func() error {
		ctx := context.TODO()
		indexerClt, err := d.wallet.DefaultClient().Indexer(ctx)
		if err != nil {
			return err
		}

		_, _, _, err = indexerClt.Foundry(ctx, foundryID)
		if err != nil {
			return err
		}

		return nil
	})
}

func (d *DockerTestFramework) AssertValidatorExists(accountAddr *iotago.AccountAddress) {
	d.Eventually(func() error {
		for _, node := range d.Nodes() {
			_, err := d.wallet.Clients[node.Name].Validator(context.TODO(), accountAddr)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (d *DockerTestFramework) AssertCommittee(expectedEpoch iotago.EpochIndex, expectedCommitteeMember []string) {
	fmt.Println("Wait for committee selection..., expected epoch: ", expectedEpoch, ", expected committee size: ", len(expectedCommitteeMember))
	defer fmt.Println("Wait for committee selection......done")

	sort.Strings(expectedCommitteeMember)

	status := d.NodeStatus("V1")
	testAPI := d.wallet.DefaultClient().CommittedAPI()
	expectedSlotStart := testAPI.TimeProvider().EpochStart(expectedEpoch)
	require.Greater(d.Testing, expectedSlotStart, status.LatestAcceptedBlockSlot)

	slotToWait := expectedSlotStart - status.LatestAcceptedBlockSlot
	secToWait := time.Duration(slotToWait) * time.Duration(testAPI.ProtocolParameters().SlotDurationInSeconds()) * time.Second
	fmt.Println("Wait for ", secToWait, "until expected epoch: ", expectedEpoch)
	time.Sleep(secToWait)

	d.Eventually(func() error {
		for _, node := range d.Nodes() {
			resp, err := d.wallet.Clients[node.Name].Committee(context.TODO())
			if err != nil {
				return err
			}

			if resp.Epoch == expectedEpoch {
				members := make([]string, len(resp.Committee))
				for i, member := range resp.Committee {
					members[i] = member.AddressBech32
				}

				sort.Strings(members)
				if match := lo.Equal(expectedCommitteeMember, members); match {
					return nil
				}

				return ierrors.Errorf("committee members does not match as expected, expected: %v, actual: %v", expectedCommitteeMember, members)
			}
		}

		return nil
	})
}

func (d *DockerTestFramework) AssertFinalizedSlot(condition func(iotago.SlotIndex) error) {
	for _, node := range d.Nodes() {
		status := d.NodeStatus(node.Name)

		err := condition(status.LatestFinalizedSlot)
		require.NoError(d.Testing, err)
	}
}

// Eventually asserts that given condition will be met in opts.waitFor time,
// periodically checking target function each opts.tick.
//
//	assert.Eventually(t, func() bool { return true; }, time.Second, 10*time.Millisecond)
func (d *DockerTestFramework) Eventually(condition func() error, waitForSync ...bool) {
	ch := make(chan error, 1)

	deadline := d.optsWaitFor
	if len(waitForSync) > 0 && waitForSync[0] {
		deadline = d.optsWaitForSync
	}

	timer := time.NewTimer(deadline)
	defer timer.Stop()

	ticker := time.NewTicker(d.optsTick)
	defer ticker.Stop()

	var lastErr error
	for tick := ticker.C; ; {
		select {
		case <-timer.C:
			require.FailNow(d.Testing, "condition never satisfied", lastErr)
		case <-tick:
			tick = nil
			go func() { ch <- condition() }()
		case lastErr = <-ch:
			// The condition is satisfied, we can exit.
			if lastErr == nil {
				return
			}
			tick = ticker.C
		}
	}
}

func (d *DockerTestFramework) AwaitTransactionPayloadAccepted(ctx context.Context, txID iotago.TransactionID) {
	clt := d.wallet.DefaultClient()

	d.Eventually(func() error {
		resp, err := clt.TransactionMetadata(ctx, txID)
		if err != nil {
			return err
		}

		if resp.TransactionState == api.TransactionStateAccepted ||
			resp.TransactionState == api.TransactionStateCommitted ||
			resp.TransactionState == api.TransactionStateFinalized {
			if resp.TransactionFailureReason == api.TxFailureNone {
				return nil
			}
		}

		return ierrors.Errorf("transaction %s is pending or having errors, state: %s, failure reason: %d", txID.ToHex(), resp.TransactionState.String(), resp.TransactionFailureReason)
	})
}

func (d *DockerTestFramework) AwaitTransactionState(ctx context.Context, txID iotago.TransactionID, expectedState api.TransactionState) {
	clt := d.wallet.DefaultClient()

	d.Eventually(func() error {
		resp, err := clt.TransactionMetadata(ctx, txID)
		if err != nil {
			return err
		}

		if expectedState == resp.TransactionState {
			return nil
		} else {
			return ierrors.Errorf("expected transaction %s to have state %s, got %s instead", txID, expectedState, resp.TransactionState)
		}
	})
}

func (d *DockerTestFramework) AwaitTransactionFailure(ctx context.Context, txID iotago.TransactionID, expectedReason api.TransactionFailureReason) {
	clt := d.wallet.DefaultClient()

	d.Eventually(func() error {
		resp, err := clt.TransactionMetadata(ctx, txID)
		if err != nil {
			return err
		}

		if expectedReason == resp.TransactionFailureReason {
			return nil
		} else {
			return ierrors.Errorf("expected transaction %s to have failure reason %T, got %s instead, failure details: %s", txID, expectedReason, resp.TransactionState, resp.TransactionFailureDetails)
		}
	})
}

func (d *DockerTestFramework) AwaitCommitment(targetSlot iotago.SlotIndex) {
	currentCommittedSlot := d.NodeStatus("V1").LatestCommitmentID.Slot()

	for t := currentCommittedSlot; t <= targetSlot; t++ {
		latestCommittedSlot := d.NodeStatus("V1").LatestCommitmentID.Slot()

		if targetSlot <= latestCommittedSlot {
			return
		}

		time.Sleep(10 * time.Second)
	}
}

func (d *DockerTestFramework) AwaitFinalization(targetSlot iotago.SlotIndex) {
	d.Eventually(func() error {
		currentFinalisedSlot := d.NodeStatus("V1").LatestFinalizedSlot
		if targetSlot > currentFinalisedSlot {
			return ierrors.Errorf("finalized slot %d is not reached yet", targetSlot)
		}

		return nil
	})
}

func (d *DockerTestFramework) AwaitAddressUnspentOutputAccepted(ctx context.Context, addr iotago.Address) (outputID iotago.OutputID, output iotago.Output, err error) {
	indexerClt, err := d.wallet.DefaultClient().Indexer(ctx)
	require.NoError(d.Testing, err)
	addrBech := addr.Bech32(d.wallet.DefaultClient().CommittedAPI().ProtocolParameters().Bech32HRP())

	for t := time.Now(); time.Since(t) < d.optsWaitFor; time.Sleep(d.optsTick) {
		res, err := indexerClt.Outputs(ctx, &api.BasicOutputsQuery{
			AddressBech32: addrBech,
		})
		if err != nil {
			return iotago.EmptyOutputID, nil, ierrors.Wrap(err, "indexer request failed in request faucet funds")
		}

		for res.Next() {
			unspents, err := res.Outputs(ctx)
			if err != nil {
				return iotago.EmptyOutputID, nil, ierrors.Wrap(err, "failed to get faucet unspent outputs")
			}

			if len(unspents) == 0 {
				break
			}

			return lo.Return1(res.Response.Items.OutputIDs())[0], unspents[0], nil
		}
	}

	return iotago.EmptyOutputID, nil, ierrors.Errorf("no unspent outputs found for address %s due to timeout", addrBech)
}

func (d *DockerTestFramework) SendFaucetRequest(ctx context.Context, receiveAddr iotago.Address) {
	cltAPI := d.wallet.DefaultClient().CommittedAPI()
	addrBech := receiveAddr.Bech32(cltAPI.ProtocolParameters().Bech32HRP())

	type EnqueueRequest struct {
		Address string `json:"address"`
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.optsFaucetURL+"/api/enqueue", func() io.Reader {
		jsonData, _ := json.Marshal(&EnqueueRequest{
			Address: addrBech,
		})

		return bytes.NewReader(jsonData)
	}())
	require.NoError(d.Testing, err)

	req.Header.Set("Content-Type", api.MIMEApplicationJSON)

	res, err := http.DefaultClient.Do(req)
	require.NoError(d.Testing, err)
	defer res.Body.Close()

	require.Equal(d.Testing, http.StatusAccepted, res.StatusCode)
}

func createLogDirectory(testName string) string {
	// make sure logs/ exists
	err := os.Mkdir("logs", 0755)
	if err != nil {
		if !os.IsExist(err) {
			panic(err)
		}
	}

	// create directory for this run
	timestamp := time.Now().Format("20060102_150405")
	dir := fmt.Sprintf("logs/%s-%s", timestamp, testName)
	err = os.Mkdir(dir, 0755)
	if err != nil {
		if !os.IsExist(err) {
			panic(err)
		}
	}

	return dir
}

func getDelegationStartEpoch(api iotago.API, commitmentSlot iotago.SlotIndex) iotago.EpochIndex {
	pastBoundedSlot := commitmentSlot + api.ProtocolParameters().MaxCommittableAge()
	pastBoundedEpoch := api.TimeProvider().EpochFromSlot(pastBoundedSlot)
	pastBoundedEpochEnd := api.TimeProvider().EpochEnd(pastBoundedEpoch)
	registrationSlot := pastBoundedEpochEnd - api.ProtocolParameters().EpochNearingThreshold()

	if pastBoundedSlot <= registrationSlot {
		return pastBoundedEpoch + 1
	}

	return pastBoundedEpoch + 2
}

func isStatusCode(err error, status int) bool {
	if err == nil {
		return false
	}
	code, err := extractStatusCode(err.Error())
	if err != nil {
		return false
	}

	return code == status
}

func extractStatusCode(errorMessage string) (int, error) {
	re := regexp.MustCompile(`code=(\d+)`)
	matches := re.FindStringSubmatch(errorMessage)
	if len(matches) != 2 {
		return 0, ierrors.Errorf("unable to extract status code from error message")
	}

	statusCode, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, err
	}

	return statusCode, nil
}
