// Package bindgen generates Go source code from a fully-resolved WIT package.
// It generates one or more Go packages, with functions, types, constants, and variables,
// along with the associated code to lift and lower Go types into Canonical ABI representation.
package bindgen

import (
	"bytes"
	"errors"
	"fmt"
	"go/token"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/ydnar/wasm-tools-go/cm"
	"github.com/ydnar/wasm-tools-go/internal/codec"
	"github.com/ydnar/wasm-tools-go/internal/go/gen"
	"github.com/ydnar/wasm-tools-go/internal/stringio"
	"github.com/ydnar/wasm-tools-go/wit"
)

const (
	GoSuffix     = ".wit.go"
	BuildDefault = "!wasip1"
	cmPackage    = "github.com/ydnar/wasm-tools-go/cm"
	emptyAsm     = `// This file exists for testing this package without WebAssembly,
// allowing empty function bodies with a //go:wasmimport directive.
// See https://pkg.go.dev/cmd/compile for more information.
`

	// Create Go type aliases for WIT type aliases.
	// This has issues with types that are simultaenously imported and exported in WIT.
	experimentCreateTypeAliases = false

	// Predeclare Go types for own<T> and borrow<T>.
	// Currently broken.
	experimentPredeclareHandles = false
)

type typeDecl struct {
	file  *gen.File // The Go file this type belongs to
	scope gen.Scope // Scope for type-local declarations like method names
	name  string    // The unique Go name for this type
}

type funcDecl struct {
	f          function // The exported Go function
	wasm       function // The wasmimport or wasmexport function
	linkerName string   // The wasmimport or wasmexport mangled linker name
}

// function represents a Go function created from a Component Model function
type function struct {
	file     *gen.File // The Go file this function belongs to
	scope    gen.Scope // Scope for function-local declarations
	name     string    // The scoped unique Go name for this function (method names are scoped to receiver type)
	receiver param     // The method receiver, if any
	params   []param   // Function param(s), with unique Go name(s)
	results  []param   // Function result(s), with unique Go name(s)
}

func (f *function) isMethod() bool {
	return f.receiver.typ != nil
}

// param represents a Go function parameter or result.
// name is a unique Go name within the function scope.
type param struct {
	name string
	typ  wit.Type
	dir  wit.Direction
}

func goFunction(file *gen.File, tdir, dir wit.Direction, f *wit.Function, goName string) function {
	scope := gen.NewScope(file)
	out := function{
		file:    file,
		scope:   scope,
		name:    goName,
		params:  goParams(scope, tdir, f.Params),
		results: goParams(scope, tdir, f.Results),
	}
	if len(out.results) == 1 && out.results[0].name == "" {
		out.results[0].name = "result"
	}
	if dir == wit.Imported && f.IsMethod() {
		out.receiver = out.params[0]
		out.params = out.params[1:]
	}
	return out
}

func goParams(scope gen.Scope, dir wit.Direction, params []wit.Param) []param {
	out := make([]param, len(params))
	for i := range params {
		out[i].name = scope.DeclareName(GoName(params[i].Name, false))
		out[i].typ = params[i].Type
		out[i].dir = dir
	}
	return out
}

type generator struct {
	opts options
	res  *wit.Resolve

	// versioned is set to true if there are multiple versions of a WIT package in res,
	// which affects the generated Go package paths.
	versioned bool

	// packages are Go packages indexed on Go package paths.
	packages map[string]*gen.Package

	// witPackages map WIT identifier paths to Go packages.
	witPackages map[string]*gen.Package

	// exportScopes map WIT identifier paths to export scopes.
	exportScopes map[string]gen.Scope

	// types map [wit.TypeDef] to their Go equivalent.
	// It is indexed on [wit.Direction], either [Imported] or [Exported].
	types [2]map[*wit.TypeDef]typeDecl

	// functions map [wit.Function] to their Go equivalent.
	// It is indexed on [wit.Direction], either [Imported] or [Exported].
	functions [2]map[*wit.Function]funcDecl

	// defined represent whether a world, interface, type, or function has been defined.
	// It is indexed on [wit.Direction], either [Imported] or [Exported].
	defined [2]map[any]bool
}

func newGenerator(res *wit.Resolve, opts ...Option) (*generator, error) {
	g := &generator{
		packages:     make(map[string]*gen.Package),
		witPackages:  make(map[string]*gen.Package),
		exportScopes: make(map[string]gen.Scope),
	}
	for i := 0; i < 2; i++ {
		g.types[i] = make(map[*wit.TypeDef]typeDecl)
		g.functions[i] = make(map[*wit.Function]funcDecl)
		g.defined[i] = make(map[any]bool)
	}
	err := g.opts.apply(opts...)
	if err != nil {
		return nil, err
	}
	if g.opts.generatedBy == "" {
		_, file, _, _ := runtime.Caller(0)
		_, g.opts.generatedBy = filepath.Split(filepath.Dir(filepath.Dir(file)))
	}
	if g.opts.cmPackage == "" {
		g.opts.cmPackage = cmPackage
	}
	g.res = res
	return g, nil
}

func (g *generator) generate() ([]*gen.Package, error) {
	g.detectVersionedPackages()
	err := g.defineWorlds()
	if err != nil {
		return nil, err
	}
	var packages []*gen.Package
	for _, path := range codec.SortedKeys(g.packages) {
		packages = append(packages, g.packages[path])
	}
	return packages, nil
}

func (g *generator) detectVersionedPackages() {
	if g.opts.versioned {
		g.versioned = true
		// fmt.Fprintf(os.Stderr, "Generated versions for all package(s)\n")
		return
	}
	packages := make(map[string]string)
	for _, pkg := range g.res.Packages {
		id := pkg.Name
		id.Version = nil
		path := id.String()
		if packages[path] != "" && packages[path] != pkg.Name.String() {
			g.versioned = true
		} else {
			packages[path] = pkg.Name.String()
		}
	}
	// if g.versioned {
	// 	fmt.Fprintf(os.Stderr, "Multiple versions of package(s) detected\n")
	// }
}

// define marks a world, interface, type, or function as defined.
// It returns true if was newly defined.
func (g *generator) define(dir wit.Direction, v any) (defined bool) {
	if g.defined[dir][v] {
		return false
	}
	g.defined[dir][v] = true
	return true
}

