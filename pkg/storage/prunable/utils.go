package prunable

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	iotago "github.com/iotaledger/iota.go/v4"
)

func dbPathFromIndex(base string, epoch iotago.EpochIndex) string {
	return filepath.Join(base, strconv.FormatInt(int64(epoch), 10))
}

type dbInstanceFileInfo struct {
	baseEpoch iotago.EpochIndex
	path      string
}

// getSortedDBInstancesFromDisk returns an ASC sorted list of db instances from the given base directory.
func getSortedDBInstancesFromDisk(baseDir string) (dbInfos []*dbInstanceFileInfo) {
	files, err := os.ReadDir(baseDir)
	if err != nil {
		panic(err)
	}

	files = lo.Filter(files, func(e os.DirEntry) bool { return e.IsDir() })
	dbInfos = lo.Map(files, func(e os.DirEntry) *dbInstanceFileInfo {
		atoi, convErr := strconv.Atoi(e.Name())
		if convErr != nil {
			return nil
		}

		return &dbInstanceFileInfo{
			baseEpoch: iotago.EpochIndex(atoi),
			path:      filepath.Join(baseDir, e.Name()),
		}
	})
	dbInfos = lo.Filter(dbInfos, func(info *dbInstanceFileInfo) bool { return info != nil })

	sort.Slice(dbInfos, func(i int, j int) bool {
		return dbInfos[i].baseEpoch < dbInfos[j].baseEpoch
	})

	return dbInfos
}

func dbPrunableDirectorySize(base string, epoch iotago.EpochIndex) (int64, error) {
	return ioutils.FolderSize(dbPathFromIndex(base, epoch))
}
