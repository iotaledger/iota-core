package main

import (
	"log"
	"strings"

	"github.com/mr-tron/base58"
	flag "github.com/spf13/pflag"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/testsuite/snapshotcreator"
	"github.com/iotaledger/iota-core/tools/genesis-snapshot/presets"
	"github.com/iotaledger/iota.go/v4/wallet"
)

func main() {
	parsedOpts, configSelected := parseFlags()
	opts := presets.SnapshotOptionsBase
	if strings.Contains(configSelected, ".yml") {
		yamlOpts, err := presets.GenerateFromYaml(configSelected)
		if err != nil {
			panic(err)
		}
		opts = append(opts, yamlOpts...)
	} else {
		switch configSelected {
		case "docker":
			opts = append(opts, presets.SnapshotOptionsDocker...)
		case "feature":
			opts = append(opts, presets.SnapshotOptionsFeature...)
		default:
			configSelected = "default"
		}
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
	keyManager, err := wallet.NewKeyManager(genesisSeed, wallet.DefaultIOTAPath)
	if err != nil {
		log.Fatal(ierrors.Wrap(err, "failed to create KeyManager from seed"))
	}
	opt = append(opt, snapshotcreator.WithGenesisKeyManager(keyManager))

	return opt, *config
}
