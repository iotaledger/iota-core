package main

import (
	"log"

	flag "github.com/spf13/pflag"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/snapshotcreator"
	"github.com/iotaledger/iota-core/tools/genesis-snapshot/presets"
)

func main() {
	parsedOpts, configSelected := parseFlags()
	opts := presets.Base
	switch configSelected {
	case "docker":
		opts = append(opts, presets.Docker...)
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

	flag.Parse()
	opt = []options.Option[snapshotcreator.Options]{}
	if *filename != "" {
		opt = append(opt, snapshotcreator.WithFilePath(*filename))
	}
	return opt, *config
}
