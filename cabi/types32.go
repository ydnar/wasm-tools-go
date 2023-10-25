//go:build wasm32 || tinygo.wasm

package cabi

import "unsafe"

type alloc32 = unsafe.Pointer
type alloc64 = [2]unsafe.Pointer
type alloc128 = [4]unsafe.Pointer