// By default, each WIT interface and world maps to a single Go package.
// Options might override the Go package, including combining multiple
// WIT interfaces and/or worlds into a single Go package.
func (g *generator) defineWorlds() error {
	// fmt.Fprintf(os.Stderr, "Generating Go for %d world(s)\n", len(g.res.Worlds))
	for i, w := range g.res.Worlds {
		if matchWorld(w, g.opts.world) || (g.opts.world == "" && i == len(g.res.Worlds)-1) {
			g.defineWorld(w)
		}
	}
	return nil
}

func matchWorld(w *wit.World, name string) bool {
	if name == w.Name {
		return true
	}
	id := w.Package.Name
	id.Extension = w.Name
	if name == id.String() {
		return true
	}
	id.Version = nil
	return name == id.String()
}

func (g *generator) defineWorld(w *wit.World) error {
	if !g.define(wit.Exported, w) {
		return nil
	}
	id := w.Package.Name
	id.Extension = w.Name
	pkg := g.packageFor(id)
	file := g.fileFor(id)

	{
		var b strings.Builder
		stringio.Write(&b, "Package ", pkg.Name, " represents the ", w.WITKind(), " \"", id.String(), "\".\n")
		if w.Docs.Contents != "" {
			b.WriteString("\n")
			b.WriteString(w.Docs.Contents)
		}
		file.PackageDocs = b.String()
	}

	var err error
	w.Imports.All()(func(name string, v wit.WorldItem) bool {
		switch v := v.(type) {
		case *wit.InterfaceStability:
			// TODO: handle Stability
			err = g.defineInterface(wit.Imported, v.Interface, name)
		case *wit.TypeDef:
			err = g.defineTypeDef(wit.Imported, v, name)
		case *wit.Function:
			if v.IsFreestanding() {
				err = g.defineFunction(id, wit.Imported, v)
			}
		}
		return err == nil
	})
	if err != nil {
		return err
	}

	w.Exports.All()(func(name string, v wit.WorldItem) bool {
		switch v := v.(type) {
		case *wit.InterfaceStability:
			// TODO: handle Stability
			err = g.defineInterface(wit.Exported, v.Interface, name)
		case *wit.TypeDef:
			// WIT does not currently allow worlds to export types.
			err = errors.New("exported type in world " + w.Name)
		case *wit.Function:
			if v.IsFreestanding() {
				err = g.defineFunction(id, wit.Exported, v)
			}
		}
		return err == nil
	})

	return err
}

func (g *generator) defineInterface(dir wit.Direction, i *wit.Interface, name string) error {
	if !g.define(dir, i) {
		return nil
	}
	if i.Name != nil {
		name = *i.Name
	}
	id := i.Package.Name
	id.Extension = name
	pkg := g.packageFor(id)
	file := g.fileFor(id)

	{
		var b strings.Builder
		stringio.Write(&b, "Package ", pkg.Name, " represents the ", dir.String(), " ", i.WITKind(), " \"", id.String(), "\".\n")
		if i.Docs.Contents != "" {
			b.WriteString("\n")
			b.WriteString(i.Docs.Contents)
		}
		file.PackageDocs = b.String()
	}

	// Declare types
	i.TypeDefs.All()(func(name string, td *wit.TypeDef) bool {
		g.declareTypeDef(nil, dir, td, "")
		return true
	})

	// Define types
	i.TypeDefs.All()(func(name string, td *wit.TypeDef) bool {
		g.defineTypeDef(dir, td, name)
		return true
	})

	// TODO: delete this
	// Declare all functions
	// i.Functions.All()(func(_ string, f *wit.Function) bool {
	// 	g.declareFunction(id, dir, f)
	// 	return true
	// })

	// Define standalone functions
	i.Functions.All()(func(_ string, f *wit.Function) bool {
		if f.IsFreestanding() {
			g.defineFunction(id, dir, f)
		}
		return true
	})

	return nil
}

func (g *generator) defineTypeDef(dir wit.Direction, t *wit.TypeDef, name string) error {
	if !experimentCreateTypeAliases && t.Root() != t {
		return nil
	}

	if !g.define(dir, t) {
		return nil
	}
	if t.Name != nil {
		name = *t.Name
	}

	decl, err := g.declareTypeDef(nil, dir, t, "")
	if err != nil {
		return err
	}
	owner := typeDefOwner(t)

	// If an alias, get root
	root := t.Root()
	rootOwner := typeDefOwner(root)
	rootName := name
	if root.Name != nil {
		rootName = *root.Name
	}

	// Define the type
	var b bytes.Buffer
	stringio.Write(&b, "// ", decl.name, " represents the ", dir.String(), " ", root.WITKind(), " \"", rootOwner.String(), "#", rootName, "\".\n")
	b.WriteString("//\n")
	if root != t {
		// Type alias
		stringio.Write(&b, "// See [", g.typeRep(decl.file, dir, root), "] for more information.\n")
		stringio.Write(&b, "type ", decl.name, " = ", g.typeRep(decl.file, dir, root), "\n\n")
	} else {
		b.WriteString(formatDocComments(t.Docs.Contents, false))
		b.WriteString("//\n")
		b.WriteString(formatDocComments(t.WIT(nil, ""), true))
		stringio.Write(&b, "type ", decl.name, " ", g.typeDefRep(decl.file, dir, t, decl.name), "\n\n")
	}

	_, err = decl.file.Write(b.Bytes())
	if err != nil {
		return err
	}

	// Return now unless the type is a resource.
	if _, ok := t.Kind.(*wit.Resource); !ok {
		return nil
	}

	// Emit type namespace in exports file.
	if dir == wit.Exported {
		xfile := g.exportsFileFor(owner)
		scope := g.exportScopes[owner.String()]
		goName := scope.GetName(GoName(*t.Name, true))
		stringio.Write(xfile, "\n// ", goName, " represents the caller-defined exports for ", root.WITKind(), " \"", rootOwner.String(), "#", rootName, "\".\n")
		stringio.Write(xfile, goName, " struct {")
	}

	// Define any associated functions
	switch dir {
	case wit.Imported:
		if f := t.ResourceDrop(); f != nil {
			err := g.defineFunction(owner, wit.Imported, f)
			if err != nil {
				return nil
			}
		}

	case wit.Exported:
		if f := t.ResourceNew(); f != nil {
			err := g.defineFunction(owner, importedWithExportedTypes, f)
			if err != nil {
				return nil
			}
		}

		if f := t.ResourceRep(); f != nil {
			err := g.defineFunction(owner, importedWithExportedTypes, f)
			if err != nil {
				return nil
			}
		}

		if f := t.ResourceDrop(); f != nil {
			err := g.defineFunction(owner, importedWithExportedTypes, f)
			if err != nil {
				return nil
			}
		}

		if f := t.Destructor(); f != nil {
			err := g.defineFunction(owner, dir, f)
			if err != nil {
				return nil
			}
		}

	default:
		return errors.New("BUG: unknown direction " + dir.String())
	}

	if f := t.Constructor(); f != nil {
		err := g.defineFunction(owner, dir, f)
		if err != nil {
			return nil
		}
	}

	for _, f := range t.StaticFunctions() {
		err := g.defineFunction(owner, dir, f)
		if err != nil {
			return nil
		}
	}

	for _, f := range t.Methods() {
		err := g.defineFunction(owner, dir, f)
		if err != nil {
			return nil
		}
	}

	// End struct definition here.
	if dir == wit.Exported {
		xfile := g.exportsFileFor(owner)
		stringio.Write(xfile, "\n}\n")
	}

	return nil
}

