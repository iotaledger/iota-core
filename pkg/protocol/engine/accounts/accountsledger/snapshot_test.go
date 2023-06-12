package accountsledger

import (
	"bytes"
	"testing"

	"github.com/orcaman/writerseeker"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/accountsledger/tpkg"
	"github.com/iotaledger/iota-core/pkg/utils"
)

func TestSlotDiffSnapshotWriter(t *testing.T) {
	accountID := utils.RandAccountID()
	accountDiff := tpkg.RandomAccountDiff()
	writer := &writerseeker.WriterSeeker{}
	pWriter := utils.NewPositionedWriter(writer)
	err := writeSlotDiff(pWriter, accountID, *accountDiff, true)

	accountDiffRead, accountIDRead, destroyedRead, err := readSlotDiff(writer.BytesReader(), tpkg.API())
	require.NoError(t, err)
	require.Equal(t, accountDiff, accountDiffRead)
	require.Equal(t, accountID, accountIDRead)
	require.Equal(t, true, destroyedRead)
}

func TestAccountDataSnapshotWriter(t *testing.T) {
	accountData := tpkg.RandomAccountData()
	accountsDataBytes, _ := accountData.Bytes()
	buf := bytes.NewReader(accountsDataBytes)

	readAccountsData, err := readAccountData(tpkg.API(), buf)
	require.NoError(t, err)

	tpkg.EqualAccountData(t, accountData, readAccountsData)
}

func TestAccountDiffSnapshotWriter(t *testing.T) {
	accountData := tpkg.RandomAccountData()
	writer := &writerseeker.WriterSeeker{}
	pWriter := utils.NewPositionedWriter(writer)
	err := writeAccountData(pWriter, accountData)
	require.NoError(t, err)
	readAccountsData, err := readAccountData(tpkg.API(), writer.BytesReader())
	require.NoError(t, err)
	tpkg.EqualAccountData(t, accountData, readAccountsData)
}

func TestManager_Import(t *testing.T) {
}

func TestManager_Export(t *testing.T) {
}
