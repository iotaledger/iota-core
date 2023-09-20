package accountsfilter

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/commitmentfilter"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type TestFramework struct {
	Test             *testing.T
	CommitmentFilter *CommitmentFilter

	commitments map[iotago.SlotIndex]*model.Commitment
	accountData map[iotago.AccountID]*accounts.AccountData
	rmcData     map[iotago.SlotIndex]iotago.Mana

	apiProvider iotago.APIProvider
}

func NewTestFramework(t *testing.T, apiProvider iotago.APIProvider, optsFilter ...options.Option[CommitmentFilter]) *TestFramework {
	tf := &TestFramework{
		Test:        t,
		apiProvider: apiProvider,
		commitments: make(map[iotago.SlotIndex]*model.Commitment),
		accountData: make(map[iotago.AccountID]*accounts.AccountData),
		rmcData:     make(map[iotago.SlotIndex]iotago.Mana),
	}
	tf.CommitmentFilter = New(apiProvider, optsFilter...)

	tf.CommitmentFilter.commitmentFunc = func(slotIndex iotago.SlotIndex) (*model.Commitment, error) {
		if commitment, ok := tf.commitments[slotIndex]; ok {
			return commitment, nil
		}
		return nil, ierrors.Errorf("no commitment available for slot index %d", slotIndex)
	}

	tf.CommitmentFilter.accountRetrieveFunc = func(accountID iotago.AccountID, targetIndex iotago.SlotIndex) (*accounts.AccountData, bool, error) {
		if accountData, ok := tf.accountData[accountID]; ok {
			return accountData, true, nil
		}
		return nil, false, ierrors.Errorf("no account data available for account id %s", accountID)
	}

	tf.CommitmentFilter.rmcRetrieveFunc = func(slotIndex iotago.SlotIndex) (iotago.Mana, error) {
		if rmc, ok := tf.rmcData[slotIndex]; ok {
			return rmc, nil
		}
		return iotago.Mana(0), ierrors.Errorf("no rmc available for slot index %d", slotIndex)
	}

	tf.CommitmentFilter.events.BlockAllowed.Hook(func(block *blocks.Block) {
		t.Logf("BlockAllowed: %s", block.ID())
	})

	tf.CommitmentFilter.events.BlockFiltered.Hook(func(event *commitmentfilter.BlockFilteredEvent) {
		t.Logf("BlockFiltered: %s - %s", event.Block.ID(), event.Reason)
	})

	return tf
}

func (t *TestFramework) AddCommitment(slotIndex iotago.SlotIndex, commitment *model.Commitment) {
	t.commitments[slotIndex] = commitment
}

func (t *TestFramework) AddAccountData(accountID iotago.AccountID, accountData *accounts.AccountData) {
	t.accountData[accountID] = accountData
}

func (t *TestFramework) AddRMCData(slotIndex iotago.SlotIndex, rmcData iotago.Mana) {
	t.rmcData[slotIndex] = rmcData
}

// q: how to get an engine block.Block from protocol block

func (t *TestFramework) processBlock(alias string, block *iotago.ProtocolBlock) {
	apiForVersion, err := t.apiProvider.APIForVersion(block.ProtocolVersion)
	require.NoError(t.Test, err)

	modelBlock, err := model.BlockFromBlock(block, apiForVersion)
	require.NoError(t.Test, err)

	modelBlock.ID().RegisterAlias(alias)
	t.CommitmentFilter.ProcessPreFilteredBlock(blocks.NewBlock(modelBlock))
}

func (t *TestFramework) processBlockWithAPI(alias string, block *iotago.ProtocolBlock, api iotago.API) {
	modelBlock, err := model.BlockFromBlock(block, api)
	require.NoError(t.Test, err)

	modelBlock.ID().RegisterAlias(alias)
	t.CommitmentFilter.ProcessPreFilteredBlock(blocks.NewBlock(modelBlock))
}

func (t *TestFramework) IssueSignedBlockAtSlot(alias string, slot iotago.SlotIndex, commitmentID iotago.SlotIdentifier, keyPair ed25519.KeyPair) {
	apiForSlot := t.apiProvider.APIForSlot(slot)

	addr := iotago.Ed25519AddressFromPubKey(keyPair.PublicKey[:])
	block, err := builder.NewBasicBlockBuilder(apiForSlot).
		StrongParents(iotago.BlockIDs{}).
		IssuingTime(apiForSlot.TimeProvider().SlotStartTime(slot)).
		SlotCommitmentID(commitmentID).
		Sign(iotago.AccountID(addr[:]), keyPair.PrivateKey[:]).
		Build()
	require.NoError(t.Test, err)

	t.processBlock(alias, block)
}

