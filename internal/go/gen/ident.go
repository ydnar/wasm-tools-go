package gen

import (
	"strings"
)

// Ident represents a qualified Go identifier. It is a combination of
// a package path and a short name. For package identifiers, Path
// represents the package path (e.g. "encoding/json") and Name
// represents the short package name (e.g. "json").
type Ident struct {
	Path string
	Name string
}

// ParseIdent parses string s into an [Ident].
// It returns an error if s cannot be successfully parsed
func ParseIdent(s string) (Ident, error) {
	var id Ident
	id.Path, id.Name, _ = strings.Cut(s, "#")
	if id.Name == "" {
		i := strings.LastIndex(id.Path, "/")
		if i >= 0 && i < len(id.Path)-1 {
			id.Name = id.Path[i+1:] // encoding/json -> json
		} else {
			id.Name = id.Path // encoding -> encoding
		}
	}
	return id, nil
}

// String returns the canonical string representation of id. It implements the [fmt.Stringer] interface.
//
// The canonical string representation of an [Ident] is "$path#$name". Examples:
//   - For a package: "encoding/xml#xml"
//   - For a type: "encoding/xml#Attr"
//   - For a constant: "encoding/xml#Header"
//   - For a function: "encoding/xml#Marshal"
func (id Ident) String() string {
	return id.Path + "#" + id.Name
}

// ParseSelector parses string s into a package path and short name.
// It does not validate the input or resulting values. Examples:
// "io" -> "io", "io"
// "encoding/json" -> "encoding/json", "json"
// "encoding/json#Decoder" -> "encoding/json", "Decoder"
// "wasi/clocks/wall#DateTime" -> "wasi/clocks/wall", "DateTime"
func ParseSelector(s string) (path, name string) {
	path, name, _ = strings.Cut(s, "#")
	if name == "" {
		i := strings.LastIndex(path, "/")
		if i >= 0 && i < len(path)-1 {
			name = path[i+1:] // encoding/json -> json
		} else {
			name = path // encoding -> encoding
		}
	}
	return path, name
}
