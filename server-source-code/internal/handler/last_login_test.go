package handler

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// #730: last_login was written only by the OIDC and Discord profile updaters,
// so anyone signing in with a username and password left the column NULL and
// the Users table showed "Never" forever. The dashboard's recent-logins widget
// reads the same column and was silently empty on password-only installs.
//
// The defect was an omitted call rather than a wrong one, so the regression
// worth guarding is that completeLogin stops recording it. Both local sign-in
// paths, plain and post-2FA, funnel through completeLogin; the OIDC path uses
// CompleteOidcLogin and records it separately.
func TestCompleteLoginRecordsLastLogin(t *testing.T) {
	t.Parallel()

	const file = "auth.go"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}

	var fn *ast.FuncDecl
	ast.Inspect(f, func(n ast.Node) bool {
		d, ok := n.(*ast.FuncDecl)
		if ok && d.Name.Name == "completeLogin" {
			fn = d
			return false
		}
		return true
	})
	if fn == nil {
		t.Fatal("completeLogin not found in auth.go; if it was renamed, move this guard with it")
	}

	var found bool
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "UpdateLastLogin" {
			return true
		}
		found = true
		return false
	})

	if !found {
		t.Error("completeLogin does not call UpdateLastLogin: local sign-ins will show Last Login as Never")
	}
}

// The two local sign-in paths must keep funnelling through completeLogin. If a
// third is added that does not, it reintroduces the same gap for that path.
func TestLocalLoginPathsGoThroughCompleteLogin(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "auth.go", nil, 0)
	if err != nil {
		t.Fatalf("parse auth.go: %v", err)
	}

	calls := 0
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "completeLogin" {
			calls++
		}
		return true
	})

	if calls < 2 {
		t.Errorf("found %d calls to completeLogin, want at least 2 (plain sign-in and post-2FA)", calls)
	}
}