func (t *TestFramework) IssueSignedBlockAtSlotWithBurnedMana(alias string, slot iotago.SlotIndex, commitmentID iotago.SlotIdentifier, keyPair ed25519.KeyPair, burnedMana iotago.Mana) {
	apiForSlot := t.apiProvider.APIForSlot(slot)

	addr := iotago.Ed25519AddressFromPubKey(keyPair.PublicKey[:])
	block, err := builder.NewBasicBlockBuilder(apiForSlot).
		StrongParents(iotago.BlockIDs{}).
		IssuingTime(apiForSlot.TimeProvider().SlotStartTime(slot)).
		SlotCommitmentID(commitmentID).
		BurnedMana(burnedMana).
		Sign(iotago.AccountID(addr[:]), keyPair.PrivateKey[:]).
		Build()
	require.NoError(t.Test, err)

	t.processBlock(alias, block)
}

func TestCommitmentFilter_NoAccount(t *testing.T) {
	testAPI := tpkg.TestAPI

	tf := NewTestFramework(t,
		api.SingleVersionProvider(testAPI),
	)

	tf.CommitmentFilter.events.BlockAllowed.Hook(func(block *blocks.Block) {
		require.NotEqual(t, "noAccount", block.ID().Alias())
	})

	tf.CommitmentFilter.events.BlockFiltered.Hook(func(event *commitmentfilter.BlockFilteredEvent) {
		require.NotEqual(t, "withAccount", event.Block.ID().Alias())
	})

	keyPair := ed25519.GenerateKeyPair()
	currentSlot := iotago.SlotIndex(100)
	currentAPI := tf.apiProvider.CurrentAPI()

	commitment := iotago.NewCommitment(currentAPI.Version(), currentSlot-currentAPI.ProtocolParameters().MinCommittableAge(), iotago.CommitmentID{}, iotago.Identifier{}, 0, 0)
	modelCommitment, err := model.CommitmentFromCommitment(commitment, currentAPI)
	commitmentID := commitment.MustID()

	require.NoError(t, err)
	tf.AddCommitment(commitment.Index, modelCommitment)

	addr := iotago.Ed25519AddressFromPubKey(keyPair.PublicKey[:])
	accountID := iotago.AccountID(addr[:])

	// register the account in the proxy account manager
	tf.AddAccountData(
		accountID,
		accounts.NewAccountData(
			accountID,
			accounts.WithExpirySlot(math.MaxUint64),
			accounts.WithBlockIssuerKeys(iotago.BlockIssuerKeyEd25519FromPublicKey(keyPair.PublicKey)),
		),
	)

	tf.AddRMCData(currentSlot-currentAPI.ProtocolParameters().MaxCommittableAge(), iotago.Mana(0))

	tf.IssueSignedBlockAtSlot("withAccount", currentSlot, commitmentID, keyPair)

	otherKeyPair := ed25519.GenerateKeyPair()
	tf.IssueSignedBlockAtSlot("noAccount", currentSlot, commitmentID, otherKeyPair)
}

func TestCommitmentFilter_BurnedMana(t *testing.T) {
	testAPI := tpkg.TestAPI

	tf := NewTestFramework(t,
		api.SingleVersionProvider(testAPI),
	)

	tf.CommitmentFilter.events.BlockAllowed.Hook(func(block *blocks.Block) {
		require.NotEqual(t, "insuffientBurnedMana", block.ID().Alias())
	})

	tf.CommitmentFilter.events.BlockFiltered.Hook(func(event *commitmentfilter.BlockFilteredEvent) {
		require.NotEqual(t, "sufficientBurnedMana", event.Block.ID().Alias())
	})

	keyPair := ed25519.GenerateKeyPair()
	currentSlot := iotago.SlotIndex(100)
	currentAPI := tf.apiProvider.CurrentAPI()

	commitment := iotago.NewCommitment(currentAPI.Version(), currentSlot-currentAPI.ProtocolParameters().MinCommittableAge(), iotago.CommitmentID{}, iotago.Identifier{}, 0, 0)
	modelCommitment, err := model.CommitmentFromCommitment(commitment, currentAPI)
	commitmentID := commitment.MustID()

	require.NoError(t, err)
	tf.AddCommitment(commitment.Index, modelCommitment)

	addr := iotago.Ed25519AddressFromPubKey(keyPair.PublicKey[:])
	accountID := iotago.AccountID(addr[:])

	// register the account in the proxy account manager
	tf.AddAccountData(
		accountID,
		accounts.NewAccountData(
			accountID,
			accounts.WithExpirySlot(math.MaxUint64),
			accounts.WithBlockIssuerKeys(iotago.BlockIssuerKeyEd25519FromPublicKey(keyPair.PublicKey)),
		),
	)

	tf.AddRMCData(currentSlot-currentAPI.ProtocolParameters().MaxCommittableAge(), iotago.Mana(10))

	tf.IssueSignedBlockAtSlotWithBurnedMana("sufficientBurnedMana", currentSlot, commitmentID, keyPair, iotago.Mana(10))
	tf.IssueSignedBlockAtSlotWithBurnedMana("sufficientBurnedMana", currentSlot, commitmentID, keyPair, iotago.Mana(11))

	tf.IssueSignedBlockAtSlotWithBurnedMana("insuffientBurnedMana", currentSlot, commitmentID, keyPair, iotago.Mana(9))
}

