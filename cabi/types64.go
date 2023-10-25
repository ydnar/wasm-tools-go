//go:build !wasm32 && !tinygo.wasm

package cabi

import (
	"unsafe"
)

type alloc32 = uint32
type alloc64 = unsafe.Pointer
type alloc128 = [2]unsafe.Pointer