func (g *generator) declareTypeDef(file *gen.File, dir wit.Direction, t *wit.TypeDef, goName string) (typeDecl, error) {
	decl, ok := g.types[dir][t]
	if ok {
		return decl, nil
	}
	if goName == "" {
		if t.Name == nil {
			return typeDecl{}, errors.New("BUG: cannot declare unnamed wit.TypeDef")
		}
		goName = GoName(*t.Name, true)
	}
	owner := typeDefOwner(t)
	if file == nil {
		file = g.fileFor(owner)
	}
	decl = typeDecl{
		file:  file,
		name:  declareDirectedName(file, dir, goName),
		scope: gen.NewScope(nil),
	}
	g.types[dir][t] = decl

	if dir == wit.Exported && g.exportScopes[owner.String()] != nil {
		println(owner.String())
		g.exportScopes[owner.String()].DeclareName(goName)
	}

	// If an imported and exported version of a TypeDef are identical, declare the other.
	otherDir := ^dir & 1
	if _, ok := g.types[otherDir][t]; !ok && !wit.HasResource(t) {
		g.types[otherDir][t] = decl
		g.define(otherDir, t) // Mark this type as defined
	}
	// fmt.Fprintf(os.Stderr, "Type:\t%s.%s\n\t%s.%s\n", owner.String(), name, decl.Package.Path, decl.Name)

	// Predeclare own<T> and borrow<T> for resource types.
	if experimentPredeclareHandles {
		switch t.Kind.(type) {
		case *wit.Resource:
			var count int
			for _, t2 := range g.res.TypeDefs {
				var err error
				switch kind := t2.Kind.(type) {
				case *wit.Own:
					if kind.Type == t {
						_, err = g.declareTypeDef(file, dir, t2, "Own"+decl.name)
						count++
					}
				case *wit.Borrow:
					if kind.Type == t {
						_, err = g.declareTypeDef(file, dir, t2, "Borrow"+decl.name)
						count++
					}
				}
				if err != nil {
					return typeDecl{}, err
				}
				if count >= 2 {
					break
				}
			}
		}
	}

	return decl, nil
}

func declareDirectedName(scope gen.Scope, dir wit.Direction, name string) string {
	if dir == wit.Exported && scope.HasName(name) {
		if token.IsExported(name) {
			// Go exported, not WIT exported!
			return scope.DeclareName("Export" + name)
		}
		return scope.DeclareName("export" + name)
	}
	return scope.DeclareName(name)
}

func (g *generator) typeDecl(dir wit.Direction, t *wit.TypeDef) (typeDecl, bool) {
	if decl, ok := g.types[dir][t]; ok {
		return decl, true
	}
	// This may return an exported type used by an imported function, which is disallowed,
	// except for Component Model administrative functions like resource.drop.
	// TODO: figure out a way to enforce that constraint here.
	decl, ok := g.types[^dir&1][t]
	return decl, ok
}

func typeDefOwner(t *wit.TypeDef) wit.Ident {
	var id wit.Ident
	switch owner := t.Owner.(type) {
	case *wit.World:
		id = owner.Package.Name
		id.Extension = owner.Name
	case *wit.Interface:
		id = owner.Package.Name
		if owner.Name == nil {
			id.Extension = "unknown"
		} else {
			id.Extension = *owner.Name
		}
	}
	return id
}

func (g *generator) typeDefRep(file *gen.File, dir wit.Direction, t *wit.TypeDef, goName string) string {
	return g.typeDefKindRep(file, dir, t.Kind, goName)
}

func (g *generator) typeDefKindRep(file *gen.File, dir wit.Direction, kind wit.TypeDefKind, goName string) string {
	switch kind := kind.(type) {
	case *wit.Pointer:
		return g.pointerRep(file, dir, kind)
	case wit.Type:
		return g.typeRep(file, dir, kind)
	case *wit.Record:
		return g.recordRep(file, dir, kind, goName)
	case *wit.Tuple:
		return g.tupleRep(file, dir, kind)
	case *wit.Flags:
		return g.flagsRep(file, dir, kind, goName)
	case *wit.Enum:
		return g.enumRep(file, dir, kind, goName)
	case *wit.Variant:
		return g.variantRep(file, dir, kind, goName)
	case *wit.Result:
		return g.resultRep(file, dir, kind)
	case *wit.Option:
		return g.optionRep(file, dir, kind)
	case *wit.List:
		return g.listRep(file, dir, kind)
	case *wit.Resource:
		return g.resourceRep(file, dir, kind)
	case *wit.Own:
		return g.ownRep(file, dir, kind)
	case *wit.Borrow:
		return g.borrowRep(file, dir, kind)
	case *wit.Future:
		return "any /* TODO: *wit.Future */"
	case *wit.Stream:
		return "any /* TODO: *wit.Stream */"
	default:
		panic(fmt.Sprintf("BUG: unknown wit.TypeDefKind %T", kind)) // should never reach here
	}
}

