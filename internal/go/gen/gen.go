package gen

import (
	"bytes"
	"go/format"
	"strings"

	"github.com/ydnar/wasm-tools-go/internal/codec"
)

// Package represents a Go package, containing zero or more files
// of generated code, along with zero or more declarations.
type Package struct {
	// Ident is the package path and name.
	Ident

	// Files is the list of Go source files in this package.
	Files map[string]*File

	// Decls is the top-level declaractions in this package,
	// including constants, variables, and functions.
	Decls map[string]struct{}
}

// File represents a generated file. It may be a Go file
type File struct {
	// Name is the short name of the file.
	// If Name ends in ".go" this file will be treated as a Go file.
	Name string

	// Build contains build tags, serialized as //gso:build ...
	// Ignored if this is not a Go file.
	Build string

	// Package this file belongs to.
	Package *Package

	// Imports maps Go package imports from package path to local name, e.g. {"encoding/json": "json"}.
	Imports Imports

	// Nodes is a slice of generated code or data nodes.
	Nodes []Node
}

// IsGo returns true if f represents a Go file.
func (f *File) IsGo() bool {
	return strings.HasSuffix(f.Name, ".go")
}

// Bytes returns the byte values of this file.
func (f *File) Bytes() []byte {
	var b bytes.Buffer
	if f.Build != "" {
		b.WriteString("//go:build ")
		b.WriteString(f.Build)
		b.WriteString("\n\n")
	}

	b.WriteString("package ")
	b.WriteString(f.Package.Name)
	b.WriteString("\n\n")

	if len(f.Imports) > 0 {
		b.WriteString(f.Imports.String())
		b.WriteString("\n\n")
	}

	formatted, _ := format.Source(b.Bytes())

	return formatted
}

// Imports is a map of Go package imports from package path to local name, e.g. {"encoding/json": "json"}.
type Imports map[string]string

// String returns the Go imports declaration for i.
func (i Imports) String() string {
	if len(i) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("import (\n")
	for _, path := range codec.SortedKeys(i) {
		name := i[path]
		b.WriteRune('\t')
		if path != name && !strings.HasSuffix(path, "/"+name) {
			b.WriteString(i[path])
			b.WriteRune(' ')
		}
		b.WriteString("\"")
		b.WriteString(path)
		b.WriteString("\"\n")
	}
	b.WriteString(")")
	return b.String()
}

// Node is the common interface implemented by blocks of code or data in a [File].
type Node interface {
	Bytes() []byte
}

// Bytes is a [Node] containing only raw bytes.
type Bytes []byte

// Bytes returns the byte values of b.
// This implements the [Node] interface.
func (b Bytes) Bytes() []byte {
	return b
}

// Decl is the common interface implemented by declarations.
type Decl interface {
	Decl() string
	Node
}

type Type struct {
	Name        string
	Constraints string
	Value       string
}

type Const struct {
	Name  string
	Value string
}

func (c *Const) Decl() string { return c.Name }

type Var struct {
	Name  string
	Value string
}

func (v *Var) Decl() string { return v.Name }
