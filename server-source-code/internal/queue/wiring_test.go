package queue

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestStoresInQueueWiringUseAContextResolver guards a mistake the type system
// cannot catch.
//
// Stores take a hostctx.DBProvider. A raw *database.DB satisfies that interface,
// because (*DB).DB(ctx) ignores its context and returns the receiver:
//
//	func (d *DB) DB(_ context.Context) *DB { return d }
//
// That exists so single-context installs can pass a plain handle. The
// consequence is that store.NewXxxStore(db) compiles, passes review, and
// silently pins the store to whichever database DATABASE_URL points at. In a
// multi-context deployment every job then reads and writes the wrong customer's
// data, with no error and no log line.
//
// This is not hypothetical: the compliance store was wired that way, so every
// scan triggered by a non-default context wrote its profile rows into the
// default context's database and its scan insert failed a foreign-key check
// that was being discarded.
//
// Queue handlers are different and are deliberately not checked here: they take
// (db, poolCache) and resolve per job internally.
func TestStoresInQueueWiringUseAContextResolver(t *testing.T) {
	t.Parallel()

	const file = "server.go"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}

	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "store" || !strings.HasPrefix(sel.Sel.Name, "New") {
			return true
		}
		// A bare identifier as the first argument means a concrete handle was
		// passed where a resolver belongs.
		if ident, ok := call.Args[0].(*ast.Ident); ok && ident.Name == "db" {
			t.Errorf("%s: store.%s is constructed with the raw default database.\n"+
				"Pass &hostctx.DBResolver{Default: db} instead, or the store resolves to the\n"+
				"default context for every job regardless of which context raised it.",
				fset.Position(call.Pos()), sel.Sel.Name)
		}
		return true
	})
}
