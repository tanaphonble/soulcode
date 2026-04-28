package repomap

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
)

// goEntry parses a Go source file with the standard AST and returns a compact
// symbol listing: exported functions, methods, types, and constants.
func goEntry(path, rel string) string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return ""
	}

	var lines []string

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name.IsExported() {
				lines = append(lines, "  "+funcSig(fset, d))
			}

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if !s.Name.IsExported() {
						continue
					}
					switch t := s.Type.(type) {
					case *ast.InterfaceType:
						lines = append(lines, fmt.Sprintf("  type %s interface", s.Name.Name))
						for _, m := range t.Methods.List {
							if fn, ok := m.Type.(*ast.FuncType); ok && len(m.Names) > 0 {
								lines = append(lines, fmt.Sprintf("    %s%s", m.Names[0].Name, funcTypeSig(fset, fn)))
							}
						}
					case *ast.StructType:
						fields := exportedFieldNames(t)
						if len(fields) > 0 {
							lines = append(lines, fmt.Sprintf("  type %s struct { %s }", s.Name.Name, strings.Join(fields, ", ")))
						} else {
							lines = append(lines, fmt.Sprintf("  type %s struct", s.Name.Name))
						}
					default:
						lines = append(lines, fmt.Sprintf("  type %s", typeSpecSig(fset, s)))
					}

				case *ast.ValueSpec:
					for _, name := range s.Names {
						if name.IsExported() {
							lines = append(lines, fmt.Sprintf("  const/var %s", name.Name))
						}
					}
				}
			}
		}
	}

	if len(lines) == 0 {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n", rel)
	for _, l := range lines {
		sb.WriteString(l)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// funcSig returns the signature of a function declaration without its body.
func funcSig(fset *token.FileSet, d *ast.FuncDecl) string {
	// Print a copy with nil Body so go/printer omits the braces.
	sig := &ast.FuncDecl{Recv: d.Recv, Name: d.Name, Type: d.Type}
	var buf bytes.Buffer
	_ = printer.Fprint(&buf, fset, sig)
	return strings.TrimSpace(buf.String())
}

// funcTypeSig returns "(params) returnType" for a FuncType — used for interface methods.
func funcTypeSig(fset *token.FileSet, ft *ast.FuncType) string {
	var buf bytes.Buffer
	_ = printer.Fprint(&buf, fset, ft)
	return strings.TrimSpace(buf.String())
}

// typeSpecSig formats a TypeSpec for non-struct/non-interface types (aliases, etc.).
func typeSpecSig(fset *token.FileSet, s *ast.TypeSpec) string {
	var buf bytes.Buffer
	_ = printer.Fprint(&buf, fset, s)
	return strings.TrimSpace(buf.String())
}

// exportedFieldNames returns the names of exported struct fields.
func exportedFieldNames(st *ast.StructType) []string {
	var names []string
	for _, f := range st.Fields.List {
		for _, n := range f.Names {
			if n.IsExported() {
				names = append(names, n.Name)
			}
		}
	}
	return names
}
