package spammer

import (
	"time"

	"go.uber.org/atomic"

	"github.com/iotaledger/hive.go/app/configuration"
	appLogger "github.com/iotaledger/hive.go/app/logger"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/tools/evil-spammer/wallet"
	"github.com/iotaledger/iota-core/tools/genesis-snapshot/presets"
	iotago "github.com/iotaledger/iota.go/v4"
)

// region Spammer //////////////////////////////////////////////////////////////////////////////////////////////////////
//
//nolint:revive
type SpammerFunc func(*Spammer)

type State struct {
	spamTicker    *time.Ticker
	logTicker     *time.Ticker
	spamStartTime time.Time
	txSent        *atomic.Int64
	batchPrepared *atomic.Int64

	logTickTime  time.Duration
	spamDuration time.Duration
}

type SpamType int

const (
	SpamEvilWallet SpamType = iota
	SpamCommitments
)

// Spammer is a utility object for new spammer creations, can be modified by passing options.
// Mandatory options: WithClients, WithSpammingFunc
// Not mandatory options, if not provided spammer will use default settings:
// WithSpamDetails, WithEvilWallet, WithErrorCounter, WithLogTickerInterval.
type Spammer struct {
	SpamDetails     *SpamDetails
	State           *State
	UseRateSetter   bool
	SpamType        SpamType
	Clients         wallet.Connector
	EvilWallet      *wallet.EvilWallet
	EvilScenario    *wallet.EvilScenario
	IdentityManager *IdentityManager
	// CommitmentManager *CommitmentManager
	ErrCounter *ErrorCounter

	log Logger
	api iotago.API

	// accessed from spamming functions
	done     chan bool
	shutdown chan types.Empty
	spamFunc SpammerFunc

	TimeDelayBetweenConflicts time.Duration
	NumberOfSpends            int
}

// NewSpammer is a constructor of Spammer.
func NewSpammer(options ...Options) *Spammer {
	protocolParams := snapshotcreator.NewOptions(presets.Docker...).ProtocolParameters
	api := iotago.V3API(protocolParams)

	state := &State{
		txSent:        atomic.NewInt64(0),
		batchPrepared: atomic.NewInt64(0),
		logTickTime:   time.Second * 30,
	}
	s := &Spammer{
		SpamDetails:     &SpamDetails{},
		spamFunc:        CustomConflictSpammingFunc,
		State:           state,
		SpamType:        SpamEvilWallet,
		EvilScenario:    wallet.NewEvilScenario(),
		IdentityManager: NewIdentityManager(),
		// CommitmentManager: NewCommitmentManager(),
		UseRateSetter:  true,
		done:           make(chan bool),
		shutdown:       make(chan types.Empty),
		NumberOfSpends: 2,
		api:            api,
	}

	for _, opt := range options {
		opt(s)
	}

	s.setup()

	return s
}

func (s *Spammer) BlocksSent() uint64 {
	return uint64(s.State.txSent.Load())
}

func (s *Spammer) BatchesPrepared() uint64 {
	return uint64(s.State.batchPrepared.Load())
}

func (s *Spammer) setup() {
	if s.log == nil {
		s.initLogger()
		s.IdentityManager.SetLogger(s.log)
	}

	switch s.SpamType {
	case SpamEvilWallet:
		if s.EvilWallet == nil {
			s.EvilWallet = wallet.NewEvilWallet()
		}
		s.Clients = s.EvilWallet.Connector()
		// case SpamCommitments:
		// 	s.CommitmentManager.Setup(s.log)
	}
	s.setupSpamDetails()

	s.State.spamTicker = s.initSpamTicker()
	s.State.logTicker = s.initLogTicker()

	if s.ErrCounter == nil {
		s.ErrCounter = NewErrorCount()
	}
}

func (s *Spammer) setupSpamDetails() {
	if s.SpamDetails.Rate <= 0 {
		s.SpamDetails.Rate = 1
	}
	if s.SpamDetails.TimeUnit == 0 {
		s.SpamDetails.TimeUnit = time.Second
	}
	// provided only maxBlkSent, calculating the default max for maxDuration
	if s.SpamDetails.MaxDuration == 0 && s.SpamDetails.MaxBatchesSent > 0 {
		s.SpamDetails.MaxDuration = time.Hour * 100
	}
	// provided only maxDuration, calculating the default max for maxBlkSent
	if s.SpamDetails.MaxBatchesSent == 0 && s.SpamDetails.MaxDuration > 0 {
		s.SpamDetails.MaxBatchesSent = int(s.SpamDetails.MaxDuration.Seconds()/s.SpamDetails.TimeUnit.Seconds()*float64(s.SpamDetails.Rate)) + 1
	}
}

func (s *Spammer) initLogger() {
	config := configuration.New()
	_ = appLogger.InitGlobalLogger(config)
	logger.SetLevel(logger.LevelDebug)
	s.log = logger.NewLogger("Spammer")
}

