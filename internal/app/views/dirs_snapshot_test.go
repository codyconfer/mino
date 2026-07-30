package views

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var snapshotAccessors = map[string]bool{"Dirs": true, "RoleDef": true}

func countDirsCalls(body ast.Node) int {
	n := 0
	ast.Inspect(body, func(node ast.Node) bool {
		switch t := node.(type) {
		case *ast.FuncLit:
			return false
		case *ast.CallExpr:
			if sel, ok := t.Fun.(*ast.SelectorExpr); ok && snapshotAccessors[sel.Sel.Name] {
				n++
			}
		}
		return true
	})
	return n
}

func TestOneDirectivesSnapshotPerFunction(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(node ast.Node) bool {
			var body ast.Node
			var label string
			switch t := node.(type) {
			case *ast.FuncDecl:
				if t.Body == nil {
					return true
				}
				body, label = t.Body, t.Name.Name
			case *ast.FuncLit:
				body, label = t.Body, "func literal"
			default:
				return true
			}
			if n := countDirsCalls(body); n > 1 {
				t.Errorf("%s: %s takes %d Dirs() snapshots; a check-then-act pair can see two different directive generations — take one snapshot and read both lookups from it",
					fset.Position(node.Pos()), label, n)
			}
			return true
		})
	}
}
