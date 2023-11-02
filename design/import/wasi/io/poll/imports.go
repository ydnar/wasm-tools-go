//go:build wasm

package poll

import (
	"runtime"
	"unsafe"
)

// Imports (if wasi:io/poll was imported)

// Poll is the Go implemention of WIT func "wasi:io/poll.poll".
func Poll(in []Pollable) []uint32 {
	ptr, size := wasmimport_poll(unsafe.SliceData(in), uint32(len(in)))
	runtime.KeepAlive(in) // TODO: necessary?
	return unsafe.Slice(ptr, size)
}

// FIXME: what is the right function signature for this?
//
//go:wasmimport wasi:io/poll poll
func wasmimport_poll(data *Pollable, size uint32) (*uint32, int32)

// Block is the Go implemention of WIT method "wasi:io/poll.pollable.block".
func (self Pollable) Block() {
	wasmimport_pollable_block()
}

//go:wasmimport wasi:io/poll [method]pollable.block
func wasmimport_pollable_block()

// Ready is the Go implemention of WIT method "wasi:io/poll.pollable.ready".
func (self Pollable) Ready() bool {
	return wasmimport_pollable_ready()
}

//go:wasmimport wasi:io/poll [method]pollable.ready
func wasmimport_pollable_ready() bool
