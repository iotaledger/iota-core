package spammer

// import (
// 	"time"

// 	iotago "github.com/iotaledger/iota.go/v4"
// 	"github.com/iotaledger/iota-core/tools/evil-spammer/wallet"
// )

// // region ClockSync //////////////evilspammerpkg/////////////////////////////////////////////////////////////////////////////////

// // ClockSync is used to synchronize with connected nodes.
// type ClockSync struct {
// 	LatestCommittedSlotClock *SlotClock

// 	syncTicker *time.Ticker
// 	clt        wallet.Client
// }

// func NewClockSync(slotDuration time.Duration, syncInterval time.Duration, clientList wallet.Client) *ClockSync {
// 	updateTicker := time.NewTicker(syncInterval)
// 	return &ClockSync{
// 		LatestCommittedSlotClock: &SlotClock{slotDuration: slotDuration},

// 		syncTicker: updateTicker,
// 		clt:        clientList,
// 	}
// }

// // Start starts the clock synchronization in the background after the first sync is done..
// func (c *ClockSync) Start() {
// 	c.Synchronize()
// 	go func() {
// 		for range c.syncTicker.C {
// 			c.Synchronize()
// 		}
// 	}()
// }

// func (c *ClockSync) Shutdown() {
// 	c.syncTicker.Stop()
// }

// func (c *ClockSync) Synchronize() {
// 	si, err := c.clt.GetLatestCommittedSlot()
// 	if err != nil {
// 		return
// 	}
// 	c.LatestCommittedSlotClock.Update(si)
// }

// // endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////

// // region SlotClock ///////////////////////////////////////////////////////////////////////////////////////////////

// type SlotClock struct {
// 	lastUpdated time.Time
// 	updatedSlot slot.Slot

// 	slotDuration time.Duration
// }

// func (c *SlotClock) Update(value slot.Slot) {
// 	c.lastUpdated = time.Now()
// 	c.updatedSlot = value
// }

// func (c *SlotClock) Get() slot.Slot {
// 	return c.updatedSlot
// }

// func (c *SlotClock) GetRelative() slot.Slot {
// 	return c.updatedSlot + slot.Slot(time.Since(c.lastUpdated)/c.slotDuration)
// }

// // endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////
