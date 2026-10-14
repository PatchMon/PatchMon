package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type noRows struct{}

func (noRows) Close()                                       {}
func (noRows) Err() error                                   { return nil }
func (noRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("SELECT 0") }
func (noRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (noRows) Next() bool                                   { return false }
func (noRows) Scan(...any) error                            { return nil }
func (noRows) Values() ([]any, error)                       { return nil, nil }
func (noRows) RawValues() [][]byte                          { return nil }
func (noRows) Conn() *pgx.Conn                              { return nil }

type hostQueryDB struct{ err error }

func (f hostQueryDB) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, f.err
}

func (f hostQueryDB) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	if f.err != nil {
		return nil, f.err
	}
	return noRows{}, nil
}

func (f hostQueryDB) QueryRow(context.Context, string, ...interface{}) pgx.Row { return nil }

type fixedDB struct{ d *database.DB }

func (p fixedDB) DB(context.Context) *database.DB { return p.d }

func TestLoadHostSeparatesNotFoundFromLookupFailure(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want int
	}{
		"unknown host":  {nil, http.StatusNotFound},
		"lookup failed": {errors.New(`column "config_version" does not exist`), http.StatusInternalServerError},
	} {
		hosts := store.NewHostsStore(fixedDB{d: &database.DB{Queries: db.New(hostQueryDB{err: tc.err})}})
		rec := httptest.NewRecorder()
		if _, ok := loadHost(rec, httptest.NewRequest(http.MethodPost, "/x", nil), hosts, "h"); ok {
			t.Fatalf("%s: loadHost succeeded", name)
		}
		if rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d", name, rec.Code, tc.want)
		}
	}
}
