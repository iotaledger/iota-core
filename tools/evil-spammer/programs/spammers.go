package programs

import (
	"sync"
	"time"

	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/tools/evil-spammer/logger"
	"github.com/iotaledger/iota-core/tools/evil-spammer/spammer"
	"github.com/iotaledger/iota-core/tools/evil-spammer/wallet"
	"github.com/iotaledger/iota.go/v4"
)

var log = logger.New("customSpam")

func CustomSpam(params *CustomSpamParams) {
	outputID := iotago.OutputIDFromTransactionIDAndIndex(snapshotcreator.GenesisTransactionID, 0)
	if params.Config.LastFaucetUnspentOutputID != "" {
		outputID, _ = iotago.OutputIDFromHex(params.Config.LastFaucetUnspentOutputID)
	}

	w := wallet.NewEvilWallet(wallet.WithClients(params.ClientURLs...), wallet.WithFaucetOutputID(outputID))
	wg := sync.WaitGroup{}

	// funds are requested fro all spam types except SpammerTypeBlock
	fundsNeeded := false
	for _, st := range params.SpamTypes {
		if st != spammer.TypeBlock {
			fundsNeeded = true
		}
	}
	if fundsNeeded {
		err := w.RequestFreshBigFaucetWallet()
		if err != nil {
			panic(err)
		}
		SaveConfigsToFile(&BasicConfig{
			LastFaucetUnspentOutputID: w.LastFaucetUnspentOutput().ToHex(),
		})
	}

	for i, sType := range params.SpamTypes {
		log.Infof("Start spamming with rate: %d, time unit: %s, and spamming type: %s.", params.Rates[i], params.TimeUnit.String(), sType)

		switch sType {
		case spammer.TypeBlock:
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				s := SpamBlocks(w, params.Rates[i], params.TimeUnit, params.Durations[i], params.BlkToBeSent[i], params.EnableRateSetter, params.AccountAlias)
				if s == nil {
					return
				}
				s.Spam()
			}(i)
		case spammer.TypeTx:
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				SpamTransaction(w, params.Rates[i], params.TimeUnit, params.Durations[i], params.DeepSpam, params.EnableRateSetter, params.AccountAlias)
			}(i)
		case spammer.TypeDs:
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				SpamDoubleSpends(w, params.Rates[i], params.NSpend, params.TimeUnit, params.Durations[i], params.DelayBetweenConflicts, params.DeepSpam, params.EnableRateSetter, params.AccountAlias)
			}(i)
		case spammer.TypeCustom:
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				s := SpamNestedConflicts(w, params.Rates[i], params.TimeUnit, params.Durations[i], params.Scenario, params.DeepSpam, false, params.EnableRateSetter, params.AccountAlias)
				if s == nil {
					return
				}
				s.Spam()
			}(i)
		case spammer.TypeCommitments:
			wg.Add(1)
			go func() {
				defer wg.Done()
			}()
		case spammer.TypeAccounts:
			wg.Add(1)
			go func() {
				defer wg.Done()

				s := SpamAccounts(w, params.Rates[i], params.TimeUnit, params.Durations[i], params.EnableRateSetter, params.AccountAlias)
				if s == nil {
					return
				}
				s.Spam()
			}()

		default:
			log.Warn("Spamming type not recognized. Try one of following: tx, ds, blk, custom, commitments")
		}
	}

	wg.Wait()
	log.Info("Basic spamming finished!")
}

func SpamTransaction(w *wallet.EvilWallet, rate int, timeUnit, duration time.Duration, deepSpam, enableRateSetter bool, accountAlias string) {
	if w.NumOfClient() < 1 {
		log.Infof("Warning: At least one client is needed to spam.")
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

func SpamDoubleSpends(w *wallet.EvilWallet, rate, nSpent int, timeUnit, duration, delayBetweenConflicts time.Duration, deepSpam, enableRateSetter bool, accountAlias string) {
	log.Debugf("Setting up double spend spammer with rate: %d, time unit: %s, and duration: %s.", rate, timeUnit.String(), duration.String())
	if w.NumOfClient() < 2 {
		log.Infof("Warning: At least two client are needed to spam, and %d was provided", w.NumOfClient())
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

func SpamNestedConflicts(w *wallet.EvilWallet, rate int, timeUnit, duration time.Duration, conflictBatch wallet.EvilBatch, deepSpam, reuseOutputs, enableRateSetter bool, accountAlias string) *spammer.Spammer {
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
		log.Infof("Warning: At least %d client are needed to spam, and %d was provided", scenario.NumOfClientsNeeded, w.NumOfClient())
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

func SpamBlocks(w *wallet.EvilWallet, rate int, timeUnit, duration time.Duration, numBlkToSend int, enableRateSetter bool, accountAlias string) *spammer.Spammer {
	if w.NumOfClient() < 1 {
		log.Infof("Warning: At least one client is needed to spam.")
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

func SpamAccounts(w *wallet.EvilWallet, rate int, timeUnit, duration time.Duration, enableRateSetter bool, accountAlias string) *spammer.Spammer {
	if w.NumOfClient() < 1 {
		log.Infof("Warning: At least one client is needed to spam.")
	}
	scenarioOptions := []wallet.ScenarioOption{
		wallet.WithScenarioCustomConflicts(wallet.SingleTransactionBatch()),
		wallet.WithCreateAccounts(),
	}

	scenarioAccount := wallet.NewEvilScenario(scenarioOptions...)

	options := []spammer.Options{
		spammer.WithSpamRate(rate, timeUnit),
		spammer.WithSpamDuration(duration),
		spammer.WithRateSetter(enableRateSetter),
		spammer.WithEvilWallet(w),
		spammer.WithSpammingFunc(spammer.AccountSpammingFunction),
		spammer.WithEvilScenario(scenarioAccount),
	}

	return spammer.NewSpammer(options...)
}
