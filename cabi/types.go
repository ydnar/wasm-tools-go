package cabi

import "unsafe"

func init() {
	var a32 alloc32
	if unsafe.Sizeof(a32) != 4 {
		panic("sizeof alloc32 != 4")
	}
	var a64 alloc64
	if unsafe.Sizeof(a64) != 8 {
		panic("sizeof alloc64 != 8")
	}
	var a128 alloc128
	if unsafe.Sizeof(a128) != 16 {
		panic("sizeof alloc128 != 16")
	}
}
