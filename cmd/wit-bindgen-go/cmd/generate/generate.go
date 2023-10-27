package generate

import (
	"github.com/urfave/cli/v3"
	"github.com/ydnar/wasm-tools-go/bindgen"
	"github.com/ydnar/wasm-tools-go/internal/witcli"
)

// Command is the CLI command for wit.
var Command = &cli.Command{
	Name:   "generate",
	Usage:  "generates Go from a fully-resolved WIT JSON file",
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
