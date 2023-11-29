package main

import (
	"log"

	"github.com/mr-tron/base58"
	flag "github.com/spf13/pflag"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/pkg/testsuite/snapshotcreator"
	"github.com/iotaledger/iota-core/tools/genesis-snapshot/presets"
)

func main() {
	parsedOpts, configSelected := parseFlags()
	opts := presets.Base
	switch configSelected {
	case "docker":
		opts = append(opts, presets.Docker...)
	case "test":
		opts = append(opts, presets.DockerTest...)
	case "feature":
		opts = append(opts, presets.Feature...)
	default:
		configSelected = "default"
	}
	opts = append(opts, parsedOpts...)
	info := snapshotcreator.NewOptions(opts...)

	log.Printf("creating snapshot with config: %s... %s", configSelected, info.FilePath)
	err := snapshotcreator.CreateSnapshot(opts...)
	if err != nil {
		panic(err)
	}
}

func parseFlags() (opt []options.Option[snapshotcreator.Options], conf string) {
	filename := flag.String("filename", "", "the name of the generated snapshot file")
	config := flag.String("config", "", "use ready config: devnet, feature, docker")
	genesisSeedStr := flag.String("seed", "7R1itJx5hVuo9w9hjg5cwKFmek4HMSoBDgJZN8hKGxih", "the genesis seed provided in base58 format.")

	flag.Parse()
	opt = []options.Option[snapshotcreator.Options]{}
	if *filename != "" {
		opt = append(opt, snapshotcreator.WithFilePath(*filename))
	}

	genesisSeed, err := base58.Decode(*genesisSeedStr)
	if err != nil {
		log.Fatal(ierrors.Wrap(err, "failed to decode base58 seed"))
	}
	keyManager := mock.NewKeyManager(genesisSeed[:], 0)
	opt = append(opt, snapshotcreator.WithGenesisKeyManager(keyManager))

	return opt, *config
}
