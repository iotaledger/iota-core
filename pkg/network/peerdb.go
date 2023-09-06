package network

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/kvstore"
)

const (
	// remove peers from DB, when the last received ping was older than this.
	peerExpiration = 30 * 24 * time.Hour
	// interval in which expired peers are checked.
	cleanupInterval = time.Hour
	// number of peers used for bootstrapping.
	seedCount = 10
)

// DB is the peer database, storing previously seen peers and any collected properties of them.
type DB struct {
	store     kvstore.KVStore
	expirerWg sync.WaitGroup
	quit      chan struct{} // Channel to signal the expiring thread to stop
}

// Keys in the node database.
const (
	dbNodePrefix = "n:" // Identifier to prefix node entries with

	dbNodeUpdated = "updated"
)

// NewDB creates a new peer database.
func NewDB(store kvstore.KVStore) *DB {
	pDB := &DB{
		store: store,
		quit:  make(chan struct{}),
	}

	pDB.expirerWg.Add(1)
	go pDB.expirer()

	return pDB
}

// UpdatePeer updates a peer in the database.
func (db *DB) UpdatePeer(p *Peer) error {
	data, err := p.Bytes()
	if err != nil {
		return err
	}

	if err := db.store.Set(nodeKey(p.ID), data); err != nil {
		return err
	}

	if err := db.setInt64(nodeFieldKey(p.ID, dbNodeUpdated), time.Now().Unix()); err != nil {
		return err
	}

	return db.store.Flush()
}

// Peer retrieves a peer from the database.
func (db *DB) Peer(id peer.ID) (*Peer, error) {
	data, err := db.store.Get(nodeKey(id))
	if err != nil {
		return nil, err
	}

	return peerFromBytes(data)
}

// SeedPeers retrieves random nodes to be used as potential bootstrap peers.
func (db *DB) SeedPeers() []*Peer {
	return randomSubset(db.getPeers(), seedCount)
}

// Close closes the peer database.
func (db *DB) Close() {
	close(db.quit)
	db.expirerWg.Wait()
}

// expirer should be started in a go routine, and is responsible for looping ad
// infinitum and dropping stale data from the database.
func (db *DB) expirer() {
	tick := time.NewTicker(cleanupInterval)
	defer tick.Stop()
	defer db.expirerWg.Done()

	for {
		select {
		case <-tick.C:
			_ = db.expireNodes()
		case <-db.quit:
			return
		}
	}
}

// expireNodes iterates over the database and deletes all nodes that have not
// been seen (i.e. received a pong from) for some time.
func (db *DB) expireNodes() error {
	threshold := time.Now().Add(-peerExpiration).Unix()
	batchedMuts, err := db.store.Batched()
	if err != nil {
		return err
	}

	var innerErr error
	if err := db.store.Iterate(kvstore.KeyPrefix(dbNodePrefix), func(key kvstore.Key, value kvstore.Value) bool {
		if bytes.HasSuffix(key, []byte(dbNodeUpdated)) {
			// if the peer has been updated before the threshold we expire it
			if parseInt64(value) < threshold {
				// delete update field of the peer
				if err := batchedMuts.Delete(key); err != nil {
					innerErr = err

					return false
				}

				// delete peer
				if err := batchedMuts.Delete(key[:len(key)-len(dbNodeUpdated)]); err != nil {
					innerErr = err

					return false
				}
			}
		}

		return true
	}); err != nil {
		batchedMuts.Cancel()

		return err
	}

	if innerErr != nil {
		batchedMuts.Cancel()

		return innerErr
	}

	if err := batchedMuts.Commit(); err != nil {
		return err
	}

	return db.store.Flush()
}

func (db *DB) getPeers() (peers []*Peer) {
	if err := db.store.Iterate(kvstore.KeyPrefix(dbNodePrefix), func(key kvstore.Key, value kvstore.Value) bool {
		// skip update fields
		if bytes.HasSuffix(key, []byte(dbNodeUpdated)) {
			return true
		}

		if p, err := peerFromBytes(value); err == nil {
			peers = append(peers, p)
		}

		return true
	}); err != nil {
		return nil
	}

	return peers
}

// setInt64 stores an integer in the given key.
func (db *DB) setInt64(key []byte, n int64) error {
	blob := make([]byte, binary.MaxVarintLen64)
	blob = blob[:binary.PutVarint(blob, n)]

	return db.store.Set(key, blob)
}

// nodeKey returns the database key for a node record.
func nodeKey(id peer.ID) []byte {
	return append([]byte(dbNodePrefix), []byte(id)...)
}

// nodeFieldKey returns the database key for a node metadata field.
func nodeFieldKey(id peer.ID, field string) []byte {
	return append(nodeKey(id), []byte(field)...)
}

func parseInt64(blob []byte) int64 {
	val, read := binary.Varint(blob)
	if read <= 0 {
		return 0
	}

	return val
}

func randomSubset(peers []*Peer, m int) []*Peer {
	if len(peers) <= m {
		return peers
	}

	result := make([]*Peer, 0, m)
	for i, p := range peers {
		//nolint:gosec // we do not care about weak random numbers here
		if rand.Intn(len(peers)-i) < m-len(result) {
			result = append(result, p)
		}
	}

	return result
}