func (g *generator) pointerRep(file *gen.File, dir wit.Direction, p *wit.Pointer) string {
	return "*" + g.typeRep(file, dir, p.Type)
}

func (g *generator) typeRep(file *gen.File, dir wit.Direction, t wit.Type) string {
	switch t := t.(type) {
	// Special-case nil for the _ in result<T, _>
	case nil:
		return "struct{}"

	case *wit.TypeDef:
		if !experimentCreateTypeAliases {
			t = t.Root()
		}
		if decl, ok := g.typeDecl(dir, t); ok {
			return file.RelativeName(decl.file.Package, decl.name)
		}
		// FIXME: this is only valid for built-in WIT types.
		// User-defined types must be named, so the Ident check above must have succeeded.
		// See https://component-model.bytecodealliance.org/design/wit.html#built-in-types
		// and https://component-model.bytecodealliance.org/design/wit.html#user-defined-types.
		// TODO: add wit.Type.BuiltIn() method?
		return g.typeDefRep(file, dir, t, "")
	case wit.Primitive:
		return g.primitiveRep(t)
	default:
		panic(fmt.Sprintf("BUG: unknown wit.Type %T", t)) // should never reach here
	}
}

func (g *generator) primitiveRep(p wit.Primitive) string {
	switch p := p.(type) {
	case wit.Bool:
		return "bool"
	case wit.S8:
		return "int8"
	case wit.U8:
		return "uint8"
	case wit.S16:
		return "int16"
	case wit.U16:
		return "uint16"
	case wit.S32:
		return "int32"
	case wit.U32:
		return "uint32"
	case wit.S64:
		return "int64"
	case wit.U64:
		return "uint64"
	case wit.F32:
		return "float32"
	case wit.F64:
		return "float64"
	case wit.Char:
		return "rune"
	case wit.String:
		return "string"
	default:
		panic(fmt.Sprintf("BUG: unknown wit.Primitive %T", p)) // should never reach here
	}
}

func (g *generator) recordRep(file *gen.File, dir wit.Direction, r *wit.Record, goName string) string {
	exported := token.IsExported(goName)
	var b strings.Builder
	b.WriteString("struct {")
	for i, f := range r.Fields {
		if i == 0 || i > 0 && f.Docs.Contents != "" {
			b.WriteRune('\n')
		}
		b.WriteString(formatDocComments(f.Docs.Contents, false))
		stringio.Write(&b, fieldName(f.Name, exported), " ", g.typeRep(file, dir, f.Type), "\n")
	}
	b.WriteRune('}')
	return b.String()
}

// Field names are implicitly scoped to their parent struct,
// so we don't need to track the mapping between WIT names and Go names.
func fieldName(name string, export bool) string {
	if name == "" {
		return ""
	}
	if name[0] >= '0' && name[0] <= '9' {
		name = "f" + name
	}
	return GoName(name, export)
}

func (g *generator) tupleRep(file *gen.File, dir wit.Direction, t *wit.Tuple) string {
	var b strings.Builder
	if typ := t.Type(); typ != nil {
		stringio.Write(&b, "[", strconv.Itoa(len(t.Types)), "]", g.typeRep(file, dir, typ))
	} else if len(t.Types) == 0 || len(t.Types) > cm.MaxTuple {
		// Force struct representation
		return g.typeDefKindRep(file, dir, t.Despecialize(), "")
	} else {
		stringio.Write(&b, file.Import(g.opts.cmPackage), ".Tuple")
		if len(t.Types) > 2 {
			b.WriteString(strconv.Itoa(len(t.Types)))
		}
		b.WriteRune('[')
		for i, typ := range t.Types {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(g.typeRep(file, dir, typ))
		}
		b.WriteRune(']')
	}
	return b.String()
}

func (g *generator) flagsRep(file *gen.File, dir wit.Direction, flags *wit.Flags, goName string) string {
	var b strings.Builder

	// FIXME: this isn't ideal
	var typ wit.Type
	size := flags.Size()
	switch size {
	case 1:
		typ = wit.U8{}
	case 2:
		typ = wit.U16{}
	case 4:
		typ = wit.U32{}
	case 8:
		typ = wit.U64{}
	default:
		panic(fmt.Sprintf("FIXME: cannot emit a flags type with %d cases", len(flags.Flags)))
	}

	b.WriteString(g.typeRep(file, dir, typ))
	b.WriteString("\n\n")
	b.WriteString("const (\n")
	for i, flag := range flags.Flags {
		if i > 0 && flag.Docs.Contents != "" {
			b.WriteRune('\n')
		}
		b.WriteString(formatDocComments(flag.Docs.Contents, false))
		flagName := file.DeclareName(goName + GoName(flag.Name, true))
		b.WriteString(flagName)
		if i == 0 {
			stringio.Write(&b, " ", goName, " = 1 << iota")
		}
		b.WriteRune('\n')
	}
	b.WriteString(")\n")
	return b.String()
}

func (g *generator) enumRep(file *gen.File, dir wit.Direction, e *wit.Enum, goName string) string {
	var b strings.Builder
	disc := wit.Discriminant(len(e.Cases))
	b.WriteString(g.typeRep(file, dir, disc))
	b.WriteString("\n\n")
	b.WriteString("const (\n")
	for i, c := range e.Cases {
		if i > 0 && c.Docs.Contents != "" {
			b.WriteRune('\n')
		}
		b.WriteString(formatDocComments(c.Docs.Contents, false))
		b.WriteString(file.DeclareName(goName + GoName(c.Name, true)))
		if i == 0 {
			b.WriteRune(' ')
			b.WriteString(goName)
			b.WriteString(" = iota")
		}
		b.WriteRune('\n')
	}
	b.WriteString(")\n")
	return b.String()
}

