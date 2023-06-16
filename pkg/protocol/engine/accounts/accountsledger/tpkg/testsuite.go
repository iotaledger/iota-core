package tpkg

import (
	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
	tpkg2 "github.com/iotaledger/iota.go/v4/tpkg"
)

type TestSuite struct {
	accounts      map[string]iotago.AccountID
	aliasAcocunts map[iotago.AccountID]string

	pubKeys map[string]ed25519.PublicKey
}

type AccountMetadata struct {
	LastUpdated      iotago.SlotIndex
	PrevLastUpdated  iotago.SlotIndex
	LastOutputID     iotago.OutputID
	PrevLastOutputID iotago.OutputID
}

func NewTestSuite() *TestSuite {
	return &TestSuite{
		accounts:      make(map[string]iotago.AccountID),
		aliasAcocunts: make(map[iotago.AccountID]string),
		pubKeys:       make(map[string]ed25519.PublicKey),
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

// updateActions updated metadata for the account with the given alias.
func (t *TestSuite) updateActions(accountID iotago.AccountID, index iotago.SlotIndex, actions *AccountActions, prevActions *AccountActions) {
	alias, exists := t.GetAlias(accountID)
	if !exists {
		panic("account does not exist in this Scenario, cannot update the metadata")
	}
	if _, exists = t.accounts[alias]; !exists {
		panic("account does not exist in this Scenario, cannot update the metadata")
	}

	// we already committed in the past, so we replace past values with the current ones

	if prevActions != nil {
		actions.prevUpdatedTime = prevActions.updatedTime
		actions.prevOutputID = prevActions.outputID
	}

	actions.outputID = tpkg2.RandOutputID(1)
	actions.updatedTime = index
}
