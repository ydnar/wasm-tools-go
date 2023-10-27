package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/ydnar/wasm-tools-go/cmd/wit-bindgen-go/cmd/generate"
	"github.com/ydnar/wasm-tools-go/cmd/wit-bindgen-go/cmd/print"
	"github.com/ydnar/wasm-tools-go/cmd/wit-bindgen-go/cmd/wit"
)

func main() {
	cmd := &cli.Command{
		Name:  "wit-bindgen-go",
		Usage: "inspect or manipulate WebAssembly Interface Types for Go",
		Commands: []*cli.Command{
			generate.Command,
			print.Command,
			wit.Command,
		},
	}

	err := cmd.Run(context.Background(), os.Args)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
}
