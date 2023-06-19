package tpkg

import (
	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
	tpkg2 "github.com/iotaledger/iota.go/v4/tpkg"
)

type TestSuite struct {
	accounts      map[string]iotago.AccountID
	aliasAcocunts map[iotago.AccountID]string

	pubKeys map[string]ed25519.PublicKey
	outputs map[string]iotago.OutputID
}

func NewTestSuite() *TestSuite {
	return &TestSuite{
		accounts:      make(map[string]iotago.AccountID),
		aliasAcocunts: make(map[iotago.AccountID]string),
		pubKeys:       make(map[string]ed25519.PublicKey),
		outputs:       make(map[string]iotago.OutputID),
	}
}

func (t *TestSuite) GetAlias(accountID iotago.AccountID) (string, bool) {
	alias, exists := t.aliasAcocunts[accountID]

	return alias, exists
}

func (t *TestSuite) AccountID(alias string) iotago.AccountID {
	if accID, exists := t.accounts[alias]; exists {
		return accID
	}
	t.accounts[alias] = tpkg2.RandAccountID()
	t.aliasAcocunts[t.accounts[alias]] = alias

	return t.accounts[alias]
}

func (t *TestSuite) PublicKey(alias string) ed25519.PublicKey {
	if pubKey, exists := t.pubKeys[alias]; exists {
		return pubKey
	}
	t.pubKeys[alias] = utils.RandPubKey()

	return t.pubKeys[alias]
}

func (t *TestSuite) OutputID(alias string) iotago.OutputID {
	if outputID, exists := t.outputs[alias]; exists {
		return outputID
	}
	t.outputs[alias] = tpkg2.RandOutputID(1)

	return t.outputs[alias]
}

func (t *TestSuite) PubKeys(pubKeys []string) []ed25519.PublicKey {
	keys := make([]ed25519.PublicKey, len(pubKeys))
	for i, pubKey := range pubKeys {
		keys[i] = t.PublicKey(pubKey)
	}

	return keys
}

func (t *TestSuite) PubKeysSet(expectedkeys []string) *advancedset.AdvancedSet[ed25519.PublicKey] {
	pubKeys := advancedset.New[ed25519.PublicKey]()
	for _, pubKey := range expectedkeys {
		pubKeys.Add(t.PublicKey(pubKey))
	}

	return pubKeys
}
