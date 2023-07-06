package snapshotcreator

import (
	"crypto/ed25519"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"

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
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget/thresholdblockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/slotgadget/totalweightslotgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/blockfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	tipmanagerv1 "github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager/v1"
	tipselectionv1 "github.com/iotaledger/iota-core/pkg/protocol/engine/tipselection/v1"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1"
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

func CreateSnapshot(opts ...options.Option[Options]) error {
	opt := NewOptions(opts...)

	protocolParams := &opt.ProtocolParameters
	api := iotago.LatestAPI(protocolParams)

	errorHandler := func(err error) {
		panic(err)
	}

	workers := workerpool.NewGroup("CreateSnapshot")
	defer workers.Shutdown()
	s := storage.New(lo.PanicOnErr(os.MkdirTemp(os.TempDir(), "*")), opt.DataBaseVersion, errorHandler)
	defer s.Shutdown()

	if err := s.Commitments().Store(model.NewEmptyCommitment(api)); err != nil {
		return errors.Wrap(err, "failed to store empty commitment")
	}
	if err := s.Settings().SetProtocolParameters(opt.ProtocolParameters); err != nil {
		return errors.Wrap(err, "failed to set the genesis time")
	}

	accounts := account.NewAccounts()
	for _, accountData := range opt.Accounts {
		fmt.Println("account ID ", accountData.AccountID, hexutil.Encode(accountData.IssuerKey))
		// Only add genesis validators if an account has both - StakedAmount and StakingEndEpoch - specified.
		if accountData.StakedAmount > 0 && accountData.StakingEpochEnd > 0 {
			accounts.Set(blake2b.Sum256(accountData.IssuerKey), &account.Pool{
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
		inmemoryblockdag.NewProvider(),
		inmemorybooker.NewProvider(),
		blocktime.NewProvider(),
		thresholdblockgadget.NewProvider(),
		totalweightslotgadget.NewProvider(),
		sybilprotectionv1.NewProvider(sybilprotectionv1.WithInitialCommittee(accounts)),
		slotnotarization.NewProvider(slotnotarization.DefaultMinSlotCommittableAge),
		slotattestation.NewProvider(slotattestation.DefaultAttestationCommitmentOffset),
		opt.LedgerProvider(),
		tipmanagerv1.NewProvider(),
		tipselectionv1.NewProvider(),
	)
	defer engineInstance.Shutdown()

	engineInstance.TriggerConstructed()
	engineInstance.TriggerInitialized()

	for blockID, commitmentID := range opt.RootBlocks {
		engineInstance.EvictionState.AddRootBlock(blockID, commitmentID)
	}

	totalAccountDeposit := lo.Reduce(opt.Accounts, func(accumulator iotago.BaseToken, details AccountDetails) iotago.BaseToken {
		return accumulator + details.Amount
	}, iotago.BaseToken(0))
	if err := createGenesisOutput(opt.ProtocolParameters.TokenSupply-totalAccountDeposit, opt.GenesisSeed, engineInstance, protocolParams); err != nil {
		return errors.Wrap(err, "failed to create genesis outputs")
	}

	if err := createGenesisAccounts(opt.Accounts, engineInstance, protocolParams); err != nil {
		return errors.Wrap(err, "failed to create genesis account outputs")
	}

	return engineInstance.WriteSnapshot(opt.FilePath)
}

func createGenesisOutput(genesisTokenAmount iotago.BaseToken, genesisSeed []byte, engineInstance *engine.Engine, protocolParams *iotago.ProtocolParameters) (err error) {
	if genesisTokenAmount > 0 {
		genesisWallet := mock.NewHDWallet("genesis", genesisSeed, 0)
		output := createOutput(genesisWallet.Address(), genesisTokenAmount)

		if _, err = protocolParams.RentStructure.CoversStateRent(output, genesisTokenAmount); err != nil {
			return errors.Wrap(err, "min rent not covered by Genesis output with index 0")
		}

		// Genesis output is on Genesis TX index 0
		// TODO: change genesis outputID from empty transaction id to some hash, to avoid problems when rolling back newly created accounts, whose previousOutputID is also emptyTrasactionID:0 (super edge case, but better have that covered)
		if err := engineInstance.Ledger.AddUnspentOutput(utxoledger.CreateOutput(engineInstance.API(), iotago.OutputIDFromTransactionIDAndIndex(iotago.TransactionID{}, 0), iotago.EmptyBlockID(), 0, 0, output)); err != nil {
			return err
		}
	}

	return nil
}

func createGenesisAccounts(accounts []AccountDetails, engineInstance *engine.Engine, protocolParams *iotago.ProtocolParameters) (err error) {
	// Account outputs start from Genesis TX index 1
	for idx, account := range accounts {
		output := createAccount(account.AccountID, account.Address, account.Amount, account.IssuerKey, account.StakedAmount, account.StakingEpochEnd, account.FixedCost)

		if _, err = protocolParams.RentStructure.CoversStateRent(output, account.Amount); err != nil {
			return errors.Wrapf(err, "min rent not covered by account output with index %d", idx+1)
		}

		accountOutput := utxoledger.CreateOutput(engineInstance.API(), iotago.OutputIDFromTransactionIDAndIndex(iotago.TransactionID{}, uint16(idx+1)), iotago.EmptyBlockID(), 0, 0, output)
		if err = engineInstance.Ledger.AddUnspentOutput(accountOutput); err != nil {
			return err
		}
		if err = engineInstance.Ledger.AddAccount(accountOutput); err != nil {
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

func createAccount(accountID iotago.AccountID, address iotago.Address, tokenAmount iotago.BaseToken, pubkey ed25519.PublicKey, stakedAmount iotago.BaseToken, stakeEndEpoch iotago.EpochIndex, stakeFixedCost iotago.Mana) (output iotago.Output) {
	accountOutput := &iotago.AccountOutput{
		AccountID: accountID,
		Amount:    tokenAmount,
		Conditions: iotago.AccountOutputUnlockConditions{
			&iotago.StateControllerAddressUnlockCondition{Address: address},
			&iotago.GovernorAddressUnlockCondition{Address: address},
		},
		Features: iotago.AccountOutputFeatures{
			&iotago.BlockIssuerFeature{
				BlockIssuerKeys: []ed25519.PublicKey{pubkey},
			},
		},
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
