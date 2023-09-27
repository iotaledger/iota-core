package spammer

// import (
// 	"crypto/sha256"
// 	"math/rand"
// 	"time"

// 	"github.com/iotaledger/hive.go/core/slot"
// 	"github.com/iotaledger/iota-core/tools/evil-spammer/wallet"
// 	iotago "github.com/iotaledger/iota.go/v4"

// 	"github.com/pkg/errors"
// )

// type CommitmentManagerParams struct {
// 	CommitmentType  string
// 	ValidClientURL  string
// 	ParentRefsCount int
// 	ClockResyncTime time.Duration
// 	GenesisTime     time.Time
// 	SlotDuration    time.Duration

// 	OptionalForkAfter int
// }
// type CommitmentManager struct {
// 	Params *CommitmentManagerParams
// 	// we store here only the valid commitments to not request them again through API
// 	validChain map[slot.Slot]*iotago.Commitment
// 	// commitments used to spam
// 	commitmentChain map[slot.Slot]*iotago.Commitment

// 	initiationSlot  slot.Slot
// 	forkIndex       slot.Slot
// 	latestCommitted slot.Slot

// 	clockSync   *ClockSync
// 	validClient wallet.Client

// 	log Logger
// }

// func NewCommitmentManager() *CommitmentManager {
// 	return &CommitmentManager{
// 		Params: &CommitmentManagerParams{
// 			ParentRefsCount: 2,
// 			ClockResyncTime: 30 * time.Second,
// 			GenesisTime:     time.Now(),
// 			SlotDuration:    5 * time.Second,
// 		},
// 		validChain:      make(map[slot.Slot]*iotago.Commitment),
// 		commitmentChain: make(map[slot.Slot]*iotago.Commitment),
// 	}
// }

// func (c *CommitmentManager) Setup(l Logger) {
// 	c.log = l

// 	c.log.Infof("Commitment Manager will be based on the valid client: %s", c.Params.ValidClientURL)
// 	c.validClient = wallet.NewWebClient(c.Params.ValidClientURL)
// 	c.setupTimeParams(c.validClient)

// 	c.clockSync = NewClockSync(c.Params.SlotDuration, c.Params.ClockResyncTime, c.validClient)
// 	c.clockSync.Start()

// 	c.setupForkingPoint()
// 	c.setupInitCommitment()
// }

// // SetupInitCommitment sets the initiation commitment which is the current valid commitment requested from validClient.
// func (c *CommitmentManager) setupInitCommitment() {
// 	c.initiationSlot = c.clockSync.LatestCommittedSlotClock.Get()
// 	comm, err := c.getValidCommitment(c.initiationSlot)
// 	if err != nil {
// 		panic(errors.Wrapf(err, "failed to get initiation commitment"))
// 	}
// 	c.commitmentChain[comm.Slot()] = comm
// 	c.latestCommitted = comm.Slot()
// }

// // SetupTimeParams requests through API and sets the genesis time and slot duration for the commitment manager.
// func (c *CommitmentManager) setupTimeParams(clt wallet.Client) {
// 	genesisTime, slotDuration, err := clt.GetTimeProvider()
// 	if err != nil {
// 		panic(errors.Wrapf(err, "failed to get time provider for the commitment manager setup"))
// 	}
// 	c.Params.GenesisTime = genesisTime
// 	c.Params.SlotDuration = slotDuration
// }

// func (c *CommitmentManager) SetCommitmentType(commitmentType string) {
// 	c.Params.CommitmentType = commitmentType
// }

// func (c *CommitmentManager) SetForkAfter(forkAfter int) {
// 	c.Params.OptionalForkAfter = forkAfter
// }

// // SetupForkingPoint sets the forking point for the commitment manager. It uses ForkAfter parameter so need to be called after params are read.
// func (c *CommitmentManager) setupForkingPoint() {
// 	c.forkIndex = c.clockSync.LatestCommittedSlotClock.Get() + slot.Slot(c.Params.OptionalForkAfter)
// }

// func (c *CommitmentManager) Shutdown() {
// 	c.clockSync.Shutdown()
// }

// func (c *CommitmentManager) commit(comm *iotago.Commitment) {
// 	c.commitmentChain[comm.Slot()] = comm
// 	if comm.Slot() > c.latestCommitted {
// 		if comm.Slot()-c.latestCommitted != 1 {
// 			panic("next committed slot is not sequential, lastCommitted: " + c.latestCommitted.String() + " nextCommitted: " + comm.Slot().String())
// 		}
// 		c.latestCommitted = comm.Slot()
// 	}
// }

// func (c *CommitmentManager) getLatestCommitment() *iotago.Commitment {
// 	return c.commitmentChain[c.latestCommitted]
// }