func (g *generator) variantRep(file *gen.File, dir wit.Direction, v *wit.Variant, goName string) string {
	// If the variant has no associated types, represent the variant as an enum.
	if e := v.Enum(); e != nil {
		return g.enumRep(file, dir, e, goName)
	}

	disc := wit.Discriminant(len(v.Cases))
	shape := variantShape(v)
	align := variantAlign(v)

	// Emit type
	var b strings.Builder
	cm := file.Import(g.opts.cmPackage)
	stringio.Write(&b, cm, ".Variant[", g.typeRep(file, dir, disc), ", ", g.typeRep(file, dir, shape), ", ", g.typeRep(file, dir, align), "]\n\n")

	// Emit cases
	for i, c := range v.Cases {
		caseNum := strconv.Itoa(i)
		caseName := GoName(c.Name, true)
		constructorName := file.DeclareName(goName + caseName)
		typeRep := g.typeRep(file, dir, c.Type)

		// Emit constructor
		stringio.Write(&b, "// ", constructorName, " returns a [", goName, "] of case \"", c.Name, "\".\n")
		b.WriteString("//\n")
		b.WriteString(formatDocComments(c.Docs.Contents, false))
		stringio.Write(&b, "func ", constructorName, "(")
		dataName := "data"
		if c.Type != nil {
			stringio.Write(&b, dataName, " ", typeRep)
		}
		stringio.Write(&b, ") ", goName, " {")
		if c.Type == nil {
			stringio.Write(&b, "var ", dataName, " ", typeRep, "\n")
		}
		stringio.Write(&b, "return ", cm, ".New[", goName, "](", caseNum, ", ", dataName, ")\n")
		b.WriteString("}\n\n")

		// Emit getter
		if c.Type == nil {
			// Case without an associated type returns bool
			stringio.Write(&b, "// ", caseName, " returns true if [", goName, "] represents the variant case \"", c.Name, "\".\n")
			stringio.Write(&b, "func (self *", goName, ") ", caseName, "() bool {\n")
			stringio.Write(&b, "return ", cm, ".Tag(self) == ", caseNum)
			b.WriteString("}\n\n")
		} else {
			// Case with associated type T returns *T
			stringio.Write(&b, "// ", caseName, " returns a non-nil *[", typeRep, "] if [", goName, "] represents the variant case \"", c.Name, "\".\n")
			stringio.Write(&b, "func (self *", goName, ") ", caseName, "() *", typeRep, " {\n")
			stringio.Write(&b, "return ", cm, ".Case[", typeRep, "](self, ", caseNum, ")")
			b.WriteString("}\n\n")
		}
	}
	return b.String()
}

func (g *generator) resultRep(file *gen.File, dir wit.Direction, r *wit.Result) string {
	var b strings.Builder
	b.WriteString(file.Import(g.opts.cmPackage))
	if r.OK == nil && r.Err == nil {
		b.WriteString(".Result")
	} else if r.OK == nil || (r.Err != nil && r.Err.Size() > r.OK.Size()) {
		stringio.Write(&b, ".ErrResult[", g.typeRep(file, dir, r.OK), ", ", g.typeRep(file, dir, r.Err), "]")
	} else {
		stringio.Write(&b, ".OKResult[", g.typeRep(file, dir, r.OK), ", ", g.typeRep(file, dir, r.Err), "]")
	}
	return b.String()
}

func (g *generator) optionRep(file *gen.File, dir wit.Direction, o *wit.Option) string {
	var b strings.Builder
	stringio.Write(&b, file.Import(g.opts.cmPackage), ".Option[", g.typeRep(file, dir, o.Type), "]")
	return b.String()
}

func (g *generator) listRep(file *gen.File, dir wit.Direction, l *wit.List) string {
	var b strings.Builder
	stringio.Write(&b, file.Import(g.opts.cmPackage), ".List[", g.typeRep(file, dir, l.Type), "]")
	return b.String()
}

func (g *generator) resourceRep(file *gen.File, dir wit.Direction, r *wit.Resource) string {
	return file.Import(g.opts.cmPackage) + ".Resource"
}

func (g *generator) ownRep(file *gen.File, dir wit.Direction, o *wit.Own) string {
	return g.typeRep(file, dir, o.Type)
}

func (g *generator) borrowRep(file *gen.File, dir wit.Direction, b *wit.Borrow) string {
	switch dir {
	case wit.Imported:
		return g.typeRep(file, dir, b.Type)
	case wit.Exported:
		// Exported borrow<T> are represented by a concrete i32 rep.
		return file.Import(g.opts.cmPackage) + ".Rep"
	default:
		panic("BUG: unknown direction " + dir.String())
	}
}

func (g *generator) declareFunction(owner wit.Ident, dir wit.Direction, f *wit.Function) (funcDecl, error) {
	// Setup
	file := g.fileFor(owner)

	var scope gen.Scope = file
	wasm := f.CoreFunction(dir)
	tdir := dir
	var pfx, linkerName string
	switch dir {
	case wit.Imported:
		pfx = "wasmimport_"
		linkerName = owner.String() + " " + f.Name

	case wit.Exported:
		scope = g.exportScopes[owner.String()]
		pfx = "wasmexport_"
		linkerName = owner.String() + "#" + f.Name

	case importedWithExportedTypes:
		dir = wit.Imported  // Imported function...
		tdir = wit.Exported // ...with exported types
		pfx = "wasmimport_"
		linkerName = "[export]" + owner.String() + " " + f.Name

	default:
		return funcDecl{}, errors.New("BUG: unknown direction " + dir.String())
	}

	if fdecl, ok := g.functions[dir][f]; ok {
		return fdecl, nil
	}

	var funcName, wasmName string
	switch f.Kind.(type) {
	case *wit.Freestanding:
		baseName := GoName(f.BaseName(), true)
		funcName = declareDirectedName(scope, dir, baseName)
		wasmName = file.DeclareName(pfx + baseName)

	case *wit.Constructor:
		t := f.Type().(*wit.TypeDef)
		td, _ := g.typeDecl(tdir, t)
		baseName := "New" + td.name
		if dir == wit.Exported {
			baseName = GoName(f.BaseName(), true)
		}
		funcName = declareDirectedName(scope, dir, baseName)
		wasmName = file.DeclareName(pfx + baseName)

	case *wit.Static:
		t := f.Type().(*wit.TypeDef)
		td, _ := g.typeDecl(tdir, t)
		baseName := td.name + GoName(f.BaseName(), true)
		if dir == wit.Exported {
			baseName = GoName(f.BaseName(), true)
		}
		funcName = declareDirectedName(scope, dir, baseName)
		wasmName = file.DeclareName(pfx + baseName)

	case *wit.Method:
		t := f.Type().(*wit.TypeDef)
		if t.Package().Name.Package != owner.Package {
			return funcDecl{}, fmt.Errorf("cannot emit functions in package %s to type %s", owner.Package, t.Package().Name.String())
		}
		td, _ := g.typeDecl(tdir, t)
		switch dir {
		case wit.Imported:
			funcName = td.scope.DeclareName(GoName(f.BaseName(), true))
			if wasm.IsMethod() {
				wasmName = td.scope.DeclareName(pfx + funcName)
			} else {
				wasmName = file.DeclareName(pfx + td.name + funcName)
			}
		case wit.Exported:
			// baseName := td.name + GoName(f.BaseName(), true)
			// funcName = g.declareDirectedName(file, dir, baseName)
			// wasmName = file.DeclareName(pfx + baseName)
			funcName = td.scope.DeclareName(GoName(f.BaseName(), true))
			wasmName = file.DeclareName(pfx + GoName(*t.Name, true) + GoName(f.BaseName(), true))
		}
	}

	fdecl := funcDecl{
		f:          goFunction(file, tdir, dir, f, funcName),
		wasm:       goFunction(file, tdir, dir, wasm, wasmName),
		linkerName: linkerName,
	}
	g.functions[dir][f] = fdecl
	return fdecl, nil
}

