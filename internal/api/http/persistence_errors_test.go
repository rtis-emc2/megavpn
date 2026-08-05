package http

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPHandlersDoNotSilentlyDiscardStoreErrors(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	files := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(files, filepath.Clean(name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			assign, ok := node.(*ast.AssignStmt)
			if !ok || !assignmentDiscardsAllResults(assign.Lhs) {
				return true
			}
			for _, rhs := range assign.Rhs {
				call, ok := rhs.(*ast.CallExpr)
				if !ok || !isServerStoreCall(call.Fun) {
					continue
				}
				position := files.Position(assign.Pos())
				t.Errorf("%s:%d silently discards a store result; handle or explicitly log the error", name, position.Line)
			}
			return true
		})
	}
}

func assignmentDiscardsAllResults(values []ast.Expr) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		ident, ok := value.(*ast.Ident)
		if !ok || ident.Name != "_" {
			return false
		}
	}
	return true
}

func isServerStoreCall(expr ast.Expr) bool {
	method, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	store, ok := method.X.(*ast.SelectorExpr)
	if !ok || store.Sel.Name != "store" {
		return false
	}
	receiver, ok := store.X.(*ast.Ident)
	return ok && receiver.Name == "s"
}
