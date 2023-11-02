//go:build wasm

package poll

import (
	"unsafe"
)

// Exports (if wasi:io/poll was exported)

var Instance Interface

//go:wasmexport wasi:io/poll poll
func wasmexport_poll(data *Pollable, size uint32) (*uint32, uint32) {
	s := Instance.Poll(unsafe.Slice(data, size))
	return unsafe.SliceData(s), uint32(len(s)) // FIXME: will this work with Go GC?
}

//go:wasmexport wasi:io/poll [method]pollable.block
func wasmexport_method_pollable_block(self Pollable) {
	// TODO
}

//go:wasmexport wasi:io/poll [method]pollable.ready
func wasmexport_method_pollable_ready(self Pollable) bool {
	// TODO
	return false
}
