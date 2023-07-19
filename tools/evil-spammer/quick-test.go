package main

import (
	"time"

	"github.com/iotaledger/iota-core/tools/evil-spammer/evilspammerpkg"
	"github.com/iotaledger/iota-core/tools/evilwallet"
)

type QuickTestParams struct {
	ClientURLs            []string
	Rate                  int
	Duration              time.Duration
	TimeUnit              time.Duration
	DelayBetweenConflicts time.Duration
	VerifyLedger          bool
	EnableRateSetter      bool
}

// QuickTest runs short spamming periods with stable mps.
func QuickTest(params *QuickTestParams) {
	evilWallet := evilwallet.NewEvilWallet(evilwallet.WithClients(params.ClientURLs...))
	counter := evilspammerpkg.NewErrorCount()
	log.Info("Starting quick test")

	nWallets := 2 * evilspammerpkg.BigWalletsNeeded(params.Rate, params.TimeUnit, params.Duration)

	log.Info("Start preparing funds")
	evilWallet.RequestFreshBigFaucetWallets(nWallets)

	// define spammers
	baseOptions := []evilspammerpkg.Options{
		evilspammerpkg.WithSpamRate(params.Rate, params.TimeUnit),
		evilspammerpkg.WithSpamDuration(params.Duration),
		evilspammerpkg.WithErrorCounter(counter),
		evilspammerpkg.WithEvilWallet(evilWallet),
	}

	//nolint:gocritic // we want a copy here
	blkOptions := append(baseOptions,
		evilspammerpkg.WithSpammingFunc(evilspammerpkg.DataSpammingFunction),
	)

	dsScenario := evilwallet.NewEvilScenario(
		evilwallet.WithScenarioCustomConflicts(evilwallet.NSpendBatch(2)),
	)

	//nolint:gocritic // we want a copy here
	dsOptions := append(baseOptions,
		evilspammerpkg.WithEvilScenario(dsScenario),
	)

	blkSpammer := evilspammerpkg.NewSpammer(blkOptions...)
	txSpammer := evilspammerpkg.NewSpammer(baseOptions...)
	dsSpammer := evilspammerpkg.NewSpammer(dsOptions...)

	// start test
	txSpammer.Spam()
	time.Sleep(5 * time.Second)

	blkSpammer.Spam()
	time.Sleep(5 * time.Second)

	dsSpammer.Spam()

	log.Info(counter.GetErrorsSummary())
	log.Info("Quick Test finished")
}
