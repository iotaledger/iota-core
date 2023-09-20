package snapshotcreator

import (
	"os"

	"golang.org/x/crypto/blake2b"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/attestation/slotattestation"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag/inmemoryblockdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker/inmemorybooker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/clock/blocktime"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/commitmentfilter/accountsfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/congestioncontrol/scheduler/passthrough"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget/thresholdblockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/slotgadget/totalweightslotgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/blockfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/syncmanager/trivialsyncmanager"
	tipmanagerv1 "github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager/v1"
	tipselectionv1 "github.com/iotaledger/iota-core/pkg/protocol/engine/tipselection/v1"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/upgrade/signalingupgradeorchestrator"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1"
	"github.com/iotaledger/iota-core/pkg/retainer/retainer"
	"github.com/iotaledger/iota-core/pkg/storage"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

// CreateSnapshot creates a new snapshot. Genesis is defined by genesisTokenAmount and seedBytes, it
// is pledged to the node that is derived from the same seed. The amount to pledge to each node is defined by
// nodesToPledge map (publicKey->amount), the funds of each pledge is sent to the same publicKey.
// | Pledge      | Funds       |
// | ----------- | ----------- |
// | empty       | genesisSeed |
// | node1       | node1       |
// | node2       | node2       |.

var GenesisTransactionID = iotago.IdentifierFromData([]byte("genesis"))

func CreateSnapshot(opts ...options.Option[Options]) error {
	opt := NewOptions(opts...)

	errorHandler := func(err error) {
		panic(err)
	}

	workers := workerpool.NewGroup("CreateSnapshot")
	defer workers.Shutdown()
	s := storage.Create(lo.PanicOnErr(os.MkdirTemp(os.TempDir(), "*")), opt.DataBaseVersion, errorHandler)
	defer s.Shutdown()

	if err := s.Settings().StoreProtocolParametersForStartEpoch(opt.ProtocolParameters, 0); err != nil {
		return ierrors.Wrap(err, "failed to store the protocol parameters for epoch 0")
	}

	api := s.Settings().APIProvider().CurrentAPI()
	if err := s.Commitments().Store(model.NewEmptyCommitment(api)); err != nil {
		return ierrors.Wrap(err, "failed to store empty commitment")
	}

	accounts := account.NewAccounts()
	for _, accountData := range opt.Accounts {
		// Only add genesis validators if an account has both - StakedAmount and StakingEndEpoch - specified.
		if accountData.StakedAmount > 0 && accountData.StakingEpochEnd > 0 {
			blockIssuerKeyEd25519, ok := accountData.IssuerKey.(iotago.BlockIssuerKeyEd25519)
			if !ok {
				panic("block issuer key must be of type ed25519")
			}
			ed25519PubKey := blockIssuerKeyEd25519.ToEd25519PublicKey()
			accountID := blake2b.Sum256(ed25519PubKey[:])
			accounts.Set(accountID, &account.Pool{
				PoolStake:      accountData.StakedAmount,
				ValidatorStake: accountData.StakedAmount,
				FixedCost:      accountData.FixedCost,
			})
		}
	}

	engineInstance := engine.New(workers.CreateGroup("Engine"),
		errorHandler,
		s,
		blockfilter.NewProvider(),
		accountsfilter.NewProvider(),
		inmemoryblockdag.NewProvider(),
		inmemorybooker.NewProvider(),
		blocktime.NewProvider(),
		thresholdblockgadget.NewProvider(),
		totalweightslotgadget.NewProvider(),
		sybilprotectionv1.NewProvider(sybilprotectionv1.WithInitialCommittee(accounts)),
		slotnotarization.NewProvider(),
		slotattestation.NewProvider(),
		opt.LedgerProvider(),
		passthrough.NewProvider(),
		tipmanagerv1.NewProvider(),
		tipselectionv1.NewProvider(),
		retainer.NewProvider(),
		signalingupgradeorchestrator.NewProvider(),
		trivialsyncmanager.NewProvider(),
		engine.WithSnapshotPath(""), // magic to disable loading snapshot
	)
	defer engineInstance.Shutdown()

	for blockID, commitmentID := range opt.RootBlocks {
		engineInstance.EvictionState.AddRootBlock(blockID, commitmentID)
	}

	totalAccountAmount := lo.Reduce(opt.Accounts, func(accumulator iotago.BaseToken, details AccountDetails) iotago.BaseToken {
		return accumulator + details.Amount
	}, iotago.BaseToken(0))
	if err := createGenesisOutput(opt.ProtocolParameters.TokenSupply()-totalAccountAmount, opt.GenesisSeed, engineInstance); err != nil {
		return ierrors.Wrap(err, "failed to create genesis outputs")
	}

	if err := createGenesisAccounts(opt.Accounts, engineInstance); err != nil {
		return ierrors.Wrap(err, "failed to create genesis account outputs")
	}

	return engineInstance.WriteSnapshot(opt.FilePath)
}

func createGenesisOutput(genesisTokenAmount iotago.BaseToken, genesisSeed []byte, engineInstance *engine.Engine) error {
	if genesisTokenAmount > 0 {
		genesisWallet := mock.NewHDWallet("genesis", genesisSeed, 0)
		output := createOutput(genesisWallet.Address(), genesisTokenAmount)

		if _, err := engineInstance.CurrentAPI().ProtocolParameters().RentStructure().CoversMinDeposit(output, genesisTokenAmount); err != nil {
			return ierrors.Wrap(err, "min rent not covered by Genesis output with index 0")
		}

		// Genesis output is on Genesis TX index 0
		if err := engineInstance.Ledger.AddGenesisUnspentOutput(utxoledger.CreateOutput(engineInstance, iotago.OutputIDFromTransactionIDAndIndex(GenesisTransactionID, 0), iotago.EmptyBlockID(), 0, 0, output)); err != nil {
			return err
		}
	}

	return nil
}

func createGenesisAccounts(accounts []AccountDetails, engineInstance *engine.Engine) error {
	// Account outputs start from Genesis TX index 1
	for idx, genesisAccount := range accounts {
		output := createAccount(genesisAccount.AccountID, genesisAccount.Address, genesisAccount.Amount, genesisAccount.Mana, genesisAccount.IssuerKey, genesisAccount.ExpirySlot, genesisAccount.StakedAmount, genesisAccount.StakingEpochEnd, genesisAccount.FixedCost)

		if _, err := engineInstance.CurrentAPI().ProtocolParameters().RentStructure().CoversMinDeposit(output, genesisAccount.Amount); err != nil {
			return ierrors.Wrapf(err, "min rent not covered by account output with index %d", idx+1)
		}

		accountOutput := utxoledger.CreateOutput(engineInstance, iotago.OutputIDFromTransactionIDAndIndex(GenesisTransactionID, uint16(idx+1)), iotago.EmptyBlockID(), 0, 0, output)
		if err := engineInstance.Ledger.AddGenesisUnspentOutput(accountOutput); err != nil {
			return err
		}
		if err := engineInstance.Ledger.AddAccount(accountOutput, genesisAccount.BlockIssuanceCredits); err != nil {
			return err
		}
	}

	return nil
}

func createOutput(address iotago.Address, tokenAmount iotago.BaseToken) (output iotago.Output) {
	return &iotago.BasicOutput{
		Amount: tokenAmount,
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: address},
		},
	}
}

