package cm

type Bool bool
type S8 int8
type U8 uint8
type S16 int16
type U16 uint16
type S32 int32
type U32 uint32
type S64 int64
type U64 uint64
type F32 float32
type F64 float64
type Char rune
type String string

// Resource is the interface implemented by all [resource] types.
type Resource[T any] interface {
	ResourceHandle() Handle[T]
	// BorrowResource() Borrow[T]
	// OwnResource() Own[T]
}

// Handle is an opaque handle to a [resource].
type Handle[T any] uint32

// Borrow is a handle to a borrowed [resource].
type Borrow[T any] Handle[T]

func (b Borrow[T]) Rep() T {
	return Rep(Handle[T](b))
}

// Own is a handle to an owned [resource].
type Own[T any] Handle[T]

func (o Own[T]) Rep() T {
	return Rep(Handle[T](o))
}

// TODO: can we use finalizers for dropping handles?

// Rep returns the representation of handle, if any.
func Rep[T any, H Handle[T]](handle H) T {
	// TODO: extract the actual representation from a table
	var v T
	return v
}
