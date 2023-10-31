package wit

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unsafe"

	"github.com/coreos/go-semver/semver"
)

// Resolve represents a fully resolved set of WIT ([WebAssembly Interface Type])
// packages and worlds. It implements the [Node] interface.
//
// This structure contains a graph of WIT packages and their contents
// merged together into slices organized by type. Items are sorted
// topologically and everything is fully resolved.
//
// Each [World], [Interface], [TypeDef], or [Package] in a Resolve must be non-nil.
//
// [WebAssembly Interface Type]: https://component-model.bytecodealliance.org/design/wit.html
type Resolve struct {
	Worlds     []*World
	Interfaces []*Interface
	TypeDefs   []*TypeDef
	Packages   []*Package
}

// A World represents all of the imports and exports of a [WebAssembly component].
// It implements the [Node] and [TypeOwner] interfaces.
//
// [WebAssembly component]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/WIT.md#wit-worlds
type World struct {
	Name    string
	Imports map[string]WorldItem
	Exports map[string]WorldItem

	// The [Package] that this World belongs to. It must be non-nil when fully resolved.
	Package *Package
	Docs    Docs

	_typeOwner
}

// Path returns the fully-qualified path for this world, such as "wasi:clocks/imports".
func (w *World) Path() string {
	return w.Package.Name.String() + "/" + w.Name
}

// AllFunctions [iterates] through all functions exported from or imported into a [World],
// calling yield for each. Iteration will stop if yield returns false.
//
// [iterates]: https://github.com/golang/go/issues/61897
func (w *World) AllFunctions(yield func(*Function) bool) bool {
	return w.AllImports(func(name string, i WorldItem) bool {
		if f, ok := i.(*Function); ok {
			if !yield(f) {
				return false
			}
		}
		return true
	}) && w.AllExports(func(name string, i WorldItem) bool {
		if f, ok := i.(*Function); ok {
			if !yield(f) {
				return false
			}
		}
		return true
	})
}

// AllImports [iterates] through all imports in a [World] in a deterministic order,
// calling yield for each. Iteration will stop if yield returns false.
//
// [iterates]: https://github.com/golang/go/issues/61897
func (w *World) AllImports(yield func(name string, i WorldItem) bool) bool {
	return iterateWorldItems(w.Imports, yield)
}

// AllExports [iterates] through all exports in a [World] in a deterministic order,
// calling yield for each. Iteration will stop if yield returns false.
//
// [iterates]: https://github.com/golang/go/issues/61897
func (w *World) AllExports(yield func(name string, i WorldItem) bool) bool {
	return iterateWorldItems(w.Exports, yield)
}

// iterateWorldItems sorts a map of [WorldItem] by type, then name,
// calling yield for each item.
func iterateWorldItems(m map[string]WorldItem, yield func(name string, i WorldItem) bool) bool {
	type named struct {
		name     string
		sortName string
		item     WorldItem
	}
	items := make([]named, 0, len(m))
	for name, v := range m {
		item := named{name, name, v}
		// TODO: add WorldItem.ItemName() method or something
		switch v := v.(type) {
		case *Interface:
			if v.Name != nil {
				item.sortName = *v.Name
			}
		case *TypeDef:
			if v.Name != nil {
				item.sortName = *v.Name
			}
		case *Function:
			item.sortName = v.Name
		}
		items = append(items, item)
	}

	// Sort slice
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i], items[j]
		as, bs := worldItemTypeSort(a.item), worldItemTypeSort(b.item)
		switch {
		case as < bs:
			return true
		case as > bs:
			return false
		case a.sortName < b.sortName:
			return true
		case a.sortName > b.sortName:
			return false
		}
		return a.name < b.name
	})

	// Iterate
	for _, item := range items {
		if !yield(item.name, item.item) {
			return false
		}
	}
	return true
}

func worldItemTypeSort(i WorldItem) int {
	switch i.(type) {
	case *Interface:
		return 0
	case *TypeDef:
		return 1
	case *Function:
		return 2
	}
	panic("BUG: unknown WorldItem type")
}

// A WorldItem is any item that can be exported from or imported into a [World],
// currently either an [Interface], [TypeDef], or [Function].
// Any WorldItem is also a [Node].
type WorldItem interface {
	Node
	isWorldItem()
}

// _worldItem is an embeddable type that conforms to the [WorldItem] interface.
type _worldItem struct{ _node }

func (_worldItem) isWorldItem() {}

