package generate

import (
	"github.com/urfave/cli/v3"
	"github.com/ydnar/wasm-tools-go/bindgen"
	"github.com/ydnar/wasm-tools-go/internal/witcli"
)

// Command is the CLI command for wit.
var Command = &cli.Command{
	Name:    "generate",
	Aliases: []string{"go"},
	Usage:   "generates Go from a fully-resolved WIT JSON file",
	Flags: []cli.Flag{
		&cli.StringMapFlag{
			Name:  "map",
			Usage: "maps WIT identifiers to Go identifiers",
		},
	},
	Action: action,
}

func action(ctx *cli.Context) error {
	res, err := witcli.LoadOne(ctx.Args().Slice()...)
	if err != nil {
		return err
	}
	packages, err := bindgen.Go(res)
	if err != nil {
		return err
	}
	var _ = packages
	return nil
}
