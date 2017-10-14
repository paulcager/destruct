package main

import (
	"fmt"
	"go/parser"
	"go/types"
	"os"
	"os/exec"
	"text/template"

	"golang.org/x/tools/go/loader"
)

type structType struct {
	pkg      *types.Package
	name     string
	fullName string
	struc    *types.Struct
}

// Types to pass into the graphvis templates.

type field struct {
	Name     string // E.g. "Thing"
	Type     string // E.g. "MyType"
	FullType string // E.g. "my/package.MyType"
}

type structInfo struct {
	Name     string // E.g. "MyType"
	FullName string // E.g. "my/package.MyType"
	Fields   []field
}

type useInfo struct {
	FromStruct, FromField, ToStruct string
}

var (
	conf = loader.Config{ParserMode: parser.ParseComments}
)

func main() {
	pkgNames := []string{"xx/types", "xx/test2"}
	conf.FromArgs(pkgNames, false)
	prog, err := conf.Load()
	must(err, "%s", err)

	structs := make(map[string]structType)
	for _, name := range pkgNames {
		pkg := prog.Imported[name]
		for k, v := range findStructs(pkg) {
			structs[k] = v
		}
	}

	if len(structs) == 0 {
		abort("No structures to print")
	}

	for k, v := range prog.AllPackages {
		fmt.Println(k.Path(), v.Pkg.Name())
	}
	os.Exit(2)
	out, err := os.Create("test.dot")
	must(err, "")
	defer out.Close()

	var links []useInfo

	hdrTmpl.Execute(out, nil)
	for _, str := range structs {
		s := structInfo{Name: str.name, FullName: str.fullName}
		n := str.struc.NumFields()
		for i := 0; i < n; i++ {
			f := str.struc.Field(i)
			names := findNamedTypes(f.Type())
			for _, name := range names {
				fullName := name.String()
				if _, include := structs[fullName]; include {
					links = append(links, useInfo{FromStruct: str.fullName, FromField: f.Name(), ToStruct: fullName})
				}
			}
			s.Fields = append(s.Fields, field{
				Name:     f.Name(),
				Type:     types.TypeString(f.Type(), types.RelativeTo(str.pkg)),
				FullType: f.Type().String(),
			})
		}
		structTmpl.Execute(out, s)
	}

	for _, l := range links {
		arrowTmpl.Execute(out, l)
	}

	tailTmpl.Execute(out, nil)
	out.Close()

	cmd := exec.Command("dot", "-Tsvg", "-O", "test.dot")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	must(cmd.Run(), "")
}

// findNamedTypes follows the given type, collecting Named Types it may reference.
// For example "[]*time.Time" would return {time.Time}
// "map[time.Time]time.Duration" would return {time.Time, time.Duration}
func findNamedTypes(t types.Type) []*types.Named {
	switch t := t.(type) {
	case *types.Named:
		return []*types.Named{t}
	case *types.Array:
		return findNamedTypes(t.Elem())
	case *types.Chan:
		return findNamedTypes(t.Elem())
	case *types.Map:
		ret := findNamedTypes(t.Key())
		ret = append(ret, findNamedTypes(t.Elem())...)
		return ret
	case *types.Pointer:
		return findNamedTypes(t.Elem())
	case *types.Slice:
		return findNamedTypes(t.Elem())
	}

	return nil
}

// findStructs returns a map of structure types in the package.
func findStructs(pkg *loader.PackageInfo) map[string]structType {
	ret := make(map[string]structType)
	for _, def := range pkg.Defs {
		if typName, ok := def.(*types.TypeName); ok {
			if typName.Parent() != pkg.Pkg.Scope() {
				// Not at package-level scope, e.g. defined locally in a func.
				continue
			}
			if str, ok := typName.Type().(*types.Named).Underlying().(*types.Struct); ok {
				st := structType{
					pkg:      pkg.Pkg,
					name:     typName.Name(),
					fullName: pkg.Pkg.Path() + "." + typName.Name(),
					struc:    str}
				ret[st.fullName] = st
			}
		}
	}

	return ret
}

func must(err error, format string, params ...interface{}) {
	if err != nil {
		abort(format, params...)
	}
}

func abort(format string, params ...interface{}) {
	fmt.Fprintf(os.Stderr, format, params...)
	os.Exit(2)
}

var hdrTmpl = template.Must(template.New("hdr").Parse(`digraph "structDiagram" {
  graph [
    //rankdir="LR"
    label="\nGenerated by typegraph"
    labeljust="l"
    //ranksep="0.5"
    //nodesep="0.2"
    fontsize="12"
    fontname="Helvetica"
    bgcolor="#f8f8f8"
  ];
  node [
    fontname="Helvetica"
    fontsize="12"
    shape="plaintext"
  ];
  edge [
    arrowsize="1.5"
  ];
`))

var structTmpl = template.Must(template.New("struct").Parse(`"{{.FullName}}" [
    label=<
    <TABLE BORDER="0" CELLBORDER="1" CELLSPACING="0" BGCOLOR="#ffffff">
      <TR><TD COLSPAN="3" BGCOLOR="#e0e0ff" ALIGN="CENTER">{{.Name}}</TD></TR>
      {{range .Fields}}      <TR><TD PORT="X{{.Name}}" COLSPAN="1" BGCOLOR="#f0f0ff" ALIGN="LEFT">{{.Name}}</TD> <TD PORT="{{.Name}}" COLSPAN="2" BGCOLOR="#f0f0ff" ALIGN="LEFT">{{.Type}}</TD></TR>
      {{end}}
    </TABLE>>
    tooltip="{{.FullName}}"
  ];
`))

var arrowTmpl = template.Must(template.New("arrow").Parse(`  "{{.FromStruct}}":"{{.FromField}}" -> "{{.ToStruct}}" [];
`))

var tailTmpl = template.Must(template.New("tail").Parse(`
}
`))