// An Interface represents a [collection of types and functions], which are imported into
// or exported from a [WebAssembly component].
// It implements the [Node], [TypeOwner], and [WorldItem] interfaces.
//
// [collection of types and functions]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/WIT.md#wit-interfaces.
// [WebAssembly component]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/WIT.md#wit-worlds
type Interface struct {
	Name      *string
	TypeDefs  map[string]*TypeDef
	Functions map[string]*Function

	// The [Package] that this Interface belongs to. It must be non-nil when fully resolved.
	Package *Package
	Docs    Docs

	_typeOwner
	_worldItem
}

// AllFunctions [iterates] through all functions in [Interface] i, calling yield for each.
// Iteration will stop if yield returns false.
//
// [iterates]: https://github.com/golang/go/issues/61897
func (i *Interface) AllFunctions(yield func(*Function) bool) bool {
	for _, f := range i.Functions {
		if !yield(f) {
			return false
		}
	}
	return true
}

// TypeDef represents a WIT type definition. A TypeDef may be named or anonymous,
// and optionally belong to a [World] or [Interface].
// It implements the [Node], [Sized], [Type], [TypeDefKind] interfaces.
type TypeDef struct {
	Name  *string
	Kind  TypeDefKind
	Owner TypeOwner
	Docs  Docs

	_worldItem
	_type
}

// Root returns the root [TypeDef] of [type alias] t.
// If t is not a type alias, Root returns t.
//
// [type alias]: https://component-model.bytecodealliance.org/design/wit.html#type-aliases
func (t *TypeDef) Root() *TypeDef {
	for {
		switch kind := t.Kind.(type) {
		case *TypeDef:
			t = kind
		default:
			return t
		}
	}
}

// Package returns the [Package] that t is associated with, if any.
func (t *TypeDef) Package() *Package {
	switch owner := t.Owner.(type) {
	case *Interface:
		return owner.Package
	case *World:
		return owner.Package
	}
	return nil
}

// Size returns the byte size for values of type t.
func (t *TypeDef) Size() uintptr {
	return t.Kind.Size()
}

// Align returns the byte alignment for values of type t.
func (t *TypeDef) Align() uintptr {
	return t.Kind.Align()
}

// Constructor returns the constructor for [TypeDef] t, or nil if none.
// Currently t must be a [Resource] to have a constructor.
func (t *TypeDef) Constructor() *Function {
	var constructor *Function
	t.Owner.AllFunctions(func(f *Function) bool {
		if c, ok := f.Kind.(*Constructor); ok && c.Type == t {
			constructor = f
			return false
		}
		return true
	})
	return constructor
}

// Methods returns all methods for [TypeDef] t.
// Currently t must be a [Resource] to have methods.
func (t *TypeDef) Methods() []*Function {
	var methods []*Function
	t.Owner.AllFunctions(func(f *Function) bool {
		if m, ok := f.Kind.(*Method); ok && m.Type == t {
			methods = append(methods, f)
		}
		return true
	})
	return methods
}

// StaticFunctions returns all static functions for [TypeDef] t.
// Currently t must be a [Resource] to have static functions.
func (t *TypeDef) StaticFunctions() []*Function {
	var statics []*Function
	t.Owner.AllFunctions(func(f *Function) bool {
		if s, ok := f.Kind.(*Static); ok && s.Type == t {
			statics = append(statics, f)
		}
		return true
	})
	return statics
}

// TypeDefKind represents the underlying type in a [TypeDef], which can be one of
// [Record], [Resource], [Handle], [Flags], [Tuple], [Variant], [Enum],
// [Option], [Result], [List], [Future], [Stream], or [Type].
// It implements the [Node] and [Sized] interfaces.
type TypeDefKind interface {
	Node
	Sized
	isTypeDefKind()
}

// _typeDefKind is an embeddable type that conforms to the [TypeDefKind] interface.
type _typeDefKind struct {
	_node
	_sized
}

func (_typeDefKind) isTypeDefKind() {}

// Record represents a WIT [record type], akin to a struct.
// It implements the [Node], [Sized], and [TypeDefKind] interfaces.
//
// [record type]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/WIT.md#item-record-bag-of-named-fields
type Record struct {
	Fields []Field
	_typeDefKind
}

// Size returns the [ABI byte size] for [Record] r.
//
// [ABI byte size]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#size
func (r *Record) Size() uintptr {
	var s uintptr
	for _, f := range r.Fields {
		s = Align(s, f.Type.Align())
		s += f.Type.Size()
	}
	return s
}