// FIXME: this is a fun hack
const importedWithExportedTypes = 2

func (g *generator) defineFunction(owner wit.Ident, dir wit.Direction, f *wit.Function) error {
	decl, err := g.declareFunction(owner, dir, f)
	if err != nil {
		return err
	}

	switch dir {
	case wit.Imported, importedWithExportedTypes:
		return g.defineImportedFunction(owner, f, decl)
	case wit.Exported:
		err := g.defineExportedFunction(owner, f, decl)
		if err != nil {
			return err
		}
		if pf := f.PostReturn(); pf != nil {
			return g.defineFunction(owner, dir, pf)
		}
	default:
		return errors.New("BUG: unknown direction " + dir.String())
	}

	return nil
}

func (g *generator) defineImportedFunction(_ wit.Ident, f *wit.Function, decl funcDecl) error {
	dir := wit.Imported
	if !g.define(dir, f) {
		return nil
	}

	file := decl.f.file

	// Bridging between Go and wasm function
	callParams := slices.Clone(decl.wasm.params)
	for i := range callParams {
		j := i - len(decl.f.params)
		if j < 0 {
			callParams[i].name = decl.f.params[i].name
		} else {
			callParams[i].name = decl.f.results[j].name
		}
	}

	var compoundParams param
	if len(decl.f.params) > 0 && derefAnonRecord(decl.wasm.params[0].typ) != nil {
		name := decl.f.scope.DeclareName("params")
		callParams[0].name = name
		t := derefAnonRecord(decl.wasm.params[0].typ)
		g.declareTypeDef(file, dir, t, decl.wasm.name+"Params")
		compoundParams.name = name
		compoundParams.typ = t
		compoundParams.dir = decl.wasm.params[0].dir
	}

	var compoundResults param
	var resultsRecord *wit.Record
	if len(decl.f.results) > 1 && derefAnonRecord(last(decl.wasm.params).typ) != nil {
		name := decl.f.scope.DeclareName("results")
		last(callParams).name = name
		t := derefAnonRecord(last(decl.wasm.params).typ)
		g.declareTypeDef(file, dir, t, decl.wasm.name+"Results")
		compoundResults.name = name
		compoundResults.typ = t
		compoundResults.dir = last(decl.wasm.params).dir
		resultsRecord = t.Kind.(*wit.Record)
	}

	var b bytes.Buffer

	// Emit docs
	b.WriteString(g.functionDocs(dir, f, decl.f.name))

	// Emit Go function
	b.WriteString("//go:nosplit\n")
	b.WriteString("func ")
	if decl.f.isMethod() {
		stringio.Write(&b, "(", decl.f.receiver.name, " ", g.typeRep(file, decl.f.receiver.dir, decl.f.receiver.typ), ") ", decl.f.name)
	} else {
		b.WriteString(decl.f.name)
	}
	b.WriteString(g.functionSignature(file, decl.f))

	// Emit function body
	b.WriteString(" {\n")
	sameResults := slices.Equal(decl.f.results, decl.wasm.results)
	if len(decl.f.results) == 1 && !sameResults {
		for _, r := range decl.f.results {
			stringio.Write(&b, "var ", r.name, " ", g.typeRep(file, r.dir, r.typ), "\n")
		}
	}

	// Emit compound types
	if compoundParams.typ != nil {
		stringio.Write(&b, compoundParams.name, " := ", g.typeRep(file, compoundParams.dir, compoundParams.typ), "{ ")
		if decl.f.receiver.name != "" {
			stringio.Write(&b, decl.f.receiver.name, ", ")
		}
		for i, p := range decl.f.params {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(p.name)
		}
		b.WriteString(" }\n")
	}
	if compoundResults.typ != nil {
		stringio.Write(&b, "var ", compoundResults.name, " ", g.typeRep(file, compoundResults.dir, compoundResults.typ), "\n")
	}

	// Emit call to wasmimport function
	if sameResults && len(decl.wasm.results) > 0 {
		b.WriteString("return ")
	}
	if decl.wasm.isMethod() {
		stringio.Write(&b, decl.wasm.receiver.name, ".")
	}
	stringio.Write(&b, decl.wasm.name, "(")
	for i, p := range callParams {
		if i > 0 {
			b.WriteString(", ")
		}
		if isPointer(p.typ) {
			b.WriteRune('&')
		}
		b.WriteString(callParams[i].name)
	}
	b.WriteString(")\n")
	if !sameResults {
		b.WriteString("return ")
		if resultsRecord != nil {
			for i, f := range resultsRecord.Fields {
				if i > 0 {
					b.WriteString(", ")
				}
				stringio.Write(&b, compoundResults.name, ".", fieldName(f.Name, false))
			}
		} else {
			for i, r := range decl.f.results {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(r.name)
			}
		}
		b.WriteRune('\n')
	}
	b.WriteString("}\n\n")

	// Emit wasmimport function
	stringio.Write(&b, "//go:wasmimport ", decl.linkerName, "\n")
	b.WriteString("//go:noescape\n")
	b.WriteString("func ")
	if decl.wasm.isMethod() {
		stringio.Write(&b, "(", decl.wasm.receiver.name, " ", g.typeRep(file, decl.wasm.receiver.dir, decl.wasm.receiver.typ), ") ", decl.wasm.name)
	} else {
		b.WriteString(decl.wasm.name)
	}
	b.WriteString(g.functionSignature(file, decl.wasm))

	b.WriteString("\n\n")

	// Emit shared types
	if t, ok := compoundParams.typ.(*wit.TypeDef); ok {
		td, _ := g.typeDecl(dir, t)
		stringio.Write(&b, "// ", td.name, " represents the flattened function params for [", decl.wasm.name, "].\n")
		stringio.Write(&b, "// See the Canonical ABI flattening rules for more information.\n")
		stringio.Write(&b, "type ", td.name, " ", g.typeDefRep(file, dir, t, td.name), "\n\n")
	}

	if t, ok := compoundResults.typ.(*wit.TypeDef); ok {
		td, _ := g.typeDecl(dir, t)
		stringio.Write(&b, "// ", td.name, " represents the flattened function results for [", decl.wasm.name, "].\n")
		stringio.Write(&b, "// See the Canonical ABI flattening rules for more information.\n")
		stringio.Write(&b, "type ", td.name, " ", g.typeDefRep(file, dir, t, td.name), "\n\n")
	}

	// Write to file
	file.Write(b.Bytes())

	return g.ensureEmptyAsm(file.Package)
}

