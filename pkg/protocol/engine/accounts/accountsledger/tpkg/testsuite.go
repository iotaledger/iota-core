package tpkg

import (
	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
	tpkg2 "github.com/iotaledger/iota.go/v4/tpkg"
)

type TestSuite struct {
	accounts map[string]iotago.AccountID
	pubKeys  map[string]ed25519.PublicKey
}

func NewTestSuite() *TestSuite {
	return &TestSuite{
		accounts: make(map[string]iotago.AccountID),
		pubKeys:  make(map[string]ed25519.PublicKey),
	}
}

func (t *TestSuite) AccountID(alias string) iotago.AccountID {
	if accID, exists := t.accounts[alias]; exists {
		return accID
	}
	t.accounts[alias] = tpkg2.RandAccountID()
	return t.accounts[alias]
}

func (t *TestSuite) PublicKey(alias string) ed25519.PublicKey {
	if pubKey, exists := t.pubKeys[alias]; exists {
		return pubKey
	}
	t.pubKeys[alias] = utils.RandPubKey()
	return t.pubKeys[alias]
}