// Align returns the [ABI byte alignment] for [Record] r.
//
// [ABI byte alignment]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#alignment
func (r *Record) Align() uintptr {
	var a uintptr = 1
	for _, f := range r.Fields {
		a = max(a, f.Type.Align())
	}
	return a
}

// Field represents a field in a [Record].
type Field struct {
	Name string
	Type Type
	Docs Docs
}

// Resource represents a WIT [resource type].
// It implements the [Node], [Sized], and [TypeDefKind] interfaces.
//
// [resource type]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/WIT.md#item-resource
type Resource struct{ _typeDefKind }

// Size returns the [ABI byte size] for [Resource] r.
//
// [ABI byte size]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#size
func (r *Resource) Size() uintptr { return 4 }

// Align returns the [ABI byte alignment] for [Resource] r.
//
// [ABI byte alignment]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#alignment
func (r *Resource) Align() uintptr { return 4 }

// Handle represents a WIT [handle type].
// It conforms to the [Node], [Sized], and [TypeDefKind] interfaces.
// Handles represent the passing of unique ownership of a resource between
// two components. When the owner of an owned handle drops that handle,
// the resource is destroyed. In contrast, a borrowed handle represents
// a temporary loan of a handle from the caller to the callee for the
// duration of the call.
//
// [handle type]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/WIT.md#handles
type Handle interface {
	TypeDefKind
	isHandle()
}

// _handle is an embeddable type that conforms to the [Handle] interface.
type _handle struct{ _typeDefKind }

func (_handle) isHandle() {}

// Size returns the [ABI byte size] for this [Handle].
//
// [ABI byte size]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#size
func (_handle) Size() uintptr { return 4 }

// Align returns the [ABI byte alignment] for this [Handle].
//
// [ABI byte alignment]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#alignment
func (_handle) Align() uintptr { return 4 }

// OwnedHandle represents an WIT [owned handle].
// It implements the [Handle], [Node], [Sized], and [TypeDefKind] interfaces.
//
// [owned handle]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/WIT.md#handles
type OwnedHandle struct {
	Type *TypeDef
	_handle
}

// BorrowedHandle represents a WIT [borrowed handle].
// It implements the [Handle], [Node], [Sized], and [TypeDefKind] interfaces.
//
// [borrowed handle]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/WIT.md#handles
type BorrowedHandle struct {
	Type *TypeDef
	_handle
}

// Flags represents a WIT [flags type], stored as a bitfield.
// It implements the [Node], [Sized], and [TypeDefKind] interfaces.
//
// [flags type]: https://component-model.bytecodealliance.org/design/wit.html#flags
type Flags struct {
	Flags []Flag
	_typeDefKind
}

// Size returns the [ABI byte size] of [Flags] f.
//
// [ABI byte size]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#size
func (f *Flags) Size() uintptr {
	n := len(f.Flags)
	switch {
	case n <= 8:
		return 1
	case n <= 16:
		return 2
	}
	return 4 * uintptr((n+31)>>5)
}

// Align returns the [ABI byte alignment] of [Flags] f.
//
// [ABI byte alignment]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#alignment
func (f *Flags) Align() uintptr {
	n := len(f.Flags)
	switch {
	case n <= 8:
		return 1
	case n <= 16:
		return 2
	}
	return 4
}

// Flag represents a single flag value in a [Flags] type.
// It implements the [Node] interface.
type Flag struct {
	Name string
	Docs Docs
}

// Tuple represents a WIT [tuple type].
// A tuple type is an ordered fixed length sequence of values of specified types.
// It is similar to a [Record], except that the fields are identified by their order instead of by names.
// It implements the [Node], [Sized], and [TypeDefKind] interfaces.
//
// [tuple type]: https://component-model.bytecodealliance.org/design/wit.html#tuples
type Tuple struct {
	Types []Type
	_typeDefKind
}

// Despecialize despecializes [Tuple] e into a [Record] with 0-based integer field names.
// See the [canonical ABI documentation] for more information.
//
// [canonical ABI documentation]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#despecialization
func (t *Tuple) Despecialize() TypeDefKind {
	r := &Record{
		Fields: make([]Field, len(t.Types)),
	}
	for i := range t.Types {
		r.Fields[i].Name = strconv.Itoa(i)
		r.Fields[i].Type = t.Types[i]
	}
	return r
}

// Size returns the [ABI byte size] for [Tuple] t.
// It is first [despecialized] into a [Record] with 0-based integer field names, then sized.
//
// [ABI byte size]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#size
// [despecialized]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#despecialization
func (t *Tuple) Size() uintptr {
	return t.Despecialize().Size()
}