// // GenerateCommitment generates a commitment based on the commitment type provided in spam details.
// func (c *CommitmentManager) GenerateCommitment(clt wallet.Client) (*iotago.Commitment, slot.Slot, error) {
// 	switch c.Params.CommitmentType {
// 	// todo refactor this to work with chainsA
// 	case "latest":
// 		comm, err := clt.GetLatestCommitment()
// 		if err != nil {
// 			return nil, 0, errors.Wrap(err, "failed to get latest commitment")
// 		}
// 		index, err := clt.GetLatestConfirmedIndex()
// 		if err != nil {
// 			return nil, 0, errors.Wrap(err, "failed to get latest confirmed index")
// 		}
// 		return comm, index, err
// 	case "random":
// 		slot := c.clockSync.LatestCommittedSlotClock.Get()
// 		newCommitment := randomCommitmentChain(slot)

// 		return newCommitment, slot - 10, nil

// 	case "fork":
// 		// it should request time periodically, and be relative
// 		slot := c.clockSync.LatestCommittedSlotClock.Get()
// 		// make sure chain is upto date to the forking point
// 		uptoSlot := c.forkIndex
// 		// get minimum
// 		if slot < c.forkIndex {
// 			uptoSlot = slot
// 		}
// 		err := c.updateChainWithValidCommitment(uptoSlot)
// 		if err != nil {
// 			return nil, 0, errors.Wrap(err, "failed to update chain with valid commitment")
// 		}
// 		if c.isAfterForkPoint(slot) {
// 			c.updateForkedChain(slot)
// 		}
// 		comm := c.getLatestCommitment()
// 		index, err := clt.GetLatestConfirmedIndex()
// 		if err != nil {
// 			return nil, 0, errors.Wrap(err, "failed to get latest confirmed index")
// 		}
// 		return comm, index - 1, nil
// 	}
// 	return nil, 0, nil
// }

// func (c *CommitmentManager) isAfterForkPoint(slot slot.Slot) bool {
// 	return c.forkIndex != 0 && slot > c.forkIndex
// }

// // updateChainWithValidCommitment commits the chain up to the given slot with the valid commitments.
// func (c *CommitmentManager) updateChainWithValidCommitment(s slot.Slot) error {
// 	for i := c.latestCommitted + 1; i <= s; i++ {
// 		comm, err := c.getValidCommitment(i)
// 		if err != nil {
// 			return errors.Wrapf(err, "failed to get valid commitment for slot %d", i)
// 		}
// 		c.commit(comm)
// 	}
// 	return nil
// }

// func (c *CommitmentManager) updateForkedChain(slot slot.Slot) {
// 	for i := c.latestCommitted + 1; i <= slot; i++ {
// 		comm, err := c.getForkedCommitment(i)
// 		if err != nil {
// 			panic(errors.Wrapf(err, "failed to get forked commitment for slot %d", i))
// 		}
// 		c.commit(comm)
// 	}
// }

// // getValidCommitment returns the valid commitment for the given slot if not exists it requests it from the node and update the validChain.
// func (c *CommitmentManager) getValidCommitment(slot slot.Slot) (*commitment.Commitment, error) {
// 	if comm, ok := c.validChain[slot]; ok {
// 		return comm, nil
// 	}
// 	// if not requested before then get it from the node
// 	comm, err := c.validClient.GetCommitment(int(slot))
// 	if err != nil {
// 		return nil, errors.Wrapf(err, "failed to get commitment for slot %d", slot)
// 	}
// 	c.validChain[slot] = comm

// 	return comm, nil
// }

// func (c *CommitmentManager) getForkedCommitment(slot slot.Slot) (*commitment.Commitment, error) {
// 	validComm, err := c.getValidCommitment(slot)
// 	if err != nil {
// 		return nil, errors.Wrapf(err, "failed to get valid commitment for slot %d", slot)
// 	}
// 	prevComm := c.commitmentChain[slot-1]
// 	forkedComm := commitment.New(
// 		validComm.Slot(),
// 		prevComm.ID(),
// 		randomRoot(),
// 		validComm.CumulativeWeight(),
// 	)
// 	return forkedComm, nil
// }

// func randomCommitmentChain(currSlot slot.Slot) *commitment.Commitment {
// 	chain := make([]*commitment.Commitment, currSlot+1)
// 	chain[0] = commitment.NewEmptyCommitment()
// 	for i := slot.Slot(0); i < currSlot-1; i++ {
// 		prevComm := chain[i]
// 		newCommitment := commitment.New(
// 			i,
// 			prevComm.ID(),
// 			randomRoot(),
// 			100,
// 		)
// 		chain[i+1] = newCommitment
// 	}
// 	return chain[currSlot-1]
// }

// func randomRoot() [32]byte {
// 	data := make([]byte, 10)
// 	for i := range data {
// 		data[i] = byte(rand.Intn(256))
// 	}
// 	return sha256.Sum256(data)
// }
