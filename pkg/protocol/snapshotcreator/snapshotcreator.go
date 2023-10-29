package snapshotcreator

import (
	"os"

	"golang.org/x/crypto/blake2b"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
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

const (
	GenesisTransactionCreationSlot = 0
)

var GenesisTransactionCommitment = iotago.IdentifierFromData([]byte("genesis"))

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

	api := s.Settings().APIProvider().CommittedAPI()
	if err := s.Commitments().Store(model.NewEmptyCommitment(api)); err != nil {
		return ierrors.Wrap(err, "failed to store empty commitment")
	}

	accounts := account.NewAccounts()
	for _, accountData := range opt.Accounts {
		// Only add genesis validators if an account has both - StakedAmount and StakingEndEpoch - specified.
		if accountData.StakedAmount > 0 && accountData.StakingEpochEnd > 0 {
			blockIssuerKeyEd25519, ok := accountData.IssuerKey.(*iotago.Ed25519PublicKeyBlockIssuerKey)
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

	engineInstance := engine.New(
		log.NewLogger("snapshot-creator"),
		workers.CreateGroup("Engine"),
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
	defer engineInstance.Shutdown.Trigger()

	for blockID, commitmentID := range opt.RootBlocks {
		engineInstance.EvictionState.AddRootBlock(blockID, commitmentID)
	}

	totalAccountAmount := lo.Reduce(opt.Accounts, func(accumulator iotago.BaseToken, details AccountDetails) iotago.BaseToken {
		return accumulator + details.Amount
	}, iotago.BaseToken(0))

	totalBasicOutputAmount := lo.Reduce(opt.BasicOutputs, func(accumulator iotago.BaseToken, details BasicOutputDetails) iotago.BaseToken {
		return accumulator + details.Amount
	}, iotago.BaseToken(0))

	var genesisTransactionOutputs iotago.TxEssenceOutputs
	genesisOutput, err := createGenesisOutput(api, opt.ProtocolParameters.TokenSupply()-totalAccountAmount-totalBasicOutputAmount, opt.GenesisSeed)
	if err != nil {
		return ierrors.Wrap(err, "failed to create genesis outputs")
	}
	genesisTransactionOutputs = append(genesisTransactionOutputs, genesisOutput)

	accountOutputs, err := createGenesisAccounts(api, opt.Accounts)
	if err != nil {
		return ierrors.Wrap(err, "failed to create genesis account outputs")
	}
	genesisTransactionOutputs = append(genesisTransactionOutputs, accountOutputs...)

	genesisBasicOutputs, err := createGenesisBasicOutputs(api, opt.BasicOutputs)
	if err != nil {
		return ierrors.Wrap(err, "failed to create genesis basic outputs")
	}
	genesisTransactionOutputs = append(genesisTransactionOutputs, genesisBasicOutputs...)

	var accountLedgerOutputs utxoledger.Outputs
	for idx, output := range genesisTransactionOutputs {
		proof, err := iotago.NewOutputIDProof(engineInstance.LatestAPI(), GenesisTransactionCommitment, GenesisTransactionCreationSlot, genesisTransactionOutputs, uint16(idx))
		if err != nil {
			return err
		}

		// Generate the OutputID from the proof since we don't have a transaction per se.
		outputID, err := proof.OutputID(output)
		if err != nil {
			return err
		}

		utxoOutput := utxoledger.CreateOutput(engineInstance, outputID, iotago.EmptyBlockID, GenesisTransactionCreationSlot, output, proof)
		if err := engineInstance.Ledger.AddGenesisUnspentOutput(utxoOutput); err != nil {
			return err
		}

		if output.Type() == iotago.OutputAccount {
			accountLedgerOutputs = append(accountLedgerOutputs, utxoOutput)
		}
	}

	if len(accountLedgerOutputs) != len(opt.Accounts) {
		return ierrors.Errorf("failed to create genesis account outputs. Expected %d and got %d", len(opt.Accounts), len(accountLedgerOutputs))
	}

	for idx, accountLedgerOutput := range accountLedgerOutputs {
		if err := engineInstance.Ledger.AddAccount(accountLedgerOutput, opt.Accounts[idx].BlockIssuanceCredits); err != nil {
			return err
		}
	}

	return engineInstance.WriteSnapshot(opt.FilePath)
}

func createGenesisOutput(api iotago.API, genesisTokenAmount iotago.BaseToken, genesisSeed []byte) (iotago.Output, error) {
	if genesisTokenAmount > 0 {
		genesisWallet := mock.NewHDWallet("genesis", genesisSeed, 0)
		output := createOutput(genesisWallet.Address(), genesisTokenAmount, 0)

		if _, err := api.StorageScoreStructure().CoversMinDeposit(output, genesisTokenAmount); err != nil {
			return nil, ierrors.Wrap(err, "min rent not covered by Genesis output with index 0")
		}

		return output, nil
	}

	return nil, nil
}

func createGenesisAccounts(api iotago.API, accounts []AccountDetails) (iotago.TxEssenceOutputs, error) {
	var outputs iotago.TxEssenceOutputs
	// Account outputs start from Genesis TX index 1
	for idx, genesisAccount := range accounts {
		output := createAccount(genesisAccount.AccountID, genesisAccount.Address, genesisAccount.Amount, genesisAccount.Mana, genesisAccount.IssuerKey, genesisAccount.ExpirySlot, genesisAccount.StakedAmount, genesisAccount.StakingEpochEnd, genesisAccount.FixedCost)

		if _, err := api.StorageScoreStructure().CoversMinDeposit(output, genesisAccount.Amount); err != nil {
			return nil, ierrors.Wrapf(err, "min rent not covered by account output with index %d", idx+1)
		}

		outputs = append(outputs, output)
	}

	return outputs, nil
}

func createGenesisBasicOutputs(api iotago.API, basicOutputs []BasicOutputDetails) (iotago.TxEssenceOutputs, error) {
	var outputs iotago.TxEssenceOutputs

	for idx, genesisBasicOutput := range basicOutputs {
		output := createOutput(genesisBasicOutput.Address, genesisBasicOutput.Amount, genesisBasicOutput.Mana)

		if _, err := api.StorageScoreStructure().CoversMinDeposit(output, genesisBasicOutput.Amount); err != nil {
			return nil, ierrors.Wrapf(err, "min rent not covered by Genesis basic output with index %d", idx)
		}

		outputs = append(outputs, output)
	}

	return outputs, nil
}

func createOutput(address iotago.Address, tokenAmount iotago.BaseToken, mana iotago.Mana) (output iotago.Output) {
	return &iotago.BasicOutput{
		Amount: tokenAmount,
		Mana:   mana,
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: address},
		},
	}
}

func createAccount(accountID iotago.AccountID, address iotago.Address, tokenAmount iotago.BaseToken, mana iotago.Mana, blockIssuerKey iotago.BlockIssuerKey, expirySlot iotago.SlotIndex, stakedAmount iotago.BaseToken, stakeEndEpoch iotago.EpochIndex, stakeFixedCost iotago.Mana) (output iotago.Output) {
	accountOutput := &iotago.AccountOutput{
		Amount:    tokenAmount,
		Mana:      mana,
		AccountID: accountID,
		Conditions: iotago.AccountOutputUnlockConditions{
			&iotago.StateControllerAddressUnlockCondition{Address: address},
			&iotago.GovernorAddressUnlockCondition{Address: address},
		},
		Features: iotago.AccountOutputFeatures{
			&iotago.BlockIssuerFeature{
				BlockIssuerKeys: iotago.NewBlockIssuerKeys(blockIssuerKey),
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