// Align returns the [ABI byte alignment] for [Tuple] t.
// It is first [despecialized] into a [Record] with 0-based integer field names, then aligned.
//
// [ABI byte alignment]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#alignment
// [despecialized]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#despecialization
func (t *Tuple) Align() uintptr {
	return t.Despecialize().Align()
}

// Variant represents a WIT [variant type], a tagged/discriminated union.
// A variant type declares one or more cases. Each case has a name and, optionally,
// a type of data associated with that case.
// It implements the [Node], [Sized], and [TypeDefKind] interfaces.
//
// [variant type]: https://component-model.bytecodealliance.org/design/wit.html#variants
type Variant struct {
	Cases []Case
	_typeDefKind
}

// Size returns the [ABI byte size] for [Variant] v.
//
// [ABI byte size]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#size
func (v *Variant) Size() uintptr {
	s := Discriminant(uint32(len(v.Cases))).Size()
	s = Align(s, v.maxCaseAlign())
	s += v.maxCaseSize()
	return Align(s, v.Align())
}

// Align returns the [ABI byte alignment] for [Variant] v.
//
// [ABI byte alignment]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#alignment
func (v *Variant) Align() uintptr {
	return max(Discriminant(uint32(len(v.Cases))).Align(), v.maxCaseAlign())
}

func (v *Variant) maxCaseSize() uintptr {
	var s uintptr
	for _, c := range v.Cases {
		if c.Type != nil {
			s = max(s, c.Type.Size())
		}
	}
	return s
}

func (v *Variant) maxCaseAlign() uintptr {
	var a uintptr = 1
	for _, c := range v.Cases {
		if c.Type != nil {
			a = max(a, c.Type.Align())
		}
	}
	return a
}

// Case represents a single case in a [Variant].
// It implements the [Node] interface.
type Case struct {
	Name string
	Type Type // optional associated Type (can be nil)
	Docs Docs
}

// Enum represents a WIT [enum type], which is a [Variant] without associated data.
// The equivalent in Go is a set of const identifiers declared with iota.
// It implements the [Node], [Sized], and [TypeDefKind] interfaces.
//
// [enum type]: https://component-model.bytecodealliance.org/design/wit.html#enums
type Enum struct {
	Cases []EnumCase
	_typeDefKind
}

// Despecialize despecializes [Enum] e into a [Variant] with no associated types.
// See the [canonical ABI documentation] for more information.
//
// [canonical ABI documentation]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#despecialization
func (e *Enum) Despecialize() TypeDefKind {
	v := &Variant{
		Cases: make([]Case, len(e.Cases)),
	}
	for i := range e.Cases {
		v.Cases[i].Name = e.Cases[i].Name
		v.Cases[i].Docs = e.Cases[i].Docs
	}
	return v
}

// Size returns the [ABI byte size] for [Enum] e, the smallest integer
// type that can represent 0...len(e.Cases).
// It is first [despecialized] into a [Variant] with no associated types, then sized.
//
// [ABI byte size]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#size
// [despecialized]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#despecialization
func (e *Enum) Size() uintptr {
	return e.Despecialize().Size()
}

// Align returns the [ABI byte alignment] for [Enum] e.
// It is first [despecialized] into a [Variant] with no associated types, then aligned.
//
// [ABI byte alignment]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#alignment
// [despecialized]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#despecialization
func (e *Enum) Align() uintptr {
	return e.Despecialize().Align()
}

// EnumCase represents a single case in an [Enum].
// It implements the [Node] interface.
type EnumCase struct {
	Name string
	Docs Docs
}

// Option represents a WIT [option type], a special case of [Variant]. An Option can
// contain a value of a single type, either build-in or user defined, or no value.
// The equivalent in Go for an option<string> could be represented as *string.
// It implements the [Node], [Sized], and [TypeDefKind] interfaces.
//
// [option type]: https://component-model.bytecodealliance.org/design/wit.html#options
type Option struct {
	Type Type
	_typeDefKind
}

// Despecialize despecializes [Option] o into a [Variant] with two cases, "none" and "some".
// See the [canonical ABI documentation] for more information.
//
// [canonical ABI documentation]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#despecialization
func (o *Option) Despecialize() TypeDefKind {
	return &Variant{
		Cases: []Case{
			{Name: "none"},
			{Name: "some", Type: o.Type},
		},
	}
}

