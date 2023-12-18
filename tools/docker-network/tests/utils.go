//go:build dockertests

package tests

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-crypto-demo/pkg/bip32path"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/wallet"
	"github.com/stretchr/testify/require"
)

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

func (d *DockerTestFramework) AwaitTransactionPayloadAccepted(ctx context.Context, blkID iotago.BlockID) {
	clt := d.Node("V1").Client

	d.Eventually(func() error {
		resp, err := clt.BlockMetadataByBlockID(ctx, blkID)
		if err != nil {
			return err
		}

		if resp.TransactionMetadata == nil {
			return ierrors.Errorf("transaction is not included in block: %s", blkID.ToHex())
		}

		if resp.TransactionMetadata.TransactionState == api.TransactionStateAccepted ||
			resp.TransactionMetadata.TransactionState == api.TransactionStateConfirmed ||
			resp.TransactionMetadata.TransactionState == api.TransactionStateFinalized {
			if resp.TransactionMetadata.TransactionFailureReason == api.TxFailureNone {
				return nil
			}
		}

		return ierrors.Errorf("transaction in block %s is pending or having errors, state: %s, failure reason: %d", blkID.ToHex(), resp.TransactionMetadata.TransactionState.String(), resp.TransactionMetadata.TransactionFailureReason)
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

func (d *DockerTestFramework) AwaitAddressUnspentOutputAccepted(ctx context.Context, addr iotago.Address) (outputID iotago.OutputID, output iotago.Output, err error) {
	indexerClt, err := d.Node("V1").Client.Indexer(ctx)
	require.NoError(d.Testing, err)
	addrBech := addr.Bech32(d.Node("V1").Client.CommittedAPI().ProtocolParameters().Bech32HRP())

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
	cltAPI := d.Node("V1").Client.CommittedAPI()
	addrBech := receiveAddr.Bech32(cltAPI.ProtocolParameters().Bech32HRP())
	fmt.Printf("Faucet request funds for Bech address: %s\n", addrBech)

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

func (d *DockerTestFramework) getAddress(addressType iotago.AddressType) (iotago.DirectUnlockableAddress, ed25519.PrivateKey) {
	newIndex := d.latestUsedIndex.Add(1)
	keyManager := lo.PanicOnErr(wallet.NewKeyManager(d.seed[:], BIP32PathForIndex(newIndex)))
	privateKey, _ := keyManager.KeyPair()
	receiverAddr := keyManager.Address(addressType)

	return receiverAddr, privateKey
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

func BIP32PathForIndex(index uint32) string {
	path := lo.PanicOnErr(bip32path.ParsePath(wallet.DefaultIOTAPath))
	if len(path) != 5 {
		panic("invalid path length")
	}

	// Set the index
	path[4] = index

	return path.String()
}
