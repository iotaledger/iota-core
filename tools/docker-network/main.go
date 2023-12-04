package main

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/nodeclient"
)

var ValidatorContainerName = map[string]string{
	"V1": "docker-network-inx-validator-1-1",
	"V2": "docker-network-inx-validator-2-1",
	"V3": "docker-network-inx-validator-3-1",
	"V4": "docker-network-inx-validator-4-1",
}

func main() {
	defer DockerNetworkStop()
	client := DockerNetworkRun()

	err := WaitUntilSync(client)
	if err != nil {
		panic(err)
	}

	err = SmallerCommittee(client)
	if err != nil {
		panic(err)
	}

	err = ReuseDueToNoFinalization(client)
	if err != nil {
		panic(err)
	}
}

func SmallerCommittee(client *nodeclient.Client) error {
	fmt.Println("Test: committee rotation to a smaller committee than targetCommitteeSize starts...")
	defer fmt.Printf("Test......done\n\n\n")

	status, err := GetNodeStatus(client)
	if err != nil {
		return err
	}
	currentEpoch := client.CommittedAPI().TimeProvider().EpochFromSlot(status.LatestAcceptedBlockSlot)

	// stop validator 2
	err = DockerContainerStop(ValidatorContainerName["V2"])
	if err != nil {
		return ierrors.Wrap(err, "dockerStop failed")
	}

	err = AwaitCommitteeSelection(client, currentEpoch+2, 3)
	if err != nil {
		return ierrors.Errorf("awaitCommitteeSelection failed: %v", err)
	}

	// restart validator 2
	err = DockerContainerRestart(ValidatorContainerName["V2"])
	if err != nil {
		return ierrors.Wrap(err, "dockerRestart failed")
	}

	err = AwaitCommitteeSelection(client, currentEpoch+3, 4)
	if err != nil {
		return ierrors.Errorf("awaitCommitteeSelection failed: %v", err)
	}

	return nil
}

func ReuseDueToNoFinalization(client *nodeclient.Client) error {
	fmt.Println("Test ReuseDueToNoFinalization: committee reuse due to no slot finalization at epochNearingThreshold and recovery after finalization comes back...")
	defer fmt.Printf("Test......done\n\n\n")

	// stop 2 validators, finalization should stop
	err := DockerContainerStop(ValidatorContainerName["V2"], ValidatorContainerName["V3"])
	if err != nil {
		return ierrors.Wrap(err, "dockerStop failed")
	}

	status, err := GetNodeStatus(client)
	if err != nil {
		return err
	}
	prevFinalizedSlot := status.LatestFinalizedSlot
	fmt.Println("First finalized slot: ", prevFinalizedSlot)

	currentEpoch := client.CommittedAPI().TimeProvider().EpochFromSlot(prevFinalizedSlot)

	// Due to no finalization, committee should be reused, remain 4 validators
	err = AwaitCommitteeSelection(client, currentEpoch+2, 4)
	if err != nil {
		return ierrors.Errorf("awaitCommitteeSelection failed: %v", err)
	}

	// check if finalization stops
	status, err = GetNodeStatus(client)
	if err != nil {
		return err
	}
	fmt.Println("Second finalized slot: ", status.LatestFinalizedSlot)

	if prevFinalizedSlot != status.LatestFinalizedSlot {
		return ierrors.Errorf("NO finalization should happened, First finalized slot: %d, Second finalized slot: %d", prevFinalizedSlot, status.LatestFinalizedSlot)
	}

	// revive 1 validator, committee size should be 3, finalization should resume
	err = DockerContainerRestart(ValidatorContainerName["V2"])
	if err != nil {
		return ierrors.Wrap(err, "dockerRestart failed")
	}

	err = AwaitCommitteeSelection(client, currentEpoch+3, 3)
	if err != nil {
		return ierrors.Errorf("awaitCommitteeSelection failed: %v", err)
	}

	// wait finalization to catch up and check if the finalization resumes
	time.Sleep(5 * time.Second)
	status, err = GetNodeStatus(client)
	if err != nil {
		return err
	}
	fmt.Println("Third finalized slot: ", status.LatestFinalizedSlot)

	if prevFinalizedSlot == status.LatestFinalizedSlot {
		return ierrors.Errorf("Finalization should happened, Second finalized slot: %d, Third finalized slot: %d", prevFinalizedSlot, status.LatestFinalizedSlot)
	}

	return nil
}

func WaitUntilSync(client *nodeclient.Client) error {
	fmt.Println("Wait until the node is synced...")
	defer fmt.Println("Wait until the node is synced......done")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	for {
		synced, err := client.Health(ctx)
		if err != nil {
			return err
		}

		if synced {
			return nil
		}

		time.Sleep(5 * time.Second)
	}
}

func GetNodeStatus(client *nodeclient.Client) (*api.InfoResNodeStatus, error) {
	info, err := client.Info(context.TODO())
	if err != nil {
		return nil, err
	}

	return info.Status, nil
}

func AwaitCommitteeSelection(client *nodeclient.Client, expectedEpoch iotago.EpochIndex, expectedCommitteeSize int) error {
	fmt.Println("Wait for committee selection..., expected epoch: ", expectedEpoch, ", expected committee size: ", expectedCommitteeSize)
	defer fmt.Println("Wait for committee selection......done")

	status, err := GetNodeStatus(client)
	if err != nil {
		return err
	}

	expectedSlotStart := client.CommittedAPI().TimeProvider().EpochStart(expectedEpoch)
	if status.LatestFinalizedSlot > expectedSlotStart {
		return ierrors.Errorf("invalid expected epoch, abort: %d < %d (in slot)", expectedSlotStart, status.LatestFinalizedSlot)
	}

	slotToWait := expectedSlotStart - status.LatestAcceptedBlockSlot
	secToWait := time.Duration(slotToWait) * time.Duration(client.CommittedAPI().ProtocolParameters().SlotDurationInSeconds()) * time.Second
	fmt.Println("Wait for ", secToWait, "until expected epoch: ", expectedEpoch)
	time.Sleep(secToWait)

	maxTries := 10
	for i := 0; i < maxTries; i++ {
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

		time.Sleep(2 * time.Second)
	}

	return ierrors.Errorf("committee did not update in %d seconds", maxTries*2)
}

func DockerNetworkRun() *nodeclient.Client {
	go func() {
		exec.Command("./run_dev_test.sh", "2>&1", "&").Run()
	}()

	// first wait until the node is available
	for {
		client, err := nodeclient.New("http://localhost:8050")
		if err == nil {
			return client
		}

		time.Sleep(5 * time.Second)
		fmt.Println("Waiting for the node to be available...")
	}
}

func DockerNetworkStop() {
	fmt.Println("Stop the network...")
	defer fmt.Println("Stop the network.....done")

	exec.Command("docker", "compose", "down").Run()
	exec.Command("rm", "docker-network.snapshot").Run()
}

func DockerContainerStop(containerName ...string) error {
	fmt.Println("Stop validator", containerName, "......")

	args := append([]string{"stop"}, containerName...)

	return exec.Command("docker", args...).Run()
}

func DockerContainerRestart(containerName ...string) error {
	fmt.Println("Restart validator", containerName, "......")

	args := append([]string{"restart"}, containerName...)

	return exec.Command("docker", args...).Run()
}