// Size returns the [ABI byte size] for [Option] o.
// It is first [despecialized] into a [Variant] with two cases, "none" and "some(T)", then sized.
//
// [ABI byte size]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#size
// [despecialized]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#despecialization
func (o *Option) Size() uintptr {
	return o.Despecialize().Size()
}

// Align returns the [ABI byte alignment] for [Option] o.
// It is first [despecialized] into a [Variant] with two cases, "none" and "some(T)", then aligned.
//
// [ABI byte alignment]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#alignment
// [despecialized]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#despecialization
func (o *Option) Align() uintptr {
	return o.Despecialize().Align()
}

// Result represents a WIT [result type], which is the result of a function call,
// returning an optional value and/or an optional error. It is roughly equivalent to
// the Go pattern of returning (T, error).
// It implements the [Node], [Sized], and [TypeDefKind] interfaces.
//
// [result type]: https://component-model.bytecodealliance.org/design/wit.html#results
type Result struct {
	OK  Type // optional associated Type (can be nil)
	Err Type // optional associated Type (can be nil)
	_typeDefKind
}

// Despecialize despecializes [Result] o into a [Variant] with two cases, "ok" and "error".
// See the [canonical ABI documentation] for more information.
//
// [canonical ABI documentation]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#despecialization
func (r *Result) Despecialize() TypeDefKind {
	return &Variant{
		Cases: []Case{
			{Name: "ok", Type: r.OK},
			{Name: "error", Type: r.Err},
		},
	}
}

// Size returns the [ABI byte size] for [Result] r.
// It is first [despecialized] into a [Variant] with two cases "ok" and "error", then sized.
//
// [ABI byte size]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#size
// [despecialized]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#despecialization
func (r *Result) Size() uintptr {
	return r.Despecialize().Size()
}

// Align returns the [ABI byte alignment] for [Result] r.
// It is first [despecialized] into a [Variant] with two cases "ok" and "error", then aligned.
//
// [ABI byte alignment]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#alignment
// [despecialized]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#despecialization
func (r *Result) Align() uintptr {
	return r.Despecialize().Align()
}

// List represents a WIT [list type], which is an ordered vector of an arbitrary type.
// It implements the [Node], [Sized], and [TypeDefKind] interfaces.
//
// [list type]: https://component-model.bytecodealliance.org/design/wit.html#lists
type List struct {
	Type Type
	_typeDefKind
}

// Size returns the [ABI byte size] for a [List].
//
// [ABI byte size]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#size
func (*List) Size() uintptr { return 8 } // [2]int32

// Align returns the [ABI byte alignment] a [List].
//
// [ABI byte alignment]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#alignment
func (*List) Align() uintptr { return 8 } // [2]int32

// Future represents a WIT [future type], expected to be part of [WASI Preview 3].
// It implements the [Node], [Sized], and [TypeDefKind] interfaces.
//
// [future type]: https://github.com/bytecodealliance/wit-bindgen/issues/270
// [WASI Preview 3]: https://bytecodealliance.org/articles/webassembly-the-updated-roadmap-for-developers
type Future struct {
	Type Type // optional associated Type (can be nil)
	_typeDefKind
}

// Size returns the [ABI byte size] for a [Future].
// TODO: what is the ABI size of a future?
//
// [ABI byte size]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#size
func (*Future) Size() uintptr { return 0 }

// Align returns the [ABI byte alignment] a [Future].
// TODO: what is the ABI alignment of a future?
//
// [ABI byte alignment]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#alignment
func (*Future) Align() uintptr { return 0 }

// Stream represents a WIT [stream type], expected to be part of [WASI Preview 3].
// It implements the [Node], [Sized], and [TypeDefKind] interfaces.
//
// [stream type]: https://github.com/WebAssembly/WASI/blob/main/docs/WitInWasi.md#streams
// [WASI Preview 3]: https://bytecodealliance.org/articles/webassembly-the-updated-roadmap-for-developers
type Stream struct {
	Element Type // optional associated Type (can be nil)
	End     Type // optional associated Type (can be nil)
	_typeDefKind
}

// Size returns the [ABI byte size] for a [Stream].
// TODO: what is the ABI size of a stream?
//
// [ABI byte size]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#size
func (*Stream) Size() uintptr { return 0 }

// Align returns the [ABI byte alignment] a [Stream].
// TODO: what is the ABI alignment of a stream?
//
// [ABI byte alignment]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md#alignment
func (*Stream) Align() uintptr { return 0 }

