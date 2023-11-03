package exports

import (
	"unsafe"

	"github.com/ydnar/wasm-tools-go/design/wasi/io/poll"
	"github.com/ydnar/wasm-tools-go/wasm/cabi"
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
func wasmexport_poll(inData *cabi.Borrow[poll.Pollable], inLen uint32) (*uint32, uint32) {
	in := unsafe.Slice(inData, inLen)
	in_ := make([]poll.Pollable, len(in))
	for i := range in {
		in_[i] = in[i].Rep()
	}
	s := impl.Poll(in_)
	// TODO: will this work with Go GC?
	sData := unsafe.SliceData(s)
	cabi.KeepAlive(unsafe.Pointer(sData))
	return sData, uint32(len(s))
}

//go:wasmexport wasi:io/poll cabi_post_poll
func wasmexport_cabi_post_poll(a0 *uint32, a1 uint32) {
	cabi.Drop(unsafe.Pointer(a0))
}

//go:wasmexport wasi:io/poll [method]pollable.block
func wasmexport_method_pollable_block(self cabi.Borrow[poll.Pollable]) {
	self.Rep().Block()
}

//go:wasmexport wasi:io/poll [method]pollable.ready
func wasmexport_method_pollable_ready(self cabi.Borrow[poll.Pollable]) bool {
	return self.Rep().Ready()
}
