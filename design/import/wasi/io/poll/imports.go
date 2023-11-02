//go:build wasm

package poll

import (
	"runtime"
	"unsafe"
)

// Pollable imports WIT type "wasi:io/poll.pollable".
type Pollable uint32

// Poll imports WIT func "wasi:io/poll.poll".
func Poll(in []Pollable) []uint32 {
	ptr, size := wasmimport_poll(unsafe.SliceData(in), uint32(len(in)))
	runtime.KeepAlive(in) // TODO: necessary?
	return unsafe.Slice(ptr, size)
}

//go:wasmimport wasi:io/poll poll
func wasmimport_poll(data *Pollable, size uint32) (*uint32, int32)

// Block imports WIT method "wasi:io/poll.pollable.block".
func (self Pollable) Block() {
	wasmimport_pollable_block(self)
}

//go:wasmimport wasi:io/poll [method]pollable.block
func wasmimport_pollable_block(self Pollable)

// Ready imports WIT method "wasi:io/poll.pollable.ready".
func (self Pollable) Ready() bool {
	return wasmimport_pollable_ready(self)
}

//go:wasmimport wasi:io/poll [method]pollable.ready
func wasmimport_pollable_ready(self Pollable) bool
