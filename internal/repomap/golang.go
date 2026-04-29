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
		lines = append(lines, declLines(fset, decl)...)
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

// declLines renders one top-level declaration into zero or more repo-map lines.
func declLines(fset *token.FileSet, decl ast.Decl) []string {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if d.Name.IsExported() {
			return []string{"  " + funcSig(fset, d)}
		}
	case *ast.GenDecl:
		var out []string
		for _, spec := range d.Specs {
			out = append(out, specLines(fset, spec)...)
		}
		return out
	}
	return nil
}

// specLines renders one spec inside a GenDecl (type/const/var).
func specLines(fset *token.FileSet, spec ast.Spec) []string {
	switch s := spec.(type) {
	case *ast.TypeSpec:
		if !s.Name.IsExported() {
			return nil
		}
		return typeSpecLines(fset, s)
	case *ast.ValueSpec:
		return valueSpecLines(s)
	}
	return nil
}

func typeSpecLines(fset *token.FileSet, s *ast.TypeSpec) []string {
	switch t := s.Type.(type) {
	case *ast.InterfaceType:
		out := []string{fmt.Sprintf("  type %s interface", s.Name.Name)}
		for _, m := range t.Methods.List {
			if fn, ok := m.Type.(*ast.FuncType); ok && len(m.Names) > 0 {
				out = append(out, fmt.Sprintf("    %s%s", m.Names[0].Name, funcTypeSig(fset, fn)))
			}
		}
		return out
	case *ast.StructType:
		fields := exportedFieldNames(t)
		if len(fields) > 0 {
			return []string{fmt.Sprintf("  type %s struct { %s }", s.Name.Name, strings.Join(fields, ", "))}
		}
		return []string{fmt.Sprintf("  type %s struct", s.Name.Name)}
	default:
		return []string{fmt.Sprintf("  type %s", typeSpecSig(fset, s))}
	}
}

func valueSpecLines(s *ast.ValueSpec) []string {
	var out []string
	for _, name := range s.Names {
		if name.IsExported() {
			out = append(out, fmt.Sprintf("  const/var %s", name.Name))
		}
	}
	return out
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
