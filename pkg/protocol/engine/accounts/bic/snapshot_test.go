package bic_test

import (
	"bytes"
	"testing"

	"github.com/orcaman/writerseeker"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/bic"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/bic/tpkg"
	"github.com/iotaledger/iota-core/pkg/utils"
)

func TestSlotDiffSnapshotWriter(t *testing.T) {
	accountID := utils.RandAccountID()
	bicDiffChange := tpkg.RandomBicDiffChange()
	writer := &writerseeker.WriterSeeker{}
	pWriter := utils.NewPositionedWriter(writer)
	err := bic.SlotDiffSnapshotWriter(pWriter, accountID, *bicDiffChange, true)

	bicDiffChangeRead, accountIDRead, destroyedRead, err := bic.SlotDiffSnapshotReader(writer.BytesReader())
	require.NoError(t, err)
	require.Equal(t, bicDiffChange, bicDiffChangeRead)
	require.Equal(t, accountID, accountIDRead)
	require.Equal(t, true, destroyedRead)
}

func TestAccountDataSnapshotWriter(t *testing.T) {
	accountData := tpkg.RandomAccountData()
	accountsDataBytes, _ := accountData.SnapshotBytes()
	buf := bytes.NewReader(accountsDataBytes)

	readAccountsData, err := bic.AccountDataFromSnapshotReader(tpkg.API(), buf)
	require.NoError(t, err)

	tpkg.EqualAccountData(t, accountData, readAccountsData)
}

func TestAccountDiffSnapshotWriter(t *testing.T) {
	accountData := tpkg.RandomAccountData()
	writer := &writerseeker.WriterSeeker{}
	pWriter := utils.NewPositionedWriter(writer)
	err := bic.AccountDiffSnapshotWriter(pWriter, accountData)
	require.NoError(t, err)
	readAccountsData, err := bic.AccountDataFromSnapshotReader(tpkg.API(), writer.BytesReader())
	require.NoError(t, err)
	tpkg.EqualAccountData(t, accountData, readAccountsData)
}

func TestBICSnapshotBytes(t *testing.T) {

}

func TestTargetBICCreation(t *testing.T) {

}
