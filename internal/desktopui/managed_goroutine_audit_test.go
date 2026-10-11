package desktopui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAppOwnedBackgroundGoroutinesUseManagedLifecycle(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate the desktopui package")
	}
	packageDir := filepath.Dir(sourceFile)
	entries, err := os.ReadDir(packageDir)
	if err != nil {
		t.Fatalf("read desktopui source directory: %v", err)
	}
	fset := token.NewFileSet()
	managedPrimitiveCount := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(packageDir, name)
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || len(function.Recv.List) == 0 {
				continue
			}
			receiver, ok := function.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			receiverName, ok := receiver.X.(*ast.Ident)
			if !ok || receiverName.Name != "App" {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				goStatement, ok := node.(*ast.GoStmt)
				if !ok {
					return true
				}
				if _, launchesFunction := goStatement.Call.Fun.(*ast.FuncLit); !launchesFunction {
					return true
				}
				if name == "managed_tasks.go" {
					managedPrimitiveCount++
					return true
				}
				position := fset.Position(goStatement.Pos())
				t.Errorf("%s:%d starts an App-owned goroutine outside launchManagedBackground", name, position.Line)
				return true
			})
		}
	}
	if managedPrimitiveCount != 1 {
		t.Fatalf("managed lifecycle primitive contains %d goroutine launchers; want exactly one", managedPrimitiveCount)
	}
}