func createAccount(accountID iotago.AccountID, address iotago.Address, tokenAmount iotago.BaseToken, mana iotago.Mana, blockIssuerKey iotago.BlockIssuerKey, expirySlot iotago.SlotIndex, stakedAmount iotago.BaseToken, stakeEndEpoch iotago.EpochIndex, stakeFixedCost iotago.Mana) (output iotago.Output) {
	accountOutput := &iotago.AccountOutput{
		Amount:       tokenAmount,
		Mana:         mana,
		NativeTokens: iotago.NativeTokens{},
		AccountID:    accountID,
		Conditions: iotago.AccountOutputUnlockConditions{
			&iotago.StateControllerAddressUnlockCondition{Address: address},
			&iotago.GovernorAddressUnlockCondition{Address: address},
		},
		Features: iotago.AccountOutputFeatures{
			&iotago.BlockIssuerFeature{
				BlockIssuerKeys: iotago.BlockIssuerKeys{blockIssuerKey},
				ExpirySlot:      expirySlot,
			},
		},
		ImmutableFeatures: iotago.AccountOutputImmFeatures{},
	}

	// The Staking feature is only added if both StakedAmount and StakeEndEpoch have non-zero values,
	// otherwise the Staking feature state would be invalid.
	if stakedAmount > 0 && stakeEndEpoch > 0 {
		accountOutput.Features = append(accountOutput.Features, &iotago.StakingFeature{
			StakedAmount: stakedAmount,
			FixedCost:    stakeFixedCost,
			StartEpoch:   0,
			EndEpoch:     stakeEndEpoch,
		})
	}

	return accountOutput
}