func TestCommitmentFilter_Expiry(t *testing.T) {
	testAPI := tpkg.TestAPI

	tf := NewTestFramework(t,
		api.SingleVersionProvider(testAPI),
	)

	tf.CommitmentFilter.events.BlockAllowed.Hook(func(block *blocks.Block) {
		require.NotEqual(t, "expired", block.ID().Alias())
	})

	tf.CommitmentFilter.events.BlockFiltered.Hook(func(event *commitmentfilter.BlockFilteredEvent) {
		require.Equal(t, "expired", event.Block.ID().Alias())
	})

	// register an account in the proxy account manager
	// with expiry slot 100
	keyPair := ed25519.GenerateKeyPair()
	addr := iotago.Ed25519AddressFromPubKey(keyPair.PublicKey[:])
	accountID := iotago.AccountID(addr[:])
	tf.AddAccountData(
		accountID,
		accounts.NewAccountData(
			accountID,
			accounts.WithExpirySlot(100),
			accounts.WithBlockIssuerKeys(iotago.BlockIssuerKeyEd25519FromPublicKey(keyPair.PublicKey)),
		),
	)

	// create a commitment for slot 90
	currentAPI := tf.apiProvider.CurrentAPI()
	commitmentSlot := iotago.SlotIndex(90)
	currentSlot := commitmentSlot + currentAPI.ProtocolParameters().MinCommittableAge()
	commitment := iotago.NewCommitment(currentAPI.Version(), commitmentSlot, iotago.CommitmentID{}, iotago.Identifier{}, 0, 0)
	modelCommitment, err := model.CommitmentFromCommitment(commitment, currentAPI)
	commitmentID := commitment.MustID()
	require.NoError(t, err)
	// add the commitment and 0 RMC to the proxy state
	tf.AddCommitment(commitment.Index, modelCommitment)
	tf.AddRMCData(currentSlot-currentAPI.ProtocolParameters().MaxCommittableAge(), iotago.Mana(0))

	tf.IssueSignedBlockAtSlot("correct", currentSlot, commitmentID, keyPair)

	// create a commitment for slot 100
	commitmentSlot = iotago.SlotIndex(100)
	currentSlot = commitmentSlot + currentAPI.ProtocolParameters().MinCommittableAge()
	commitment = iotago.NewCommitment(currentAPI.Version(), commitmentSlot, iotago.CommitmentID{}, iotago.Identifier{}, 0, 0)
	modelCommitment, err = model.CommitmentFromCommitment(commitment, currentAPI)
	commitmentID = commitment.MustID()
	require.NoError(t, err)
	// add the commitment and 0 RMC to the proxy state
	tf.AddCommitment(commitment.Index, modelCommitment)
	tf.AddRMCData(currentSlot-currentAPI.ProtocolParameters().MaxCommittableAge(), iotago.Mana(0))

	tf.IssueSignedBlockAtSlot("almostExpired", currentSlot, commitmentID, keyPair)

	// create a commitment for slot 110
	commitmentSlot = iotago.SlotIndex(101)
	currentSlot = commitmentSlot + currentAPI.ProtocolParameters().MinCommittableAge()
	commitment = iotago.NewCommitment(currentAPI.Version(), commitmentSlot, iotago.CommitmentID{}, iotago.Identifier{}, 0, 0)
	modelCommitment, err = model.CommitmentFromCommitment(commitment, currentAPI)
	commitmentID = commitment.MustID()
	require.NoError(t, err)
	// add the commitment and 0 RMC to the proxy state
	tf.AddCommitment(commitment.Index, modelCommitment)
	tf.AddRMCData(currentSlot-currentAPI.ProtocolParameters().MaxCommittableAge(), iotago.Mana(0))

	tf.IssueSignedBlockAtSlot("expired", currentSlot, commitmentID, keyPair)
}
