package tests

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/testsuite/snapshotcreator"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/nodeclient"
	"github.com/iotaledger/iota.go/v4/wallet"
	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/require"
)

var (
	// need to build snapshotfile in tools/docker-network
	snapshotFilePath = "../docker-network.snapshot"
	keyManager       = func() *wallet.KeyManager {
		genesisSeed, err := base58.Decode("7R1itJx5hVuo9w9hjg5cwKFmek4HMSoBDgJZN8hKGxih")
		if err != nil {
			log.Fatal(ierrors.Wrap(err, "failed to decode base58 seed"))
		}
		keyManager, err := wallet.NewKeyManager(genesisSeed[:], wallet.DefaultIOTAPath)
		if err != nil {
			log.Fatal(ierrors.Wrap(err, "failed to create KeyManager from seed"))
		}

		return keyManager
	}
)

type DockerTestFramework struct {
	Testing *testing.T

	validatorContainerNames map[string]string
	clientURLs              map[string]string
	nodeClients             map[string]*nodeclient.Client

	snapshotPath     string
	logDirectoryPath string

	optsProtocolParameterOptions []options.Option[iotago.V3ProtocolParameters]
	optsSnapshotOptions          []options.Option[snapshotcreator.Options]
	optsWaitForSync              time.Duration
	optsWaitFor                  time.Duration
	optsTick                     time.Duration
}

func NewDockerTestFramework(t *testing.T, validatorNames map[string]string, clientURLs map[string]string, opts ...options.Option[DockerTestFramework]) *DockerTestFramework {
	return options.Apply(&DockerTestFramework{
		Testing:                 t,
		validatorContainerNames: validatorNames,
		clientURLs:              clientURLs,
		nodeClients:             make(map[string]*nodeclient.Client),
		optsWaitForSync:         5 * time.Minute,
		optsWaitFor:             2 * time.Minute,
		optsTick:                5 * time.Second,
	}, opts, func(d *DockerTestFramework) {
		d.optsProtocolParameterOptions = append(DefaultProtocolParameterOptions, d.optsProtocolParameterOptions...)

		protocolParams := iotago.NewV3ProtocolParameters(d.optsProtocolParameterOptions...)
		api := iotago.V3API(protocolParams)

		d.logDirectoryPath = createLogDirectory(t.Name())
		d.snapshotPath = snapshotFilePath
		d.optsSnapshotOptions = append(DefaultAccountOptions(protocolParams),
			[]options.Option[snapshotcreator.Options]{
				snapshotcreator.WithDatabaseVersion(protocol.DatabaseVersion),
				snapshotcreator.WithFilePath(d.snapshotPath),
				snapshotcreator.WithProtocolParameters(api.ProtocolParameters()),
				snapshotcreator.WithRootBlocks(map[iotago.BlockID]iotago.CommitmentID{
					api.ProtocolParameters().GenesisBlockID(): iotago.NewEmptyCommitment(api).MustID(),
				}),
				snapshotcreator.WithGenesisKeyManager(keyManager()),
			}...)

		err := snapshotcreator.CreateSnapshot(d.optsSnapshotOptions...)
		if err != nil {
			panic(fmt.Sprintf("failed to create snapshot: %s", err))
		}
	})
}

func (d *DockerTestFramework) Run() error {
	go func() {
		exec.Command("docker", "compose", "up").Run()
	}()

	// first wait until the nodes are available
	d.nodeClients = make(map[string]*nodeclient.Client)
	for name, url := range d.clientURLs {
		for {
			client, err := nodeclient.New(url)
			if err == nil {
				d.nodeClients[name] = client
				break
			}

			time.Sleep(d.optsTick)
			fmt.Printf("Waiting for node %s to be available...\n", name)
		}
	}

	// make sure all nodes are up then we can start dumping logs
	d.DumpContainerLogsToFiles()

	return nil
}

func (d *DockerTestFramework) WaitUntilSync() error {
	fmt.Println("Wait until the nodes are synced...")
	defer fmt.Println("Wait until the nodes are synced......done")

	d.Eventually(func() error {
		for name, client := range d.nodeClients {
			for {
				synced, err := client.Health(context.TODO())
				if err != nil {
					return err
				}

				if synced {
					fmt.Println("Node", name, "is synced")
					break
				}
			}
		}

		return nil
	}, true)

	return nil
}

