package queue

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestStoresInQueueWiringUseAContextResolver guards a mistake the compiler
// cannot catch: a raw *database.DB satisfies hostctx.DBProvider, because
// (*DB).DB(ctx) ignores its context and returns the receiver. So passing one to
// a store compiles and silently pins it to the default database.
//
// Queue handlers are not checked here: they take (db, poolCache) and resolve
// per job internally.
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
		if ident, ok := call.Args[0].(*ast.Ident); ok && ident.Name == "db" {
			t.Errorf("%s: store.%s takes the raw default database. "+
				"Pass &hostctx.DBResolver{Default: db} instead.",
				fset.Position(call.Pos()), sel.Sel.Name)
		}
		return true
	})
}
