package mana

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/core/safemath"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func TestManager_GetManaOnAccountOverflow(t *testing.T) {
	accountIDOverflow := tpkg.RandAccountID()
	accountIDValid := tpkg.RandAccountID()
	accountIDRecentBIC := tpkg.RandAccountID()
	accountIDRecentOutput := tpkg.RandAccountID()

	outputRetriever := func(id iotago.AccountID, _ iotago.SlotIndex) (*utxoledger.Output, error) {
		switch id {
		case accountIDOverflow:
			return utxoledger.CreateOutput(
				iotago.SingleVersionProvider(tpkg.ZeroCostTestAPI),
				iotago.OutputIDFromTransactionIDAndIndex(iotago.NewTransactionID(0, tpkg.Rand32ByteArray()), 0),
				tpkg.RandBlockID(),
				tpkg.RandSlot(),
				&iotago.AccountOutput{
					Amount:    iotago.MaxBaseToken / 2,
					Mana:      iotago.MaxMana/2 + iotago.MaxMana/4,
					AccountID: accountIDOverflow,
				},
				lo.PanicOnErr(iotago.NewOutputIDProof(tpkg.ZeroCostTestAPI, tpkg.Rand32ByteArray(), tpkg.RandSlot(), iotago.TxEssenceOutputs{tpkg.RandBasicOutput(iotago.AddressEd25519)}, 0)),
			), nil
		case accountIDRecentOutput:
			return utxoledger.CreateOutput(
				iotago.SingleVersionProvider(tpkg.ZeroCostTestAPI),
				iotago.OutputIDFromTransactionIDAndIndex(iotago.NewTransactionID(1, tpkg.Rand32ByteArray()), 0),
				tpkg.RandBlockID(),
				tpkg.RandSlot(),
				&iotago.AccountOutput{
					Amount:    iotago.MaxBaseToken / 2,
					Mana:      iotago.MaxMana / 2,
					AccountID: id,
				},
				lo.PanicOnErr(iotago.NewOutputIDProof(tpkg.ZeroCostTestAPI, tpkg.Rand32ByteArray(), tpkg.RandSlot(), iotago.TxEssenceOutputs{tpkg.RandBasicOutput(iotago.AddressEd25519)}, 0)),
			), nil
		default:
			return utxoledger.CreateOutput(
				iotago.SingleVersionProvider(tpkg.ZeroCostTestAPI),
				iotago.OutputIDFromTransactionIDAndIndex(iotago.NewTransactionID(0, tpkg.Rand32ByteArray()), 0),
				tpkg.RandBlockID(),
				tpkg.RandSlot(),
				&iotago.AccountOutput{
					Amount:    iotago.MaxBaseToken / 2,
					Mana:      iotago.MaxMana / 2,
					AccountID: id,
				},
				lo.PanicOnErr(iotago.NewOutputIDProof(tpkg.ZeroCostTestAPI, tpkg.Rand32ByteArray(), tpkg.RandSlot(), iotago.TxEssenceOutputs{tpkg.RandBasicOutput(iotago.AddressEd25519)}, 0)),
			), nil
		}
	}

	accountRetriever := func(id iotago.AccountID, index iotago.SlotIndex) (*accounts.AccountData, bool, error) {
		switch id {
		case accountIDRecentBIC:
			return &accounts.AccountData{
				ID: id,
				Credits: &accounts.BlockIssuanceCredits{
					Value:      iotago.MaxBlockIssuanceCredits/2 + iotago.MaxBlockIssuanceCredits/4,
					UpdateSlot: 1,
				},
				ExpirySlot:                            iotago.MaxSlotIndex,
				OutputID:                              iotago.OutputID{},
				BlockIssuerKeys:                       nil,
				ValidatorStake:                        0,
				DelegationStake:                       0,
				FixedCost:                             0,
				StakeEndEpoch:                         0,
				LatestSupportedProtocolVersionAndHash: model.VersionAndHash{},
			}, true, nil
		default:
			return &accounts.AccountData{
				ID: id,
				Credits: &accounts.BlockIssuanceCredits{
					Value: iotago.MaxBlockIssuanceCredits/2 + iotago.MaxBlockIssuanceCredits/4,
				},
				ExpirySlot:                            iotago.MaxSlotIndex,
				OutputID:                              iotago.OutputID{},
				BlockIssuerKeys:                       nil,
				ValidatorStake:                        0,
				DelegationStake:                       0,
				FixedCost:                             0,
				StakeEndEpoch:                         0,
				LatestSupportedProtocolVersionAndHash: model.VersionAndHash{},
			}, true, nil
		}
	}

	manager := NewManager(iotago.SingleVersionProvider(tpkg.ZeroCostTestAPI), outputRetriever, accountRetriever)
	manaDecayProvider := manager.apiProvider.LatestAPI().ManaDecayProvider()

	// The value for this account will overflow because component values are too big.
	{
		_, err := manager.GetManaOnAccount(accountIDOverflow, 0)
		require.ErrorIs(t, err, safemath.ErrIntegerOverflow)
	}

	// The value for this account should be correctly calculated and create an entry in cache.
	{
		mana, err := manager.GetManaOnAccount(accountIDValid, 0)
		require.NoError(t, err)

		require.EqualValues(t, iotago.MaxMana/2+iotago.Mana(iotago.MaxBlockIssuanceCredits/2+iotago.MaxBlockIssuanceCredits/4), mana)

		cachedMana, exists := manager.manaVectorCache.Get(accountIDValid)
		require.True(t, exists)
		require.EqualValues(t, mana, cachedMana.Value())
		require.EqualValues(t, 0, cachedMana.UpdateTime())

		// Make sure that the entry retrieved from the cache is the same as the previous one.
		mana, err = manager.GetManaOnAccount(accountIDValid, 0)
		require.NoError(t, err)

		require.EqualValues(t, iotago.MaxMana/2+iotago.Mana(iotago.MaxBlockIssuanceCredits/2+iotago.MaxBlockIssuanceCredits/4), mana)
	}

	// Make sure that mana is decayed correctly.
	{
		mana, err := manager.GetManaOnAccount(accountIDValid, 1)
		require.NoError(t, err)

		decayedMana, err := manaDecayProvider.ManaWithDecay(iotago.MaxMana/2+iotago.Mana(iotago.MaxBlockIssuanceCredits/2+iotago.MaxBlockIssuanceCredits/4), 0, 1)
		require.NoError(t, err)

		generatedMana, err := manaDecayProvider.ManaGenerationWithDecay(iotago.MaxBaseToken/2, 0, 1)
		require.NoError(t, err)

		require.EqualValues(t, decayedMana+generatedMana, mana)

		// Make sure that it's possible to ask for mana value at an earlier slot than the cached value.
		mana, err = manager.GetManaOnAccount(accountIDValid, 0)
		require.NoError(t, err)

		require.EqualValues(t, iotago.MaxMana/2+iotago.Mana(iotago.MaxBlockIssuanceCredits/2+iotago.MaxBlockIssuanceCredits/4), mana)
	}

	// Make sure that decaying BIC to match the output creation slot works.
	{
		mana, err := manager.GetManaOnAccount(accountIDRecentOutput, 2)
		require.NoError(t, err)

		decayedStoredMana, err := manaDecayProvider.ManaWithDecay(iotago.MaxMana/2, 1, 2)
		require.NoError(t, err)
		decayedBIC2Slots, err := manaDecayProvider.ManaWithDecay(iotago.Mana(iotago.MaxBlockIssuanceCredits/2+iotago.MaxBlockIssuanceCredits/4), 0, 2)
		require.NoError(t, err)

		generatedMana, err := manaDecayProvider.ManaGenerationWithDecay(iotago.MaxBaseToken/2, 1, 2)
		require.NoError(t, err)

		require.EqualValues(t, decayedBIC2Slots+decayedStoredMana+generatedMana, mana)

		decayedBIC1Slot, err := manaDecayProvider.ManaWithDecay(iotago.Mana(iotago.MaxBlockIssuanceCredits/2+iotago.MaxBlockIssuanceCredits/4), 0, 2)
		require.NoError(t, err)

		// Make sure that cache entry is for slot 1.
		cachedMana, exists := manager.manaVectorCache.Get(accountIDRecentOutput)
		require.True(t, exists)
		require.EqualValues(t, iotago.MaxMana/2+decayedBIC1Slot, cachedMana.Value())
		require.EqualValues(t, 1, cachedMana.UpdateTime())
	}

	// Make sure that decaying StoredMana to match the BIC update time works.
	{
		mana, err := manager.GetManaOnAccount(accountIDRecentBIC, 2)
		require.NoError(t, err)

		decayedStoredMana2Slots, err := manaDecayProvider.ManaWithDecay(iotago.MaxMana/2, 0, 0)
		require.NoError(t, err)

		generatedMana1Slot, err := manaDecayProvider.ManaGenerationWithDecay(iotago.MaxBaseToken/2, 0, 1)
		require.NoError(t, err)

		decayedBIC, err := manaDecayProvider.ManaWithDecay(iotago.Mana(iotago.MaxBlockIssuanceCredits/2+iotago.MaxBlockIssuanceCredits/4), 1, 1)
		require.NoError(t, err)

		// generatedMana1Slot is multiplied to calculate generation for two slots. It cannot be done in a single step
		// because the value does not match due to approximation errors inherent to the underlying calculation method.
		require.EqualValues(t, decayedStoredMana2Slots+generatedMana1Slot*2+decayedBIC, mana)

		decayedStoredMana1Slot, err := manaDecayProvider.ManaWithDecay(iotago.MaxMana/2, 0, 1)
		require.NoError(t, err)

		// Make sure that cache entry is for slot 1.
		cachedMana, exists := manager.manaVectorCache.Get(accountIDRecentBIC)
		require.True(t, exists)
		require.EqualValues(t, 1, cachedMana.UpdateTime())
		require.EqualValues(t, iotago.Mana(iotago.MaxBlockIssuanceCredits/2+iotago.MaxBlockIssuanceCredits/4)+decayedStoredMana1Slot+generatedMana1Slot, cachedMana.Value())
	}

}