// TypeOwner is the interface implemented by any type that can own a TypeDef,
// currently [World] and [Interface].
type TypeOwner interface {
	Node
	AllFunctions(yield func(*Function) bool) bool
	isTypeOwner()
}

type _typeOwner struct{ _node }

func (_typeOwner) AllFunctions(yield func(*Function) bool) bool { return false }
func (_typeOwner) isTypeOwner()                                 {}

// Type is the interface implemented by any type definition. This can be a
// [primitive type] or a user-defined type in a [TypeDef].
// It also conforms to the [Node], [Sized], and [TypeDefKind] interfaces.
//
// [primitive type]: https://component-model.bytecodealliance.org/design/wit.html#primitive-types
type Type interface {
	Node
	Sized
	TypeDefKind
	isType()
}

// _type is an embeddable struct that conforms to the [Type] interface.
// It also implements the [Node], [Sized], and [TypeDefKind] interfaces.
type _type struct{ _typeDefKind }

func (_type) isType() {}

// ParseType parses a WIT [primitive type] string into
// the associated Type implementation from this package.
// It returns an error if the type string is not recoginized.
//
// [primitive type]: https://component-model.bytecodealliance.org/design/wit.html#primitive-types
func ParseType(s string) (Type, error) {
	switch s {
	case "bool":
		return Bool{}, nil
	case "s8":
		return S8{}, nil
	case "u8":
		return U8{}, nil
	case "s16":
		return S16{}, nil
	case "u16":
		return U16{}, nil
	case "s32":
		return S32{}, nil
	case "u32":
		return U32{}, nil
	case "s64":
		return S64{}, nil
	case "u64":
		return U64{}, nil
	case "float32":
		return Float32{}, nil
	case "float64":
		return Float64{}, nil
	case "char":
		return Char{}, nil
	case "string":
		return String{}, nil
	}
	return nil, fmt.Errorf("unknown primitive type %q", s)
}

// Primitive is a type constriant of the Go equivalents of WIT [primitive types].
//
// [primitive types]: https://component-model.bytecodealliance.org/design/wit.html#primitive-types
type Primitive interface {
	bool | int8 | uint8 | int16 | uint16 | int32 | uint32 | int64 | uint64 | float32 | float64 | char | string
}

// char is defined because [rune] is an alias of [int32]
type char rune

// _primitive represents a WebAssembly Component Model [primitive type]
// mapped to its equivalent Go type.
// It conforms to the [Node], [Sized], [Type], and [TypeDefKind] interfaces.
//
// [primitive type]: https://component-model.bytecodealliance.org/design/wit.html#primitive-types
type _primitive[T Primitive] struct{ _type }

// _primitive is a generic embeddable type that conforms to the [Type] interface.
func (_primitive[T]) isType() {}

// Size returns the byte size for values of this type.
func (_primitive[T]) Size() uintptr {
	var v T
	switch any(v).(type) {
	case string:
		return 8 // [2]int32
	default:
		return unsafe.Sizeof(v)
	}
}

// Align returns the byte alignment for values of this type.
func (_primitive[T]) Align() uintptr {
	var v T
	switch any(v).(type) {
	case string:
		return 4 // int32
	default:
		return unsafe.Alignof(v)
	}
}

// String returns the canonical [primitive type] name in [WIT] text format.
//
// [WIT]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/WIT.md
// [primitive type]: https://component-model.bytecodealliance.org/design/wit.html#primitive-types
func (_primitive[T]) String() string {
	var v T
	switch any(v).(type) {
	case bool:
		return "bool"
	case int8:
		return "s8"
	case uint8:
		return "u8"
	case int16:
		return "s16"
	case uint16:
		return "u16"
	case int32:
		return "s32"
	case uint32:
		return "u32"
	case int64:
		return "s64"
	case uint64:
		return "u64"
	case float32:
		return "float32"
	case float64:
		return "float64"
	case char:
		return "char"
	case string:
		return "string"
	default:
		panic(fmt.Sprintf("BUG: unknown primitive type %T", v)) // should never reach here
	}
}

// Bool represents the WIT [primitive type] bool, a boolean value either true or false.
// It is equivalent to the Go type [bool].
// It implements the [Node], [Sized], [Type], and [TypeDefKind] interfaces.
//
// [primitive type]: https://component-model.bytecodealliance.org/design/wit.html#primitive-types
// [bool]: https://pkg.go.dev/builtin#bool
type Bool struct{ _primitive[bool] }