func (g *generator) defineExportedFunction(owner wit.Ident, f *wit.Function, decl funcDecl) error {
	dir := wit.Exported
	if !g.define(dir, f) {
		return nil
	}
	file := decl.f.file
	scope := g.exportScopes[owner.String()]

	var compoundParams param
	var paramsRecord *wit.Record
	if len(decl.f.params) > 0 && derefAnonRecord(decl.wasm.params[0].typ) != nil {
		name := decl.f.scope.DeclareName("params")
		t := derefAnonRecord(decl.wasm.params[0].typ)
		g.declareTypeDef(file, dir, t, decl.wasm.name+"Params")
		compoundParams.name = name
		compoundParams.typ = t
		compoundParams.dir = decl.wasm.params[0].dir
		paramsRecord = t.Kind.(*wit.Record)
	}

	var compoundResults param
	var resultsRecord *wit.Record
	if len(decl.f.results) > 1 && derefAnonRecord(decl.wasm.results[0].typ) != nil {
		name := decl.f.scope.DeclareName("results")
		t := derefAnonRecord(decl.wasm.results[0].typ)
		g.declareTypeDef(file, dir, t, decl.wasm.name+"Results")
		compoundResults.name = name
		compoundResults.typ = t
		compoundParams.dir = decl.wasm.results[0].dir
		resultsRecord = t.Kind.(*wit.Record)
	}

	// Emit exports declaration in exports file
	{
		xfile := g.exportsFileFor(owner)
		stringio.Write(xfile, "\n", g.functionDocs(dir, f, decl.f.name))
		stringio.Write(xfile, decl.f.name, " func", g.functionSignature(xfile, decl.f), "\n")
	}

	var b bytes.Buffer

	// Emit wasmexport function
	stringio.Write(&b, "//go:wasmexport ", decl.linkerName, "\n")
	stringio.Write(&b, "//export ", decl.linkerName, "\n") // TODO: remove this once TinyGo supports go:wasmexport.
	stringio.Write(&b, "func ", decl.wasm.name, g.functionSignature(file, decl.wasm))

	// Emit function body
	b.WriteString(" {\n")
	sameResults := slices.Equal(decl.f.results, decl.wasm.results)

	// Emit call to caller-defined Go function
	if len(decl.f.results) > 0 {
		if sameResults {
			b.WriteString("return ")
		} else if resultsRecord != nil {
			stringio.Write(&b, "var ", compoundResults.name, " ", g.typeRep(file, compoundResults.dir, compoundResults.typ), "\n")
			for i, f := range resultsRecord.Fields {
				if i > 0 {
					b.WriteString(", ")
				}
				stringio.Write(&b, compoundResults.name, ".", fieldName(f.Name, false))
			}
			b.WriteString(" = ")
		} else {
			for i, r := range decl.f.results {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(r.name)
			}
			b.WriteString(" := ")
		}
	}

	// Emit caller-defined function name
	fqName := file.GetName("Exports") + "." + decl.f.name
	if t := f.Type(); t != nil {
		fqName = file.GetName("Exports") + "." + scope.GetName(GoName(t.TypeName(), true)) + "." + decl.f.name
	}
	stringio.Write(&b, fqName, "(")

	// Emit call params
	if paramsRecord != nil {
		for i, f := range paramsRecord.Fields {
			if i > 0 {
				b.WriteString(", ")
			}
			stringio.Write(&b, compoundParams.name, ".", fieldName(f.Name, false))
		}
	} else {
		for i, p := range decl.wasm.params {
			if i > 0 {
				b.WriteString(", ")
			}
			if isPointer(p.typ) {
				b.WriteRune('*')
			}
			b.WriteString(p.name)
		}
	}
	b.WriteString(")\n")
	if !sameResults {
		b.WriteString("return ")
		if resultsRecord != nil {
			stringio.Write(&b, "&", compoundResults.name)
		} else {
			for i, r := range decl.wasm.results {
				if i > 0 {
					b.WriteString(", ")
				}
				if isPointer(r.typ) {
					b.WriteRune('&')
				}
				b.WriteString(r.name)
			}
		}
		b.WriteRune('\n')
	}

	b.WriteString("}\n\n")

	// Emit default function body
	if strings.HasPrefix(f.Name, "[dtor]") || strings.HasPrefix(f.Name, "cabi_post_") {
		stringio.Write(&b, "func init() {")
		stringio.Write(&b, fqName, " = func", g.functionSignature(file, decl.f), " {}\n")
		b.WriteString("}\n\n")
	}

	// Emit shared types
	if t, ok := compoundParams.typ.(*wit.TypeDef); ok {
		td, _ := g.typeDecl(dir, t)
		stringio.Write(&b, "// ", td.name, " represents the flattened function params for [", decl.wasm.name, "].\n")
		stringio.Write(&b, "// See the Canonical ABI flattening rules for more information.\n")
		stringio.Write(&b, "type ", td.name, " ", g.typeDefRep(file, dir, t, td.name), "\n\n")
	}

	if t, ok := compoundResults.typ.(*wit.TypeDef); ok {
		td, _ := g.typeDecl(dir, t)
		stringio.Write(&b, "// ", td.name, " represents the flattened function results for [", decl.wasm.name, "].\n")
		stringio.Write(&b, "// See the Canonical ABI flattening rules for more information.\n")
		stringio.Write(&b, "type ", td.name, " ", g.typeDefRep(file, dir, t, td.name), "\n\n")
	}

	// Write to file
	file.Write(b.Bytes())

	return g.ensureEmptyAsm(file.Package)
}

