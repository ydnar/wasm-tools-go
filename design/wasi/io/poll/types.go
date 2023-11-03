package poll

import "github.com/ydnar/wasm-tools-go/wasm/cm"

// Pollable represents the Component Model type "wasi:io/poll.pollable".
type Pollable interface {
	Block()
	Ready() bool
	cm.Resource[Pollable]
}