// S8 represents the WIT [primitive type] s8, a signed 8-bit integer.
// It is equivalent to the Go type [int8].
// It implements the [Node], [Sized], [Type], and [TypeDefKind] interfaces.
//
// [primitive type]: https://component-model.bytecodealliance.org/design/wit.html#primitive-types
// [int8]: https://pkg.go.dev/builtin#int8
type S8 struct{ _primitive[int8] }

// U8 represents the WIT [primitive type] u8, an unsigned 8-bit integer.
// It is equivalent to the Go type [uint8].
// It implements the [Node], [Sized], [Type], and [TypeDefKind] interfaces.
//
// [primitive type]: https://component-model.bytecodealliance.org/design/wit.html#primitive-types
// [uint8]: https://pkg.go.dev/builtin#uint8
type U8 struct{ _primitive[uint8] }

// S16 represents the WIT [primitive type] s16, a signed 16-bit integer.
// It is equivalent to the Go type [int16].
// It implements the [Node], [Sized], [Type], and [TypeDefKind] interfaces.
//
// [primitive type]: https://component-model.bytecodealliance.org/design/wit.html#primitive-types
// [int16]: https://pkg.go.dev/builtin#int16
type S16 struct{ _primitive[int16] }

// U16 represents the WIT [primitive type] u16, an unsigned 16-bit integer.
// It is equivalent to the Go type [uint16].
// It implements the [Node], [Sized], [Type], and [TypeDefKind] interfaces.
//
// [primitive type]: https://component-model.bytecodealliance.org/design/wit.html#primitive-types
// [uint16]: https://pkg.go.dev/builtin#uint16
type U16 struct{ _primitive[uint16] }

// S32 represents the WIT [primitive type] s32, a signed 32-bit integer.
// It is equivalent to the Go type [int32].
// It implements the [Node], [Sized], [Type], and [TypeDefKind] interfaces.
//
// [primitive type]: https://component-model.bytecodealliance.org/design/wit.html#primitive-types
// [int32]: https://pkg.go.dev/builtin#int32
type S32 struct{ _primitive[int32] }

// U32 represents the WIT [primitive type] u32, an unsigned 32-bit integer.
// It is equivalent to the Go type [uint32].
// It implements the [Node], [Sized], [Type], and [TypeDefKind] interfaces.
//
// [primitive type]: https://component-model.bytecodealliance.org/design/wit.html#primitive-types
// [uint32]: https://pkg.go.dev/builtin#uint32
type U32 struct{ _primitive[uint32] }

// S64 represents the WIT [primitive type] s64, a signed 64-bit integer.
// It is equivalent to the Go type [int64].
// It implements the [Node], [Sized], [Type], and [TypeDefKind] interfaces.
//
// [primitive type]: https://component-model.bytecodealliance.org/design/wit.html#primitive-types
// [int64]: https://pkg.go.dev/builtin#int64
type S64 struct{ _primitive[int64] }

// U64 represents the WIT [primitive type] u64, an unsigned 64-bit integer.
// It is equivalent to the Go type [uint64].
// It implements the [Node], [Sized], [Type], and [TypeDefKind] interfaces.
//
// [primitive type]: https://component-model.bytecodealliance.org/design/wit.html#primitive-types
// [uint64]: https://pkg.go.dev/builtin#uint64
type U64 struct{ _primitive[uint64] }

// Float32 represents the WIT [primitive type] float32, a 32-bit floating point value.
// It is equivalent to the Go type [float32].
// It implements the [Node], [Sized], [Type], and [TypeDefKind] interfaces.
//
// [primitive type]: https://component-model.bytecodealliance.org/design/wit.html#primitive-types
// [float32]: https://pkg.go.dev/builtin#float32
type Float32 struct{ _primitive[float32] }

// Float64 represents the WIT [primitive type] float64, a 64-bit floating point value.
// It is equivalent to the Go type [float64].
// It implements the [Node], [Sized], [Type], and [TypeDefKind] interfaces.
//
// [primitive type]: https://component-model.bytecodealliance.org/design/wit.html#primitive-types
// [float64]: https://pkg.go.dev/builtin#float64
type Float64 struct{ _primitive[float64] }

// Char represents the WIT [primitive type] char, a single Unicode character,
// specifically a [Unicode scalar value]. It is equivalent to the Go type [rune].
// It implements the [Node], [Sized], [Type], and [TypeDefKind] interfaces.
//
// [primitive type]: https://component-model.bytecodealliance.org/design/wit.html#primitive-types
// [Unicode scalar value]: https://unicode.org/glossary/#unicode_scalar_value
// [rune]: https://pkg.go.dev/builtin#rune
type Char struct{ _primitive[char] }

