package handler

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

// #1043: every rejected sign-in was logged at Debug while the default log level
// is info, so a stock install recorded nothing at all. The unknown-username
// branch was worse: it neither logged nor counted toward the lockout, so the
// reporter's exact repro (random user and password) left no trace anywhere.

func TestTruncateForLog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"short string is untouched", "admin", 64, "admin"},
		{"empty is untouched", "", 64, ""},
		{"exactly at the limit is untouched", "abcde", 5, "abcde"},
		{"over the limit is cut and marked", "abcdef", 5, "abcde..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := truncateForLog(tt.in, tt.max); got != tt.want {
				t.Errorf("truncateForLog(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.want)
			}
		})
	}
}

// A byte-wise cut can land mid-rune. The result still has to be valid UTF-8,
// otherwise the JSON log handler emits replacement characters.
func TestTruncateForLogKeepsValidUTF8(t *testing.T) {
	t.Parallel()

	const multibyte = "日本語テスト"
	for max := 1; max < len(multibyte); max++ {
		got := truncateForLog(multibyte, max)
		if !utf8.ValidString(got) {
			t.Errorf("truncateForLog(%q, %d) = %q, which is not valid UTF-8", multibyte, max, got)
		}
	}
}

func TestLoginFailuresAreLoggedAboveDebug(t *testing.T) {
	t.Parallel()

	fn := findFunc(t, "auth.go", "logLoginFailure")
	calls := loggerMethodsCalled(fn)

	if !calls["Warn"] {
		t.Error("logLoginFailure does not call h.log.Warn; failed sign-ins are invisible at the default log level, which is the whole point of #1043")
	}
	if calls["Debug"] {
		t.Error("logLoginFailure calls h.log.Debug; Debug is hidden at the default log level")
	}
}

// The unknown-username branch must record a failed attempt too. The lockout key
// is IP plus username, so this does not throttle spraying across many invented
// names; what it removes is the enumeration oracle, where a 429 could only ever
// be reached for a username that exists.
func TestLoginCountsUnknownUsernamesTowardLockout(t *testing.T) {
	t.Parallel()

	fn := findFunc(t, "auth.go", "Login")

	var recorded int
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "RecordFailedAttempt" {
			recorded++
		}
		return true
	})

	if recorded < 2 {
		t.Errorf("Login calls RecordFailedAttempt %d time(s), want at least 2 (one for a wrong password, one for an unknown username)", recorded)
	}
}

// GetByUsernameOrEmail returns the raw driver error, so "no such user" and "the
// database is unreachable" arrive on the same branch. Since that branch now
// consumes a lockout attempt, failing to separate them would let a database
// outage lock out the users retrying a correct password.
func TestLoginSeparatesMissingUserFromDatabaseFailure(t *testing.T) {
	t.Parallel()

	fn := findFunc(t, "auth.go", "Login")

	var checksNoRows bool
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Is" {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "errors" {
			return true
		}
		for _, arg := range call.Args {
			if argSel, ok := arg.(*ast.SelectorExpr); ok && argSel.Sel.Name == "ErrNoRows" {
				checksNoRows = true
			}
		}
		return true
	})

	if !checksNoRows {
		t.Error("Login does not test the user lookup error with errors.Is(err, pgx.ErrNoRows); a database failure would be counted as a wrong username and consume a lockout attempt")
	}
}

// The stored bcrypt hash has no business in a log line at any level.
func TestLoginDoesNotLogPasswordHash(t *testing.T) {
	t.Parallel()

	src := readSource(t, "auth.go")
	for _, banned := range []string{"hash_prefix", "hashPrefix"} {
		if strings.Contains(src, banned) {
			t.Errorf("auth.go still references %q; the stored hash must not reach the log", banned)
		}
	}
}

func readSource(t *testing.T, file string) string {
	t.Helper()

	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	return string(b)
}

func findFunc(t *testing.T, file, name string) *ast.FuncDecl {
	t.Helper()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}

	var fn *ast.FuncDecl
	ast.Inspect(f, func(n ast.Node) bool {
		d, ok := n.(*ast.FuncDecl)
		if ok && d.Name.Name == name {
			fn = d
			return false
		}
		return true
	})
	if fn == nil {
		t.Fatalf("%s not found in %s; if it was renamed, move this guard with it", name, file)
	}
	return fn
}

// loggerMethodsCalled collects the method names invoked on h.log specifically,
// so an unrelated call such as strings.TrimSpace cannot satisfy the assertion.
func loggerMethodsCalled(fn *ast.FuncDecl) map[string]bool {
	called := map[string]bool{}
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		recv, ok := sel.X.(*ast.SelectorExpr)
		if !ok || recv.Sel.Name != "log" {
			return true
		}
		if ident, ok := recv.X.(*ast.Ident); !ok || ident.Name != "h" {
			return true
		}
		called[sel.Sel.Name] = true
		return true
	})
	return called
}