func (s *Spammer) initSpamTicker() *time.Ticker {
	tickerTime := float64(s.SpamDetails.TimeUnit) / float64(s.SpamDetails.Rate)
	return time.NewTicker(time.Duration(tickerTime))
}

func (s *Spammer) initLogTicker() *time.Ticker {
	return time.NewTicker(s.State.logTickTime)
}

// Spam runs the spammer. Function will stop after maxDuration time will pass or when maxBlkSent will be exceeded.
func (s *Spammer) Spam() {
	s.log.Infof("Start spamming transactions with %d rate", s.SpamDetails.Rate)

	s.State.spamStartTime = time.Now()
	timeExceeded := time.After(s.SpamDetails.MaxDuration)

	go func() {
		goroutineCount := atomic.NewInt32(0)
		for {
			select {
			case <-s.State.logTicker.C:
				s.log.Infof("Blocks issued so far: %d, errors encountered: %d", s.State.txSent.Load(), s.ErrCounter.GetTotalErrorCount())
			case <-timeExceeded:
				s.log.Infof("Maximum spam duration exceeded, stopping spammer....")
				s.StopSpamming()

				return
			case <-s.done:
				s.StopSpamming()
				return
			case <-s.State.spamTicker.C:
				if goroutineCount.Load() > 100 {
					break
				}
				go func() {
					goroutineCount.Inc()
					defer goroutineCount.Dec()
					s.spamFunc(s)
				}()
			}
		}
	}()
	<-s.shutdown
	s.log.Info(s.ErrCounter.GetErrorsSummary())
	s.log.Infof("Finishing spamming, total txns sent: %v, TotalTime: %v, Rate: %f", s.State.txSent.Load(), s.State.spamDuration.Seconds(), float64(s.State.txSent.Load())/s.State.spamDuration.Seconds())
}

func (s *Spammer) CheckIfAllSent() {
	if s.State.batchPrepared.Load() >= int64(s.SpamDetails.MaxBatchesSent) {
		s.log.Infof("Maximum number of blocks sent, stopping spammer...")
		s.done <- true
	}
}

// StopSpamming finishes tasks before shutting down the spammer.
func (s *Spammer) StopSpamming() {
	s.State.spamDuration = time.Since(s.State.spamStartTime)
	s.State.spamTicker.Stop()
	s.State.logTicker.Stop()
	// s.CommitmentManager.Shutdown()
	s.shutdown <- types.Void
}

// PostTransaction use provided client to issue a transaction. It chooses API method based on Spammer options. Counts errors,
// counts transactions and provides debug logs.
func (s *Spammer) PostTransaction(tx *iotago.SignedTransaction, clt wallet.Client) {
	if tx == nil {
		s.log.Debug(ErrTransactionIsNil)
		s.ErrCounter.CountError(ErrTransactionIsNil)

		return
	}

	txID := lo.PanicOnErr(tx.ID())
	allSolid := s.handleSolidityForReuseOutputs(clt, tx)
	if !allSolid {
		s.log.Debug(ErrInputsNotSolid)
		s.ErrCounter.CountError(ierrors.Wrapf(ErrInputsNotSolid, "txID: %s", txID.ToHex()))

		return
	}

	blockID, err := clt.PostTransaction(tx)
	if err != nil {
		s.log.Debug(ierrors.Wrapf(ErrFailPostTransaction, err.Error()))
		s.ErrCounter.CountError(ierrors.Wrapf(ErrFailPostTransaction, err.Error()))

		return
	}
	if s.EvilScenario.OutputWallet.Type() == wallet.Reuse {
		var outputIDs iotago.OutputIDs
		for index := range tx.Transaction.Outputs {
			outputIDs = append(outputIDs, iotago.OutputIDFromTransactionIDAndIndex(txID, uint16(index)))
		}
		s.EvilWallet.SetTxOutputsSolid(outputIDs, clt.URL())
	}

	count := s.State.txSent.Add(1)
	s.log.Debugf("%s: Last transaction sent, ID: %s, txCount: %d", blockID.ToHex(), txID.ToHex(), count)
}

func (s *Spammer) handleSolidityForReuseOutputs(_ wallet.Client, _ *iotago.SignedTransaction) (ok bool) {
	// ok = s.EvilWallet.AwaitInputsSolidity(tx.SignedTransaction().Inputs(), clt)
	// if s.EvilScenario.OutputWallet.Type() == wallet.Reuse {
	// 	s.EvilWallet.AddReuseOutputsToThePool(tx.SignedTransaction().Outputs())
	// }
	return true
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

type Logger interface {
	Infof(template string, args ...interface{})
	Info(args ...interface{})
	Debugf(template string, args ...interface{})
	Debug(args ...interface{})
	Warn(args ...interface{})
	Warnf(template string, args ...interface{})
	Error(args ...interface{})
	Errorf(template string, args ...interface{})
}
