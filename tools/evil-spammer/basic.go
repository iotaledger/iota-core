package main

import (
	"sync"
	"time"

	"github.com/iotaledger/iota-core/pkg/testsuite/snapshotcreator"
	"github.com/iotaledger/iota-core/tools/evil-spammer/spammer"
	"github.com/iotaledger/iota-core/tools/evil-spammer/wallet"
	iotago "github.com/iotaledger/iota.go/v4"
)

type CustomSpamParams struct {
	ClientURLs            []string
	SpamTypes             []string
	Rates                 []int
	Durations             []time.Duration
	BlkToBeSent           []int
	TimeUnit              time.Duration
	DelayBetweenConflicts time.Duration
	NSpend                int
	Scenario              wallet.EvilBatch
	DeepSpam              bool
	EnableRateSetter      bool

	config *BasicConfig
}

func CustomSpam(params *CustomSpamParams) {
	outputID := iotago.OutputIDFromTransactionIDAndIndex(snapshotcreator.GenesisTransactionID, 0)
	if params.config.LastFaucetUnspentOutputID != "" {
		outputID, _ = iotago.OutputIDFromHex(params.config.LastFaucetUnspentOutputID)
	}

	w := wallet.NewEvilWallet(wallet.WithClients(params.ClientURLs...), wallet.WithFaucetOutputID(outputID))
	wg := sync.WaitGroup{}

	fundsNeeded := false
	for _, st := range params.SpamTypes {
		if st != SpammerTypeBlock {
			fundsNeeded = true
		}
	}
	if fundsNeeded {
		err := w.RequestFreshBigFaucetWallet()
		if err != nil {
			panic(err)
		}
		saveConfigsToFile(&BasicConfig{
			LastFaucetUnspentOutputID: w.LastFaucetUnspentOutput().ToHex(),
		})
	}

	for i, sType := range params.SpamTypes {
		log.Infof("Start spamming with rate: %d, time unit: %s, and spamming type: %s.", params.Rates[i], params.TimeUnit.String(), sType)

		switch sType {
		case SpammerTypeBlock:
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				s := SpamBlocks(w, params.Rates[i], params.TimeUnit, params.Durations[i], params.BlkToBeSent[i], params.EnableRateSetter)
				if s == nil {
					return
				}
				s.Spam()
			}(i)
		case SpammerTypeTx:
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				SpamTransaction(w, params.Rates[i], params.TimeUnit, params.Durations[i], params.DeepSpam, params.EnableRateSetter)
			}(i)
		case SpammerTypeDs:
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				SpamDoubleSpends(w, params.Rates[i], params.NSpend, params.TimeUnit, params.Durations[i], params.DelayBetweenConflicts, params.DeepSpam, params.EnableRateSetter)
			}(i)
		case SpammerTypeCustom:
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				s := SpamNestedConflicts(w, params.Rates[i], params.TimeUnit, params.Durations[i], params.Scenario, params.DeepSpam, false, params.EnableRateSetter)
				if s == nil {
					return
				}
				s.Spam()
			}(i)
		case SpammerTypeCommitments:
			wg.Add(1)
			go func() {
				defer wg.Done()
			}()

		default:
			log.Warn("Spamming type not recognized. Try one of following: tx, ds, blk, custom, commitments")
		}
	}

	wg.Wait()
	log.Info("Basic spamming finished!")
}

func SpamTransaction(w *wallet.EvilWallet, rate int, timeUnit, duration time.Duration, deepSpam, enableRateSetter bool) {
	if w.NumOfClient() < 1 {
		printer.NotEnoughClientsWarning(1)
	}

	scenarioOptions := []wallet.ScenarioOption{
		wallet.WithScenarioCustomConflicts(wallet.SingleTransactionBatch()),
	}
	if deepSpam {
		outWallet := wallet.NewWallet(wallet.Reuse)
		scenarioOptions = append(scenarioOptions,
			wallet.WithScenarioDeepSpamEnabled(),
			wallet.WithScenarioReuseOutputWallet(outWallet),
			wallet.WithScenarioInputWalletForDeepSpam(outWallet),
		)
	}
	scenarioTx := wallet.NewEvilScenario(scenarioOptions...)

	options := []spammer.Options{
		spammer.WithSpamRate(rate, timeUnit),
		spammer.WithSpamDuration(duration),
		spammer.WithRateSetter(enableRateSetter),
		spammer.WithEvilWallet(w),
		spammer.WithEvilScenario(scenarioTx),
	}

	s := spammer.NewSpammer(options...)
	s.Spam()
}