func (d *DockerTestFramework) GetRandomNode() *nodeclient.Client {
	for _, client := range d.nodeClients {
		return client
	}

	return nil
}

func (d *DockerTestFramework) AssertCommitteeSelection(expectedEpoch iotago.EpochIndex, expectedCommitteeSize int) {
	fmt.Println("Wait for committee selection..., expected epoch: ", expectedEpoch, ", expected committee size: ", expectedCommitteeSize)
	defer fmt.Println("Wait for committee selection......done")

	var node *nodeclient.Client
	for _, client := range d.nodeClients {
		node = client
		break
	}
	status, err := getNodeStatus(node)
	require.NoError(d.Testing, err)

	api := node.CommittedAPI()
	expectedSlotStart := api.TimeProvider().EpochStart(expectedEpoch)
	require.Greater(d.Testing, expectedSlotStart, status.LatestAcceptedBlockSlot)

	slotToWait := expectedSlotStart - status.LatestAcceptedBlockSlot
	secToWait := time.Duration(slotToWait) * time.Duration(api.ProtocolParameters().SlotDurationInSeconds()) * time.Second
	fmt.Println("Wait for ", secToWait, "until expected epoch: ", expectedEpoch)
	time.Sleep(secToWait)

	d.Eventually(func() error {
		for _, client := range d.nodeClients {
			resp, err := client.Committee(context.TODO())
			if err != nil {
				return err
			}

			if resp.Epoch == expectedEpoch {
				if len(resp.Committee) == expectedCommitteeSize {
					return nil
				} else {
					return ierrors.Errorf("committee does not updated as expected")
				}
			}
		}

		return nil
	})
}

func (d *DockerTestFramework) AssertFinalizedSlot(condition func(iotago.SlotIndex) error) {
	for _, client := range d.nodeClients {
		status, err := getNodeStatus(client)
		require.NoError(d.Testing, err)

		err = condition(status.LatestFinalizedSlot)
		require.NoError(d.Testing, err)
	}
}

func (d *DockerTestFramework) Stop() {
	fmt.Println("Stop the network...")
	defer fmt.Println("Stop the network.....done")

	exec.Command("docker", "compose", "down").Run()
	exec.Command("rm", d.snapshotPath).Run()
}

func (d *DockerTestFramework) StopContainer(containerIndex ...string) error {
	fmt.Println("Stop validator", containerIndex, "......")

	args := []string{}
	for _, index := range containerIndex {
		args = append(args, d.validatorContainerNames[index])
	}

	return dockerContainerStop(args...)
}

func (d *DockerTestFramework) RestartContainer(containerIndex ...string) error {
	fmt.Println("Restart validator", containerIndex, "......")

	args := []string{}
	for _, index := range containerIndex {
		args = append(args, d.validatorContainerNames[index])
	}

	return dockerContainerRestart(args...)
}

func (d *DockerTestFramework) DumpContainerLogsToFiles() {
	// get container names
	cmd := "docker compose ps | awk '{print $1}' | tail -n +2"
	containerNamesBytes, err := exec.Command("bash", "-c", cmd).Output()
	require.NoError(d.Testing, err)

	// dump logs to files
	fmt.Println("Dump container logs to files...")
	containerNames := strings.Split(string(containerNamesBytes), "\n")

	for _, name := range containerNames {
		if name == "" {
			continue
		}

		filePath := fmt.Sprintf("%s/%s.log", d.logDirectoryPath, name)
		logCmd := fmt.Sprintf("docker logs -f %s > %s &", name, filePath)
		err := exec.Command("bash", "-c", logCmd).Run()
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

///////////////////////////////

func dockerContainerStop(containerName ...string) error {
	fmt.Println("Stop validator", containerName, "......")

	args := append([]string{"stop"}, containerName...)

	return exec.Command("docker", args...).Run()
}

func dockerContainerRestart(containerName ...string) error {
	fmt.Println("Restart validator", containerName, "......")

	args := append([]string{"restart"}, containerName...)

	return exec.Command("docker", args...).Run()
}

func getNodeStatus(client *nodeclient.Client) (*api.InfoResNodeStatus, error) {
	info, err := client.Info(context.TODO())
	if err != nil {
		return nil, err
	}

	return info.Status, nil
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
