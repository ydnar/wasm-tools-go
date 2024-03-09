// Code generated by wit-bindgen-go. DO NOT EDIT.

//go:build !wasip1

// Package exit represents the interface "wasi:cli/exit@0.2.0".
package exit

import (
	"github.com/ydnar/wasm-tools-go/cm"
)

// Exit represents function "wasi:cli/exit@0.2.0#exit".
//
// Exit the current instance and any linked instances.
//
//	exit: func(status: result)
//
//go:nosplit
func Exit(status cm.Result) {
	wasmimport_Exit(status)
}

//go:wasmimport wasi:cli/exit@0.2.0 exit
//go:noescape
func wasmimport_Exit(status cm.Result)
