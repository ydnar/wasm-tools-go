package exports

import (
	"unsafe"

	"github.com/ydnar/wasm-tools-go/design/wasi/io/poll"
	"github.com/ydnar/wasm-tools-go/wasm/cm"
)

// Interface implements the Component Model interface "wasi:io/poll".
type Interface interface {
	Poll(in []poll.Pollable) []uint32
	Pollable() interface {
		// constructor, static functions would go here
	}
}

// Export registers a concrete implementation of "wasi:io/poll".
func Export(i Interface) {
	impl = i
}

// TODO: make a default implementation that panics with a helpful message on all function calls.
var impl Interface

//go:wasmexport wasi:io/poll poll
func wasmexport_poll(data *cm.Borrow[poll.Pollable], size uint32) (*uint32, uint32) {
	in := unsafe.Slice(data, size)
	in_ := make([]poll.Pollable, len(in))
	for i := range in {
		in_[i] = in[i].Rep()
	}
	s := impl.Poll(in_)
	return unsafe.SliceData(s), uint32(len(s)) // FIXME: will this work with Go GC?
}

//go:wasmexport wasi:io/poll [method]pollable.block
func wasmexport_method_pollable_block(self cm.Borrow[poll.Pollable]) {
	self.Rep().Block()
}

//go:wasmexport wasi:io/poll [method]pollable.ready
func wasmexport_method_pollable_ready(self cm.Borrow[poll.Pollable]) bool {
	return self.Rep().Ready()
}
