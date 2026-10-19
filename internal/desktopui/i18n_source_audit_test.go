package desktopui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

func TestDesktopUIHasNoLegacyLocalizedStatusCalls(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate source audit test")
	}
	root := filepath.Dir(sourceFile)
	legacy := map[string]struct{}{
		"setStatus":                   {},
		"feedbackSeverity":            {},
		"isSideEffectStatus":          {},
		"defaultConfirmStyleForTitle": {},
		"showConfirmDialog":           {},
	}
	legacyModalMethods := map[string]struct{}{
		"ShowConfirm":   {},
		"ShowCustom":    {},
		"ShowForm":      {},
		"ShowInput":     {},
		"ShowDualInput": {},
	}

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		checkName := func(name string, pos token.Pos) {
			if _, forbidden := legacy[name]; forbidden {
				t.Errorf("%s:%d uses legacy localized UI helper %q", filepath.Base(path), fset.Position(pos).Line, name)
			}
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				checkName(fn.Name.Name, fn.Name.Pos())
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch current := node.(type) {
			case *ast.CallExpr:
				if identifier, ok := current.Fun.(*ast.Ident); ok {
					checkName(identifier.Name, identifier.Pos())
				}
				selector, ok := current.Fun.(*ast.SelectorExpr)
				if !ok {
					break
				}
				if modalSelector, ok := selector.X.(*ast.SelectorExpr); ok {
					if app, ok := modalSelector.X.(*ast.Ident); ok && app.Name == "a" && modalSelector.Sel.Name == "modal" {
						if _, forbidden := legacyModalMethods[selector.Sel.Name]; forbidden {
							t.Errorf("%s:%d calls legacy string modal method %q", filepath.Base(path), fset.Position(selector.Pos()).Line, selector.Sel.Name)
						}
					}
				}
			case *ast.BinaryExpr:
				if current.Op == token.EQL || current.Op == token.NEQ {
					if selectorPath(current.X) == "a.modal.Title" || selectorPath(current.Y) == "a.modal.Title" {
						t.Errorf("%s:%d compares a localized modal title instead of a stable dialog ID", filepath.Base(path), fset.Position(current.Pos()).Line)
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan desktop UI sources: %v", err)
	}
}

func TestDesktopUIHasNoHardCodedHanWidgetLabels(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate source audit test")
	}
	root := filepath.Dir(sourceFile)
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, index := callNameAndTextArgument(call)
			if name == "" || index < 0 || index >= len(call.Args) {
				return true
			}
			literal, ok := call.Args[index].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err != nil {
				return true
			}
			for _, r := range value {
				if unicode.Is(unicode.Han, r) {
					t.Errorf("%s:%d passes hard-coded Chinese copy %q to %s; use a catalog message", filepath.Base(path), fset.Position(literal.Pos()).Line, value, name)
					break
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan desktop UI widget labels: %v", err)
	}
}

func callNameAndTextArgument(call *ast.CallExpr) (string, int) {
	switch function := call.Fun.(type) {
	case *ast.SelectorExpr:
		if packageName, ok := function.X.(*ast.Ident); ok && packageName.Name == "material" {
			switch function.Sel.Name {
			case "Label", "Caption", "Body1", "Body2", "H1", "H2", "H3", "H4", "H5", "H6", "Subhead":
				return "material." + function.Sel.Name, len(call.Args) - 1
			case "Editor", "Button", "ButtonStyle", "CheckBox", "RadioButton":
				return "material." + function.Sel.Name, len(call.Args) - 1
			}
		}
		if packageName, ok := function.X.(*ast.Ident); ok && packageName.Name == "semantic" && function.Sel.Name == "DescriptionOp" {
			return "semantic.DescriptionOp", 0
		}
	case *ast.Ident:
		switch function.Name {
		case "jsonLabel", "homeTextLabel":
			return function.Name, 2
		case "jsonButtonSemantics":
			return function.Name, 1
		case "jsonActionButton", "markdownTextButton":
			return function.Name, 2
		case "layoutAgentSidebarIconButton":
			return function.Name, 4
		case "agentHistoryLabel":
			return function.Name, 1
		}
	}
	return "", 0
}

func selectorPath(expression ast.Expr) string {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		if identifier, ok := expression.(*ast.Ident); ok {
			return identifier.Name
		}
		return ""
	}
	base := selectorPath(selector.X)
	if base == "" {
		return ""
	}
	return base + "." + selector.Sel.Name
}
