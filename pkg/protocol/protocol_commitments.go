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

type CommitmentsProtocol struct {
	protocol   *Protocol
	workerPool *workerpool.WorkerPool
	ticker     *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]

	log.Logger
}

func NewCommitmentsProtocol(protocol *Protocol) *CommitmentsProtocol {
	c := &CommitmentsProtocol{
		Logger:     lo.Return1(protocol.Logger.NewChildLogger("Commitments")),
		protocol:   protocol,
		workerPool: protocol.Workers.CreatePool("Commitments"),
		ticker:     eventticker.New[iotago.SlotIndex, iotago.CommitmentID](protocol.Options.CommitmentRequesterOptions...),
	}

	c.ticker.Events.Tick.Hook(c.SendRequest)

	return c
}

func (c *CommitmentsProtocol) StartTicker(commitmentPromise *promise.Promise[*Commitment], commitmentID iotago.CommitmentID) {
	c.ticker.StartTicker(commitmentID)

	commitmentPromise.OnComplete(func() {
		c.ticker.StopTicker(commitmentID)
	})
}

func (c *CommitmentsProtocol) SendRequest(commitmentID iotago.CommitmentID) {
	c.workerPool.Submit(func() {
		c.protocol.Network.RequestSlotCommitment(commitmentID)

		c.LogDebug("request", "commitment", commitmentID)
	})
}

func (c *CommitmentsProtocol) SendResponse(commitment *Commitment, to peer.ID) {
	c.workerPool.Submit(func() {
		c.protocol.Network.SendSlotCommitment(commitment.Commitment, to)

		c.LogTrace("sent commitment", "commitment", commitment.LogName(), "toPeer", to)
	})
}

func (c *CommitmentsProtocol) ProcessResponse(commitmentModel *model.Commitment, from peer.ID) {
	c.workerPool.Submit(func() {
		if commitment, published, err := c.protocol.Commitments.publishCommitmentModel(commitmentModel); err != nil {
			c.LogError("failed to process commitment", "fromPeer", from, "err", err)
		} else if published {
			c.LogTrace("received response", "commitment", commitment.LogName(), "fromPeer", from)
		}
	})
}

func (c *CommitmentsProtocol) ProcessRequest(commitmentID iotago.CommitmentID, from peer.ID) {
	c.workerPool.Submit(func() {
		commitment, err := c.protocol.Commitments.Get(commitmentID)
		if err != nil {
			logLevel := lo.Cond(ierrors.Is(err, ErrorCommitmentNotFound), log.LevelTrace, log.LevelError)

			c.Log("failed to load commitment for commitment request", logLevel, "commitmentID", commitmentID, "fromPeer", from, "error", err)

			return
		}

		c.SendResponse(commitment, from)
	})
}

func (c *CommitmentsProtocol) Shutdown() {
	c.ticker.Shutdown()
	c.workerPool.Shutdown().ShutdownComplete.Wait()
}
