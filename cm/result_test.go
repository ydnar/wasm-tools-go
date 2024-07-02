package cm

import (
	"runtime"
	"testing"
	"unsafe"
)

var (
	_ resulter[string, bool] = &OKResult[string, bool]{}
	_ resulter[bool, string] = &ErrResult[bool, string]{}
)

type resulter[OK, Err any] interface {
	IsOK() bool
	IsErr() bool
	OK() *OK
	Err() *Err
}

func TestResultLayout(t *testing.T) {
	// 8 on 64-bit, 4 on 32-bit
	ptrSize := unsafe.Sizeof(uintptr(0))

	tests := []struct {
		name   string
		r      ResultDebug
		size   uintptr
		offset uintptr
	}{
		{"result", Result(false), 1, 0},
		{"ok", Result(ResultOK), 1, 0},
		{"err", Result(ResultErr), 1, 0},

		{"result<string, string>", OKResult[string, string]{}, sizePlusAlignOf[string](), ptrSize},
		{"result<bool, string>", ErrResult[bool, string]{}, sizePlusAlignOf[string](), ptrSize},
		{"result<string, _>", OKResult[string, struct{}]{}, sizePlusAlignOf[string](), ptrSize},
		{"result<_, string>", ErrResult[struct{}, string]{}, sizePlusAlignOf[string](), ptrSize},
		{"result<u64, u64>", OKResult[uint64, uint64]{}, 16, alignOf[uint64]()},
		{"result<u32, u64>", ErrResult[uint32, uint64]{}, 16, alignOf[uint64]()},
		{"result<u64, u32>", OKResult[uint64, uint32]{}, 16, alignOf[uint64]()},
		{"result<u8, u64>", ErrResult[uint8, uint64]{}, 16, alignOf[uint64]()},
		{"result<u64, u8>", OKResult[uint64, uint8]{}, 16, alignOf[uint64]()},
		{"result<u8, u32>", ErrResult[uint8, uint32]{}, 8, alignOf[uint32]()},
		{"result<u32, u8>", OKResult[uint32, uint8]{}, 8, alignOf[uint32]()},
		{"result<[9]u8, u64>", OKResult[[9]byte, uint64]{}, 24, alignOf[uint64]()},

		{"result<string, _>", OKResult[string, struct{}]{}, sizePlusAlignOf[string](), ptrSize},
		{"result<string, _>", OKResult[string, struct{}]{}, sizePlusAlignOf[string](), ptrSize},
		{"result<string, bool>", OKResult[string, bool]{}, sizePlusAlignOf[string](), ptrSize},
		{"result<[9]u8, u64>", OKResult[[9]byte, uint64]{}, 24, alignOf[uint64]()},

		{"result<_, string>", ErrResult[struct{}, string]{}, sizePlusAlignOf[string](), ptrSize},
		{"result<_, string>", ErrResult[struct{}, string]{}, sizePlusAlignOf[string](), ptrSize},
		{"result<bool, string>", ErrResult[bool, string]{}, sizePlusAlignOf[string](), ptrSize},
		{"result<u64, [9]u8>", ErrResult[uint64, [9]byte]{}, 24, alignOf[uint64]()},
	}

	for _, tt := range tests {
		typ := typeName(tt.r)
		t.Run(tt.name, func(t *testing.T) {
			if got, want := tt.r.Size(), tt.size; got != want {
				t.Errorf("(%s).Size(): %v, expected %v", typ, got, want)
			}
			if got, want := tt.r.DataOffset(), tt.offset; got != want {
				t.Errorf("(%s).DataOffset(): %v, expected %v", typ, got, want)
			}
		})
	}
}

func TestResultOKOrErr(t *testing.T) {
	r1 := OK[OKResult[string, struct{}]]("hello")
	if ok := r1.OK(); ok == nil {
		t.Errorf("OK(): %v, expected non-nil OK", ok)
	}
	if err := r1.Err(); err != nil {
		t.Errorf("Err(): %v, expected nil Err", err)
	}

	r2 := Err[ErrResult[struct{}, bool]](true)
	if ok := r2.OK(); ok != nil {
		t.Errorf("OK(): %v, expected nil OK", ok)
	}
	if err := r2.Err(); err == nil {
		t.Errorf("Err(): %v, expected non-nil Err", err)
	}
}

func TestAltResult1(t *testing.T) {
	type alt1[Shape, OK, Err any] struct {
		_     [0]OK
		_     [0]Err
		data  Shape
		isErr bool
	}

	equalSize(t, result[uint8, struct{}, uint8]{}, alt1[uint8, struct{}, uint8]{})
	equalSize(t, result[uint16, struct{}, uint16]{}, alt1[uint16, struct{}, uint16]{})
	equalSize(t, result[uint32, struct{}, uint32]{}, alt1[uint32, struct{}, uint32]{})
	equalSize(t, result[uint64, struct{}, uint64]{}, alt1[uint64, struct{}, uint64]{})
	equalSize(t, result[uint64, [5]uint8, uint64]{}, alt1[uint64, [5]uint8, uint64]{})
	equalSize(t, result[uint64, [6]uint8, uint64]{}, alt1[uint64, [6]uint8, uint64]{})

	// Go adds padding to structs with zero-length trailing fields.
	// TinyGo does not.
	if runtime.Compiler != "tinygo" {
		unequalSize(t, result[struct{}, struct{}, struct{}]{}, alt1[struct{}, struct{}, struct{}]{})

		// zero-length arrays have alignment of their associated type
		// TODO: document why zero-length arrays are not allowed as result or variant associated types
		unequalSize(t, result[[0]uint64, [0]uint64, struct{}]{}, alt1[[0]uint64, [0]uint64, struct{}]{})
	}
}

func equalSize[A, B any](t *testing.T, a A, b B) {
	if unsafe.Sizeof(a) != unsafe.Sizeof(b) {
		t.Errorf("unsafe.Sizeof(%T) (%d) != unsafe.Sizeof(%T) (%d)", a, unsafe.Sizeof(a), b, unsafe.Sizeof(b))
	}
}

func unequalSize[A, B any](t *testing.T, a A, b B) {
	if unsafe.Sizeof(a) == unsafe.Sizeof(b) {
		t.Errorf("unsafe.Sizeof(%T) (%d) == unsafe.Sizeof(%T) (%d)", a, unsafe.Sizeof(a), b, unsafe.Sizeof(b))
	}
}

func BenchmarkResultInlines(b *testing.B) {
	var ok *struct{}
	var err *string
	r1 := Err[ErrResult[struct{}, string]]("hello")
	for i := 0; i < b.N; i++ {
		ok = r1.OK()
	}
	_ = ok
	_ = err
}

func TestIssue95(t *testing.T) {
	want := "hello"
	res := issue95(false, want)
	got := *res.OK()
	if got != want {
		t.Errorf("*res.OK(): %s, expected %s", got, want)
	}
}

func issue95(isErr bool, v string) stringResult {
	if isErr {
		err := New[stringVariant](0, v)
		return Err[stringResult](err)
	}
	return OK[stringResult](v)
}

type stringResult ErrResult[string, stringVariant]

type stringVariant Variant[uint8, string, string]
