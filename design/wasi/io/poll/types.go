package poll

import "github.com/ydnar/wasm-tools-go/wasm/cm"

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

// Interface is the Go implementation of WIT interface "wasi:io/poll".
type Interface interface {
	// Poll(in wasi.List[wasi.Borrow[Pollable]]) wasi.List[uint32]
	// Poll(in []wasi.Borrow[Pollable]) []uint32
	// Poll(in []BorrowedPollable) []uint32
	Poll(in []Pollable) []uint32
}

// Pollable is the Go implementation of WIT type "wasi:io/poll.pollable".
type Pollable cm.Resource
