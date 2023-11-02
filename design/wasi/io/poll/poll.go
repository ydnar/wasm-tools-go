//go:build wasm

package poll

import (
	"runtime"
	"unsafe"

	"github.com/ydnar/wasm-tools-go/wasm/cm"
)

/*
package wasi:io;

interface poll {
    resource pollable {
        block: func();
        ready: func() -> bool;
    }
    poll: func(in: list<borrow<pollable>>) -> list<u32>;
}
*/

type Interface interface {
	// Poll(in wasi.List[wasi.Borrow[Pollable]]) wasi.List[uint32]
	// Poll(in []wasi.Borrow[Pollable]) []uint32
	// Poll(in []BorrowedPollable) []uint32
	Poll(in []Pollable) []uint32
}

func Poll(in []Pollable) []uint32 {
	ptr, n := wasmimport_poll(unsafe.SliceData(in), uint32(len(in)))
	runtime.KeepAlive(in) // TODO: necessary?
	return unsafe.Slice(ptr, n)
}

// FIXME: what is the right function signature for this?
//
//go:wasmimport wasi:io/poll poll
func wasmimport_poll(data *Pollable, len uint32) (*uint32, int32)

type Pollable cm.Resource

// Imports

func (self Pollable) Block() {
	wasmimport_pollable_block()
}

//go:wasmimport wasi:io/poll [method]pollable.block
func wasmimport_pollable_block()

func (self Pollable) Ready() bool {
	return wasmimport_pollable_ready()
}

//go:wasmimport wasi:io/poll [method]pollable.ready
func wasmimport_pollable_ready() bool

// Exports

//go:wasmexport wasi:io/poll [method]pollable.block
func wasmexport_pollable_block(self Pollable)

//go:wasmexport wasi:io/poll [method]pollable.ready
func wasmexport_pollable_ready(self Pollable) bool
