package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

// CommitmentsProtocol is a subcomponent of the protocol that is responsible for handling commitment requests and
// responses.
type CommitmentsProtocol struct {
	// protocol contains a reference to the Protocol instance that this component belongs to.
	protocol *Protocol

	// workerPool contains the worker pool that is used to process commitment requests and responses asynchronously.
	workerPool *workerpool.WorkerPool

	// ticker contains the ticker that is used to send commitment requests.
	ticker *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]

	// Logger embeds a logger that can be used to log messages emitted by this chain.
	log.Logger
}

// newCommitmentsProtocol creates a new commitment protocol instance for the given protocol.
func newCommitmentsProtocol(protocol *Protocol) *CommitmentsProtocol {
	c := &CommitmentsProtocol{
		Logger:     lo.Return1(protocol.Logger.NewChildLogger("Commitments")),
		protocol:   protocol,
		workerPool: protocol.Workers.CreatePool("Commitments"),
		ticker:     eventticker.New[iotago.SlotIndex, iotago.CommitmentID](protocol.Options.CommitmentRequesterOptions...),
	}

	c.ticker.Events.Tick.Hook(c.SendRequest)

	return c
}

// StartTicker starts the ticker for the given commitment.
func (c *CommitmentsProtocol) StartTicker(commitmentPromise *promise.Promise[*Commitment], commitmentID iotago.CommitmentID) {
	c.ticker.StartTicker(commitmentID)

	commitmentPromise.OnComplete(func() {
		c.ticker.StopTicker(commitmentID)
	})
}

// SendRequest sends a commitment request for the given commitment ID to all peers.
func (c *CommitmentsProtocol) SendRequest(commitmentID iotago.CommitmentID) {
	c.workerPool.Submit(func() {
		c.protocol.Network.RequestSlotCommitment(commitmentID)

		c.LogDebug("request", "commitment", commitmentID)
	})
}

// SendResponse sends a commitment response for the given commitment to the given peer.
func (c *CommitmentsProtocol) SendResponse(commitment *Commitment, to peer.ID) {
	c.workerPool.Submit(func() {
		c.protocol.Network.SendSlotCommitment(commitment.Commitment, to)

		c.LogTrace("sent commitment", "commitment", commitment.LogName(), "toPeer", to)
	})
}

// ProcessResponse processes the given commitment response.
func (c *CommitmentsProtocol) ProcessResponse(commitmentModel *model.Commitment, from peer.ID) {
	c.workerPool.Submit(func() {
		// Verify the commitment's version corresponds to the protocol version for the slot.
		apiForSlot := c.protocol.APIForSlot(commitmentModel.Slot())
		if apiForSlot.Version() != commitmentModel.Commitment().ProtocolVersion {
			c.LogDebug("received commitment with invalid protocol version", "commitment", commitmentModel.ID(), "version", commitmentModel.Commitment().ProtocolVersion, "expectedVersion", apiForSlot.Version(), "fromPeer", from)

			return
		}

		if commitment, published, err := c.protocol.Commitments.publishCommitmentModel(commitmentModel); err != nil {
			c.LogError("failed to process commitment", "fromPeer", from, "err", err)
		} else if published {
			c.LogTrace("received response", "commitment", commitment.LogName(), "fromPeer", from)
		}
	})
}

// ProcessRequest processes the given commitment request.
func (c *CommitmentsProtocol) ProcessRequest(commitmentID iotago.CommitmentID, from peer.ID) {
	c.workerPool.Submit(func() {
		commitment, err := c.protocol.Commitments.Get(commitmentID)
		if err != nil {
			if !ierrors.Is(err, ErrorCommitmentNotFound) {
				c.LogError("failed to load commitment for commitment request", "commitmentID", commitmentID, "fromPeer", from, "error", err)

				return
			}

			slotAPI, slotAPIErr := c.protocol.Engines.Main.Get().CommittedSlot(commitmentID)
			if slotAPIErr != nil {
				c.LogDebug("failed to load committed slot for commitment request", "commitmentID", commitmentID, "fromPeer", from, "error", slotAPIErr)

				return
			}

			commitmentModel, slotAPIErr := slotAPI.Commitment()
			if slotAPIErr != nil {
				c.LogDebug("failed to load commitment for commitment request", "commitmentID", commitmentID, "fromPeer", from, "error", slotAPIErr)

				return
			}

			c.protocol.Network.SendSlotCommitment(commitmentModel, from)

			c.LogTrace("sent commitment", "commitmentID", commitmentID, "toPeer", from)

			return
		}

		c.SendResponse(commitment, from)
	})
}

// Shutdown shuts down the commitment protocol and waits for all pending requests to be processed.
func (c *CommitmentsProtocol) Shutdown() {
	c.ticker.Shutdown()
	c.workerPool.Shutdown().ShutdownComplete.Wait()
}
