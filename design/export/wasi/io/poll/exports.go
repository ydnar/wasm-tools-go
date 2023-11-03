package poll

import (
	"unsafe"

	"github.com/ydnar/wasm-tools-go/wasm/cm"
)

// Interface implements WIT interface "wasi:io/poll".
type Interface interface {
	Poll(in []cm.Borrow[Pollable]) []uint32
	Pollable() PollableInterface
}

type PollableInterface interface {
	Own(cm.Own[Pollable]) PollableMethods
	Borrow(cm.Borrow[Pollable]) PollableMethods
}

type PollableMethods interface {
	Block()
	Ready() bool
}

// Pollable implements WIT type "wasi:io/poll.pollable".
type Pollable cm.Resource

// Export registers a concrete implementation of WIT interface "wasi:io/poll".
func Export(i Interface) {
	impl = i
}

// TODO: make a default implementation that panics with a helpful message on all function calls.
var impl Interface

//go:wasmexport wasi:io/poll poll
func wasmexport_poll(data *cm.Borrow[Pollable], size uint32) (*uint32, uint32) {
	s := impl.Poll(unsafe.Slice(data, size))
	return unsafe.SliceData(s), uint32(len(s)) // FIXME: will this work with Go GC?
}

//go:wasmexport wasi:io/poll [method]pollable.block
func wasmexport_method_pollable_block(self cm.Borrow[Pollable]) {
	impl.Pollable().Borrow(self).Block()
}

//go:wasmexport wasi:io/poll [method]pollable.ready
func wasmexport_method_pollable_ready(self cm.Borrow[Pollable]) bool {
	return impl.Pollable().Borrow(self).Ready()
}
