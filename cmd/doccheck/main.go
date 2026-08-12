// Command doccheck verifies package and exported API documentation coverage.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type finding struct {
	path string
	line int
	name string
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	findings, err := inspect(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "doccheck:", err)
		os.Exit(2)
	}
	for _, item := range findings {
		fmt.Fprintf(os.Stderr, "%s:%d: missing documentation for %s\n", item.path, item.line, item.name)
	}
	if len(findings) > 0 {
		fmt.Fprintf(os.Stderr, "doccheck: %d documentation finding(s)\n", len(findings))
		os.Exit(1)
	}
	fmt.Println("doccheck: package and exported API documentation is complete")
}

func inspect(root string) ([]finding, error) {
	directories := make(map[string]struct{})
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && shouldSkipDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			directories[filepath.Dir(path)] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	var findings []finding
	for directory := range directories {
		packages, parseErr := parser.ParseDir(fset, directory, productionGoFile, parser.ParseComments)
		if parseErr != nil {
			return nil, parseErr
		}
		for _, pkg := range packages {
			if pkg.Name == "main" {
				continue
			}
			findings = append(findings, inspectPackage(fset, directory, pkg.Name, pkg.Files)...)
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].path == findings[j].path {
			return findings[i].line < findings[j].line
		}
		return findings[i].path < findings[j].path
	})
	return findings, nil
}

func shouldSkipDirectory(name string) bool {
	return strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" || name == "node_modules"
}

func productionGoFile(info fs.FileInfo) bool {
	return !info.IsDir() && strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go")
}

func inspectPackage(fset *token.FileSet, directory, packageName string, files map[string]*ast.File) []finding {
	var findings []finding
	if !hasPackageDocumentation(files) {
		findings = append(findings, finding{path: directory, line: 1, name: "package " + packageName})
	}
	for _, file := range files {
		for _, declaration := range file.Decls {
			findings = append(findings, inspectDeclaration(fset, declaration)...)
		}
	}
	return findings
}

func hasPackageDocumentation(files map[string]*ast.File) bool {
	for _, file := range files {
		if file.Doc != nil && strings.TrimSpace(file.Doc.Text()) != "" {
			return true
		}
	}
	return false
}

func inspectDeclaration(fset *token.FileSet, declaration ast.Decl) []finding {
	switch typed := declaration.(type) {
	case *ast.FuncDecl:
		if !ast.IsExported(typed.Name.Name) || !exportedReceiver(typed) || typed.Doc != nil {
			return nil
		}
		return []finding{newFinding(fset, typed.Pos(), typed.Name.Name)}
	case *ast.GenDecl:
		return inspectGeneralDeclaration(fset, typed)
	default:
		return nil
	}
}

func exportedReceiver(function *ast.FuncDecl) bool {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return true
	}
	name := receiverName(function.Recv.List[0].Type)
	return ast.IsExported(name)
}

func receiverName(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.StarExpr:
		return receiverName(typed.X)
	case *ast.IndexExpr:
		return receiverName(typed.X)
	case *ast.IndexListExpr:
		return receiverName(typed.X)
	default:
		return ""
	}
}

func inspectGeneralDeclaration(fset *token.FileSet, declaration *ast.GenDecl) []finding {
	var findings []finding
	for _, rawSpec := range declaration.Specs {
		switch spec := rawSpec.(type) {
		case *ast.TypeSpec:
			if ast.IsExported(spec.Name.Name) && declaration.Doc == nil && spec.Doc == nil {
				findings = append(findings, newFinding(fset, spec.Pos(), spec.Name.Name))
			}
		case *ast.ValueSpec:
			if declaration.Doc != nil || spec.Doc != nil {
				continue
			}
			for _, name := range spec.Names {
				if ast.IsExported(name.Name) {
					findings = append(findings, newFinding(fset, name.Pos(), name.Name))
				}
			}
		}
	}
	return findings
}

func newFinding(fset *token.FileSet, position token.Pos, name string) finding {
	location := fset.Position(position)
	return finding{path: location.Filename, line: location.Line, name: name}
}
