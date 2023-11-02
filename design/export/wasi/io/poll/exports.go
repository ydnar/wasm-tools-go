//go:build wasm

package poll

import (
	"unsafe"
)

// Interface implements WIT interface "wasi:io/poll".
type Interface interface {
	Poll(in []Pollable) []uint32
	Pollable() interface {
		Block(self Pollable)
		Ready(self Pollable) bool
	}
}

// Pollable implements WIT type "wasi:io/poll.pollable".
type Pollable uint32

// Export registers a concrete implementation of WIT interface "wasi:io/poll".
func Export(i Interface) {
	impl = i
}

// TODO: make a default implementation that panics with a helpful message on all function calls.
var impl Interface

//go:wasmexport wasi:io/poll poll
func wasmexport_poll(data *Pollable, size uint32) (*uint32, uint32) {
	s := impl.Poll(unsafe.Slice(data, size))
	return unsafe.SliceData(s), uint32(len(s)) // FIXME: will this work with Go GC?
}

//go:wasmexport wasi:io/poll [method]pollable.block
func wasmexport_method_pollable_block(self Pollable) {
	impl.Pollable().Block(self)
}

//go:wasmexport wasi:io/poll [method]pollable.ready
func wasmexport_method_pollable_ready(self Pollable) bool {
	return impl.Pollable().Ready(self)
}
