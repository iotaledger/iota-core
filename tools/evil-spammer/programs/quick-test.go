package programs

import (
	"time"

	"github.com/iotaledger/iota-core/tools/evil-spammer/evilwallet"
	"github.com/iotaledger/iota-core/tools/evil-spammer/spammer"
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
	counter := spammer.NewErrorCount()
	log.Info("Starting quick test")

	nWallets := 2 * spammer.BigWalletsNeeded(params.Rate, params.TimeUnit, params.Duration)

	log.Info("Start preparing funds")
	evilWallet.RequestFreshBigFaucetWallets(nWallets)

	// define spammers
	baseOptions := []spammer.Options{
		spammer.WithSpamRate(params.Rate, params.TimeUnit),
		spammer.WithSpamDuration(params.Duration),
		spammer.WithErrorCounter(counter),
		spammer.WithEvilWallet(evilWallet),
	}

	//nolint:gocritic // we want a copy here
	blkOptions := append(baseOptions,
		spammer.WithSpammingFunc(spammer.DataSpammingFunction),
	)

	dsScenario := evilwallet.NewEvilScenario(
		evilwallet.WithScenarioCustomConflicts(evilwallet.NSpendBatch(2)),
	)

	//nolint:gocritic // we want a copy here
	dsOptions := append(baseOptions,
		spammer.WithEvilScenario(dsScenario),
	)

	blkSpammer := spammer.NewSpammer(blkOptions...)
	txSpammer := spammer.NewSpammer(baseOptions...)
	dsSpammer := spammer.NewSpammer(dsOptions...)

	// start test
	txSpammer.Spam()
	time.Sleep(5 * time.Second)

	blkSpammer.Spam()
	time.Sleep(5 * time.Second)

	dsSpammer.Spam()

	log.Info(counter.GetErrorsSummary())
	log.Info("Quick Test finished")
}
