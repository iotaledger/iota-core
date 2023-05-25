package accountpubkeys

import (
	"crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Manager struct {
	pubKeysMap shrinkingmap.ShrinkingMap[iotago.SlotIndex, *AccountPubKeys]
}

type AccountPubKeys struct {
}

func (m *Manager) IsPublicKeyAllowed(iotago.AccountID, iotago.SlotIndex, ed25519.PublicKey) bool {
	return false
}

type PubKeyDiff struct {
	index   iotago.SlotIndex
	added   map[iotago.AccountID]ed25519.PublicKey
	removed map[iotago.AccountID]ed25519.PublicKey
}

func (m *Manager) ApplyDiff(pubKeyDiff *PubKeyDiff) {
	//for accountID, pubKey := range pubKeyDiff.added {
	//
	//}
	//for _, pubKey := range pubKeyDiff.removed {
	//}
}

func (m *Manager) addPublicKey(pubKey ed25519.PublicKey) {
}

func (m *Manager) removePublicKey(pubKey ed25519.PublicKey) {
}
