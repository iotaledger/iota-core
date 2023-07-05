package p2p

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/autopeering/peer"
	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/rocksdb"
	"github.com/iotaledger/iota-core/pkg/storage/database"
)

// ErrMismatchedPrivateKeys is returned when the private key derived from the config does not correspond to the private
// key stored in an already existing peer database.
var ErrMismatchedPrivateKeys = errors.New("private key derived from the seed defined in the config does not correspond with the already stored private key in the database")

// checks whether the seed from the cfg corresponds to the one in the peer database.
func checkCfgSeedAgainstDB(cfgSeed []byte, peerDB *peer.DB) error {
	prvKeyDB, err := peerDB.LocalPrivateKey()
	if err != nil {
		return errors.Wrap(err, "unable to retrieve private key from peer database")
	}
	prvKeyDBBytes, err := prvKeyDB.Bytes()
	if err != nil {
		return err
	}
	prvKeyCfg := ed25519.PrivateKeyFromSeed(cfgSeed)
	prvKeyCfgBytes, err := prvKeyCfg.Bytes()
	if err != nil {
		return err
	}

	if !bytes.Equal(prvKeyCfgBytes, prvKeyDBBytes) {
		return errors.WithMessagef(ErrMismatchedPrivateKeys, "identities - pub keys (cfg/db): %s vs. %s", prvKeyCfg.Public(), prvKeyDB.Public())
	}

	return nil
}

func readPeerIP() (net.IP, error) {
	if strings.ToLower(ParamsP2P.ExternalAddress) == "auto" {
		// let the autopeering discover the IP
		return net.IPv4zero, nil
	}

	peeringIP := net.ParseIP(ParamsP2P.ExternalAddress)
	if peeringIP == nil {
		return nil, errors.Errorf("invalid IP address: %s", ParamsP2P.ExternalAddress)
	}

	return peeringIP, nil
}

// inits the peer database, returns a bool indicating whether the database is new.
func initPeerDB() (peerDB *peer.DB, peerDBKVStore kvstore.KVStore, isNewDB bool, err error) {
	if err = checkValidPeerDBPath(); err != nil {
		return nil, nil, false, errors.Wrap(err, "invalid peer database path")
	}

	if isNewDB, err = isPeerDBNew(); err != nil {
		return nil, nil, false, errors.Wrap(err, "unable to check whether peer database is new")
	}

	db, err := database.NewRocksDB(ParamsP2P.PeerDBDirectory)
	if err != nil {
		return nil, nil, false, errors.Wrap(err, "error creating peer database")
	}

	peerDBKVStore = rocksdb.New(db)

	if peerDB, err = peer.NewDB(peerDBKVStore); err != nil {
		return nil, nil, false, errors.Wrap(err, "error creating peer database")
	}

	if db == nil {
		return nil, nil, false, errors.New("couldn't create peer database; nil")
	}

	return
}

// checks whether the peer database is new by examining whether the directory
// exists or whether it contains any files.
func isPeerDBNew() (bool, error) {
	var isNewDB bool
	fileInfo, err := os.Stat(ParamsP2P.PeerDBDirectory)
	switch {
	case fileInfo != nil:
		files, readDirErr := os.ReadDir(ParamsP2P.PeerDBDirectory)
		if readDirErr != nil {
			return false, errors.Wrap(readDirErr, "unable to check whether peer database is empty")
		}
		if len(files) != 0 {
			break
		}

		fallthrough
	case os.IsNotExist(err):
		isNewDB = true
	}

	return isNewDB, nil
}

// checks that the peer database path does not reside within the main database directory.
func checkValidPeerDBPath() error {
	_, err := filepath.Abs(ParamsP2P.PeerDBDirectory)
	if err != nil {
		return errors.Wrapf(err, "cannot resolve absolute path of %s", ParamsP2P.PeerDBDirectory)
	}

	return nil
}

func readSeedFromCfg() ([]byte, error) {
	var seedBytes []byte
	var err error

	seedBytes, err = hexutil.Decode(ParamsP2P.Seed)
	if err != nil {
		return nil, errors.Wrap(err, "invalid seed")
	}

	if l := len(seedBytes); l != ed25519.SeedSize {
		return nil, errors.Errorf("invalid seed length: %d, need %d", l, ed25519.SeedSize)
	}

	return seedBytes, nil
}