func (g *generator) functionSignature(file *gen.File, f function) string {
	var b strings.Builder

	b.WriteRune('(')

	// Emit params
	for i, p := range f.params {
		if i > 0 {
			b.WriteString(", ")
		}
		stringio.Write(&b, p.name, " ", g.typeRep(file, p.dir, p.typ))
	}
	b.WriteString(") ")

	// Emit results
	if len(f.results) == 1 {
		b.WriteString(g.typeRep(file, f.results[0].dir, f.results[0].typ))
	} else if len(f.results) > 0 {
		b.WriteRune('(')
		for i, r := range f.results {
			if i > 0 {
				b.WriteString(", ")
			}
			stringio.Write(&b, r.name, " ", g.typeRep(file, r.dir, r.typ))
		}
		b.WriteRune(')')
	}

	return b.String()
}

func last[S ~[]E, E any](s S) *E {
	if len(s) == 0 {
		return nil
	}
	return &s[len(s)-1]
}

func isPointer(t wit.Type) bool {
	if td, ok := t.(*wit.TypeDef); ok {
		if _, ok := td.Kind.(*wit.Pointer); ok {
			return true
		}
	}
	return false
}

func derefTypeDef(t wit.Type) *wit.TypeDef {
	if td, ok := t.(*wit.TypeDef); ok {
		if p, ok := td.Kind.(*wit.Pointer); ok {
			if td, ok := p.Type.(*wit.TypeDef); ok {
				return td
			}
		}
	}
	return nil
}

func derefAnonRecord(t wit.Type) *wit.TypeDef {
	if td := derefTypeDef(t); td != nil && td.Name == nil && td.Owner == nil {
		if _, ok := td.Kind.(*wit.Record); ok {
			return td
		}
	}
	return nil
}

func (g *generator) functionDocs(dir wit.Direction, f *wit.Function, goName string) string {
	var b strings.Builder
	kind := f.WITKind()
	dirString := "the " + dir.String()
	if dir == wit.Exported {
		dirString = "the caller-defined, exported"
	}
	if f.IsAdmin() && f.Type() != nil {
		stringio.Write(&b, "// ", goName, " represents ", dirString, " ", f.BaseName(), " for ", f.Type().WITKind(), " \"", f.Type().TypeName(), "\".\n")
	} else if f.IsConstructor() {
		stringio.Write(&b, "// ", goName, " represents ", dirString, " constructor for ", f.Type().WITKind(), " \"", f.Type().TypeName(), "\".\n")
	} else if f.IsFreestanding() {
		stringio.Write(&b, "// ", goName, " represents ", dirString, " ", kind, " \"", f.Name, "\".\n")
	} else {
		stringio.Write(&b, "// ", goName, " represents ", dirString, " ", kind, " \"", f.BaseName(), "\".\n")
	}
	if f.Docs.Contents != "" {
		b.WriteString("//\n")
		b.WriteString(formatDocComments(f.Docs.Contents, false))
	}
	b.WriteString("//\n")
	if !f.IsAdmin() {
		w := strings.TrimSuffix(f.WIT(nil, f.BaseName()), ";")
		b.WriteString(formatDocComments(w, true))
	}
	return b.String()
}

func (g *generator) ensureEmptyAsm(pkg *gen.Package) error {
	f := pkg.File("empty.s")
	if len(f.Content) > 0 {
		return nil
	}
	_, err := f.Write([]byte(emptyAsm))
	return err
}

func (g *generator) fileFor(id wit.Ident) *gen.File {
	pkg := g.packageFor(id)
	file := pkg.File(id.Extension + GoSuffix)
	file.GeneratedBy = g.opts.generatedBy
	file.Build = BuildDefault
	return file
}

func (g *generator) exportsFileFor(id wit.Ident) *gen.File {
	pkg := g.packageFor(id)
	file := pkg.File(id.Extension + ".exports" + GoSuffix)
	file.GeneratedBy = g.opts.generatedBy
	file.Build = BuildDefault
	if len(file.Header) == 0 {
		exports := file.GetName("Exports")
		var b strings.Builder
		stringio.Write(&b, "// ", exports, " represents the caller-defined exports from \"", id.String(), "\".\n")
		stringio.Write(&b, "var ", exports, " struct {")
		file.Header = b.String()
	}
	file.Trailer = "}\n"
	return file
}

func (g *generator) packageFor(id wit.Ident) *gen.Package {
	// Find existing
	pkg := g.witPackages[id.String()]
	if pkg != nil {
		return pkg
	}

	// Create the package path and name
	var segments []string
	if g.opts.packageRoot != "" && g.opts.packageRoot != "std" {
		segments = append(segments, g.opts.packageRoot)
	}
	segments = append(segments, id.Namespace, id.Package)
	if g.versioned && id.Version != nil {
		segments = append(segments, "v"+id.Version.String())
	}
	segments = append(segments, id.Extension)
	path := strings.Join(segments, "/")

	// TODO: write tests for this
	name := GoPackageName(id.Extension)
	// Ensure local name doesn’t conflict with Go keywords or predeclared identifiers
	if gen.UniqueName(name, gen.IsReserved) != name {
		// Try with package prefix, like error -> ioerror
		name = FlatName(id.Package + name)
		if gen.UniqueName(name, gen.IsReserved) != name {
			// Try with namespace prefix, like ioerror -> wasiioerror
			name = gen.UniqueName(FlatName(id.Namespace+name), gen.IsReserved)
		}
	}

	pkg = gen.NewPackage(path + "#" + name)
	g.packages[pkg.Path] = pkg
	g.witPackages[id.String()] = pkg
	g.exportScopes[id.String()] = gen.NewScope(nil)
	pkg.DeclareName("Exports")

	return pkg
}
