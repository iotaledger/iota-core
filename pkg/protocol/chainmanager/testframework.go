package chainmanager

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TestFramework struct {
	Instance *Manager

	test               *testing.T
	api                iotago.API
	commitmentsByAlias map[string]*model.Commitment

	forkDetected              int32
	commitmentMissing         int32
	missingCommitmentReceived int32
	commitmentBelowRoot       int32

	syncutils.RWMutex
}

func NewTestFramework(test *testing.T, api iotago.API, opts ...options.Option[TestFramework]) (testFramework *TestFramework) {
	snapshotCommitment := model.NewEmptyCommitment(api)

	return options.Apply(&TestFramework{
		Instance: NewManager(),

		test: test,
		api:  api,
		commitmentsByAlias: map[string]*model.Commitment{
			"Genesis": snapshotCommitment,
		},
	}, opts, func(t *TestFramework) {
		t.Instance.Initialize(snapshotCommitment)
		t.Instance.Events.ForkDetected.Hook(func(fork *Fork) {
			t.test.Logf("ForkDetected: %s", fork)
			atomic.AddInt32(&t.forkDetected, 1)
		})
		t.Instance.commitmentRequester.Events.TickerStarted.Hook(func(id iotago.CommitmentID) {
			t.test.Logf("CommitmentMissing: %s", id)
			atomic.AddInt32(&t.commitmentMissing, 1)
		})
		t.Instance.commitmentRequester.Events.TickerStopped.Hook(func(id iotago.CommitmentID) {
			t.test.Logf("MissingCommitmentReceived: %s", id)
			atomic.AddInt32(&t.missingCommitmentReceived, 1)
		})
		t.Instance.Events.CommitmentBelowRoot.Hook(func(id iotago.CommitmentID) {
			t.test.Logf("CommitmentBelowRoot: %s", id)
			atomic.AddInt32(&t.commitmentBelowRoot, 1)
		})
	})
}

func (t *TestFramework) CreateCommitment(alias string, prevAlias string, cumulativeWeight uint64) {
	t.Lock()
	defer t.Unlock()

	prevCommitmentID, previousIndex := t.previousCommitmentID(prevAlias)
	randomECR := blake2b.Sum256([]byte(alias + prevAlias))

	cm, err := model.CommitmentFromCommitment(iotago.NewCommitment(t.api.ProtocolParameters().Version(), previousIndex+1, prevCommitmentID, randomECR, cumulativeWeight, 0), t.api)
	require.NoError(t.test, err)
	t.commitmentsByAlias[alias] = cm
	t.commitmentsByAlias[alias].ID().RegisterAlias(alias)
}

func (t *TestFramework) ProcessCommitment(alias string) (isSolid bool, chain *Chain) {
	return t.Instance.ProcessCommitment(t.commitment(alias))
}

func (t *TestFramework) ProcessCommitmentFromOtherSource(alias string) (isSolid bool, chain *Chain) {
	return t.Instance.ProcessCommitmentFromSource(t.commitment(alias), "otherid")
}

func (t *TestFramework) Chain(alias string) (chain *Chain) {
	return t.Instance.Chain(t.SlotCommitment(alias))
}

func (t *TestFramework) commitment(alias string) *model.Commitment {
	t.RLock()
	defer t.RUnlock()

	commitment, exists := t.commitmentsByAlias[alias]
	if !exists {
		panic("the commitment does not exist")
	}

	return commitment
}

func (t *TestFramework) ChainCommitment(alias string) *ChainCommitment {
	cm, exists := t.Instance.Commitment(t.SlotCommitment(alias))
	require.True(t.test, exists)

	return cm
}

func (t *TestFramework) AssertForkDetectedCount(expected int) {
	require.EqualValues(t.test, expected, t.forkDetected, "forkDetected count does not match")
}

func (t *TestFramework) AssertCommitmentMissingCount(expected int) {
	require.EqualValues(t.test, expected, t.commitmentMissing, "commitmentMissing count does not match")
}

func (t *TestFramework) AssertMissingCommitmentReceivedCount(expected int) {
	require.EqualValues(t.test, expected, t.missingCommitmentReceived, "missingCommitmentReceived count does not match")
}

func (t *TestFramework) AssertCommitmentBelowRootCount(expected int) {
	require.EqualValues(t.test, expected, t.commitmentBelowRoot, "commitmentBelowRoot count does not match")
}

func (t *TestFramework) AssertEqualChainCommitments(commitments []*ChainCommitment, aliases ...string) {
	var chainCommitments []*ChainCommitment
	for _, alias := range aliases {
		chainCommitments = append(chainCommitments, t.ChainCommitment(alias))
	}

	require.EqualValues(t.test, commitments, chainCommitments)
}

func (t *TestFramework) SlotCommitment(alias string) iotago.CommitmentID {
	return t.commitment(alias).ID()
}

func (t *TestFramework) SlotIndex(alias string) iotago.SlotIndex {
	return t.commitment(alias).Index()
}

func (t *TestFramework) SlotCommitmentRoot(alias string) iotago.Identifier {
	return t.commitment(alias).RootsID()
}

func (t *TestFramework) PrevSlotCommitment(alias string) iotago.CommitmentID {
	return t.commitment(alias).PrevID()
}

func (t *TestFramework) AssertChainIsAlias(chain *Chain, alias string) {
	if alias == "" {
		require.Nil(t.test, chain)
		return
	}

	require.Equal(t.test, t.commitment(alias).ID(), chain.ForkingPoint.ID())
}

func (t *TestFramework) AssertChainState(chains map[string]string) {
	commitmentsByChainAlias := make(map[string][]string)

	for commitmentAlias, chainAlias := range chains {
		if chainAlias == "" {
			require.Nil(t.test, t.Chain(commitmentAlias))
			continue
		}
		if chainAlias == "evicted" {
			_, exists := t.Instance.Commitment(t.SlotCommitment(commitmentAlias))
			require.False(t.test, exists, "commitment %s should be evicted", commitmentAlias)
			continue
		}
		commitmentsByChainAlias[chainAlias] = append(commitmentsByChainAlias[chainAlias], commitmentAlias)

		chain := t.Chain(commitmentAlias)

		require.NotNil(t.test, chain, "chain for commitment %s is nil", commitmentAlias)
		require.Equal(t.test, t.SlotCommitment(chainAlias), chain.ForkingPoint.ID())
	}
}

func (t *TestFramework) previousCommitmentID(alias string) (previousCommitmentID iotago.CommitmentID, previousIndex iotago.SlotIndex) {
	if alias == "" {
		return
	}

	previousCommitment, exists := t.commitmentsByAlias[alias]
	if !exists {
		panic("the previous commitment does not exist")
	}

	return previousCommitment.ID(), previousCommitment.Index()
}