func SpamDoubleSpends(w *wallet.EvilWallet, rate, nSpent int, timeUnit, duration, delayBetweenConflicts time.Duration, deepSpam, enableRateSetter bool) {
	log.Debugf("Setting up double spend spammer with rate: %d, time unit: %s, and duration: %s.", rate, timeUnit.String(), duration.String())
	if w.NumOfClient() < 2 {
		printer.NotEnoughClientsWarning(2)
	}

	scenarioOptions := []wallet.ScenarioOption{
		wallet.WithScenarioCustomConflicts(wallet.NSpendBatch(nSpent)),
	}
	if deepSpam {
		outWallet := wallet.NewWallet(wallet.Reuse)
		scenarioOptions = append(scenarioOptions,
			wallet.WithScenarioDeepSpamEnabled(),
			wallet.WithScenarioReuseOutputWallet(outWallet),
			wallet.WithScenarioInputWalletForDeepSpam(outWallet),
		)
	}
	scenarioDs := wallet.NewEvilScenario(scenarioOptions...)
	options := []spammer.Options{
		spammer.WithSpamRate(rate, timeUnit),
		spammer.WithSpamDuration(duration),
		spammer.WithEvilWallet(w),
		spammer.WithRateSetter(enableRateSetter),
		spammer.WithTimeDelayForDoubleSpend(delayBetweenConflicts),
		spammer.WithEvilScenario(scenarioDs),
	}

	s := spammer.NewSpammer(options...)
	s.Spam()
}

func SpamNestedConflicts(w *wallet.EvilWallet, rate int, timeUnit, duration time.Duration, conflictBatch wallet.EvilBatch, deepSpam, reuseOutputs, enableRateSetter bool) *spammer.Spammer {
	scenarioOptions := []wallet.ScenarioOption{
		wallet.WithScenarioCustomConflicts(conflictBatch),
	}
	if deepSpam {
		outWallet := wallet.NewWallet(wallet.Reuse)
		scenarioOptions = append(scenarioOptions,
			wallet.WithScenarioDeepSpamEnabled(),
			wallet.WithScenarioReuseOutputWallet(outWallet),
			wallet.WithScenarioInputWalletForDeepSpam(outWallet),
		)
	} else if reuseOutputs {
		outWallet := wallet.NewWallet(wallet.Reuse)
		scenarioOptions = append(scenarioOptions, wallet.WithScenarioReuseOutputWallet(outWallet))
	}
	scenario := wallet.NewEvilScenario(scenarioOptions...)
	if scenario.NumOfClientsNeeded > w.NumOfClient() {
		printer.NotEnoughClientsWarning(scenario.NumOfClientsNeeded)
	}

	options := []spammer.Options{
		spammer.WithSpamRate(rate, timeUnit),
		spammer.WithSpamDuration(duration),
		spammer.WithEvilWallet(w),
		spammer.WithRateSetter(enableRateSetter),
		spammer.WithEvilScenario(scenario),
	}

	return spammer.NewSpammer(options...)
}

func SpamBlocks(w *wallet.EvilWallet, rate int, timeUnit, duration time.Duration, numBlkToSend int, enableRateSetter bool) *spammer.Spammer {
	if w.NumOfClient() < 1 {
		printer.NotEnoughClientsWarning(1)
	}

	options := []spammer.Options{
		spammer.WithSpamRate(rate, timeUnit),
		spammer.WithSpamDuration(duration),
		spammer.WithBatchesSent(numBlkToSend),
		spammer.WithRateSetter(enableRateSetter),
		spammer.WithEvilWallet(w),
		spammer.WithSpammingFunc(spammer.DataSpammingFunction),
	}

	return spammer.NewSpammer(options...)
}