// String represents the WIT [primitive type] string, a finite string of Unicode characters.
// It is equivalent to the Go type [string].
// It implements the [Node], [Sized], [Type], and [TypeDefKind] interfaces.
//
// [primitive type]: https://component-model.bytecodealliance.org/design/wit.html#primitive-types
// [string]: https://pkg.go.dev/builtin#string
type String struct{ _primitive[string] }

// Function represents a WIT [function].
// Functions can be freestanding, methods, constructors or static.
// It implements the [Node] and [WorldItem] interfaces.
//
// [function]: https://component-model.bytecodealliance.org/design/wit.html#functions
type Function struct {
	Name    string
	Kind    FunctionKind
	Params  []Param // arguments to the function
	Results []Param // a function can have a single anonymous result, or > 1 named results
	Docs    Docs

	_worldItem
}

// FunctionKind represents the kind of a WIT [function], which can be one of
// [Freestanding], [Method], [Static], or [Constructor].
//
// [function]: https://component-model.bytecodealliance.org/design/wit.html#functions
type FunctionKind interface {
	isFunctionKind()
}

// _functionKind is an embeddable type that conforms to the [FunctionKind] interface.
type _functionKind struct{ _node }

func (_functionKind) isFunctionKind() {}

// Freestanding represents a free-standing function that is not a method, static, or a constructor.
type Freestanding struct {
	_functionKind
}

// Method represents a function that is a method on its associated [Type].
// The first argument to the function is self, an instance of [Type].
type Method struct {
	Type Type
	_functionKind
}

// Static represents a function that is a static method of its associated [Type].
type Static struct {
	Type Type
	_functionKind
}

// Constructor represents a function that is a constructor for its associated [Type].
type Constructor struct {
	Type Type
	_functionKind
}

// Param represents a parameter to or the result of a [Function].
// A Param can be unnamed.
type Param struct {
	Name string
	Type Type
}

// Package represents a [WIT package] within a [Resolve].
// It implements the [Node] interface.
//
// A Package is a collection of [Interface] and [World] values. Additionally,
// a Package contains a unique identifier that affects generated components and uniquely
// identifies this particular package.
//
// [WIT package]: https://component-model.bytecodealliance.org/design/wit.html#packages
type Package struct {
	Name       PackageName
	Interfaces map[string]*Interface
	Worlds     map[string]*World
	Docs       Docs
}

// PackageName represents a [WebAssembly Component Model] package name,
// such as [wasi:clocks@1.0.0]. It contains a namespace, name, and
// optional [SemVer] version information.
//
// [WebAssembly Component Model]: https://component-model.bytecodealliance.org/introduction.html
// [wasi:clocks@1.0.0]: https://github.com/WebAssembly/wasi-clocks
// [SemVer]: https://semver.org/
type PackageName struct {
	// Namespace specifies the package namespace, such as "wasi" in "wasi:foo/bar".
	Namespace string

	// Name specifies the kebab-name of the package.
	Name string

	// Version contains optional major/minor version information.
	Version *semver.Version

	_node
}

// ParsePackageName parses a package string into a [PackageName],
// returning any errors encountered. The resulting PackageName
// may not be valid.
func ParsePackageName(s string) (PackageName, error) {
	var pn PackageName
	name, ver, hasVer := strings.Cut(s, "@")
	pn.Namespace, pn.Name, _ = strings.Cut(name, ":")
	if hasVer {
		var err error
		pn.Version, err = semver.NewVersion(ver)
		if err != nil {
			return pn, err
		}
	}
	return pn, pn.Validate()
}

// Validate validates p, returning any errors.
// TODO: finish this.
func (pn *PackageName) Validate() error {
	switch {
	case pn.Namespace == "":
		return errors.New("missing package namespace")
	case pn.Name == "":
		return errors.New("missing package name")
		// TODO: other validations
	}
	return nil
}

// String implements [fmt.Stringer], returning the canonical string representation of a [PackageName].
func (pn *PackageName) String() string {
	if pn.Version == nil {
		return pn.ShortString()
	}
	return pn.Namespace + ":" + pn.Name + "@" + pn.Version.String()
}

// ShortString returns the canonical string representation of a [PackageName] pn without version information.
func (pn *PackageName) ShortString() string {
	return pn.Namespace + ":" + pn.Name
}

// Docs represent WIT documentation text extracted from comments.
type Docs struct {
	Contents string // may be empty
	_node
}
