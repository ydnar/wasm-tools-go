package cabi

import (
	"slices"
	"testing"
	"unsafe"
)

func TestRealloc(t *testing.T) {
	const threshold = 16

	tests := []struct {
		name    string
		ptr     uintptr
		size    uintptr
		align   uintptr
		newsize uintptr
		want    uintptr
	}{
		{"nil", 0, 0, 1, 0, 0},
		{"nil with align", 0, 0, 2, 0, 0},
		{"align to 2", 1, 0, 2, 0, 2},
		{"align to 2", 1, 0, 2, 0, 2},
		{"align to 4", 1, 0, 4, 0, 4},
		{"align to 8", 3, 0, 8, 0, 8},
		{"align to 8", 9, 0, 16, 0, 16},
		{"alloc 100 bytes", 0, 0, 1, 100, sliceData(hundred)},
		{"preserve 5 bytes", stringData("hello"), 5, 1, 5, stringData("hello")},
		{"shrink 8 bytes to 4", stringData("fourfour"), 8, 1, 4, stringData("four")},
		{"expand 4 bytes to 8", stringData("zero"), 4, 1, 8, stringData("zero\u0000\u0000\u0000\u0000")},
		{"cut down lorem ipsum", stringData(lorem), 4, 1, 200, stringData(lorem[:200])},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := realloc(unsafePointer(tt.ptr), tt.size, tt.align, tt.newsize)
			if (tt.want < threshold && uintptr(got) != tt.want) || (tt.want >= threshold && uintptr(got) < threshold) {
				t.Errorf("realloc(%d, %d, %d, %d): expected %d, got %d",
					tt.ptr, tt.size, tt.align, tt.newsize, tt.want, got)
			}
			if uintptr(got)%tt.align != 0 {
				t.Errorf("expected ptr aligned to %d, got %d", tt.align, uintptr(got)&tt.align)
			}
			if uintptr(got) < threshold {
				return // it didn’t allocate
			}
			if tt.ptr == 0 {
				wants := unsafe.Slice((*byte)(unsafePointer(tt.want)), tt.newsize)
				gots := unsafe.Slice((*byte)(got), tt.newsize)
				if slices.Compare(wants, gots) != 0 {
					t.Errorf("expected %v, got %v", wants, gots)
				}
			}
		})
	}
}

var hundred = make([]byte, 100)
var lorem = `Lorem ipsum dolor sit amet, consectetur adipiscing elit. Ut volutpat arcu eu est tristique suscipit. Nulla laoreet purus magna, at feugiat tortor fermentum non. Integer semper et magna id placerat. Quisque purus lorem, mollis vel convallis eu, ullamcorper sit amet justo. Duis tempus gravida lacus, vel dapibus augue. Nunc sed condimentum lacus. Cras vulputate cursus dictum. Etiam felis metus, volutpat id luctus ac, ultrices nec metus. Proin sagittis nulla a pretium sagittis. Nullam tristique sapien sed semper faucibus. Fusce condimentum nulla dui. Donec egestas nunc in blandit mollis.`

// Appease vet, see https://github.com/golang/go/issues/58625
func unsafePointer(p uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&p))
}

func sliceData[T any](s []T) uintptr {
	return uintptr(unsafe.Pointer(unsafe.SliceData(s)))
}

func stringData(s string) uintptr {
	return uintptr(unsafe.Pointer(unsafe.StringData(s)))
}
