package gen

import (
	"fmt"
	"testing"
)

func TestFileString(t *testing.T) {
	f := &File{
		Build: "wasm || wasm32 || tinygo.wasm",
		Package: &Package{
			Ident: Ident{
				Path: "wasm/wasi/clocks/wallclock",
				Name: "wallclock",
			},
		},
		Imports: map[string]string{
			"encoding/json": "json",
			"io":            "io",
		},
	}

	fmt.Println(string(f.Bytes()))
}
