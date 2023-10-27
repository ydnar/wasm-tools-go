package wit

import (
	"strings"
)

// Ident represents a WIT identifier. It is a combination of:
//   - A package name (without version information)
//   - An optional interface name
//   - An optional name
type Ident struct {
	Package   string
	Interface string
	Name      string
}

// ParseIdent parses string s into an [Ident].
// It returns an error if s cannot be successfully parsed
func ParseIdent(s string) (Ident, error) {
	var id Ident
	pkg, path, _ := strings.Cut(s, "/")
	pn, err := ParsePackageName(pkg)
	if err != nil {
		return id, err
	}
	id.Package = pn.ShortString()
	id.Interface, id.Name, _ = strings.Cut(path, ".")
	return id, err
}

// String returns the canonical string representation of id. It implements the [fmt.Stringer] interface.
//
// The canonical string representation of an [Ident] is "$package[/$interface[.$name]]". Examples:
//   - For a [Package]: "wasi:clocks"
//   - For an [Interface]: "wasi:clocks/wall-clock"
//   - For a [Type]: "wasi:clocks/wall-clock.datetime"
//   - For a [Function]: "wasi:clocks/wall-clock.now"
func (id Ident) String() string {
	switch {
	case id.Package != "" && id.Interface != "" && id.Name != "":
		return id.Package + "/" + id.Interface + "." + id.Name
	case id.Package != "" && id.Interface != "":
		return id.Package + "/" + id.Interface
	case id.Package != "":
		return id.Package
	}
	return ""
}
