package migrate_test

import (
	"context"
	"os"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/jackc/pgx/v5"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Main(m)) }

func TestOidcProviderIssuerMigration(t *testing.T) {
	ctx := context.Background()
	up, err := os.ReadFile("migrations/000058_v2-1-4_oidc_provider_issuer.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })

	type row struct{ id, sub, provider *string }
	str := func(s string) *string { return &s }
	fixtures := []row{
		{str("match"), str("sub-match"), str("IDP.example.com")},
		{str("other-host"), str("sub-other"), str("other.example.com")},
		{str("full-url"), str("sub-full"), str("https://old.example.com/realms/x")},
		{str("null-provider"), str("sub-null"), nil},
		{str("no-sub"), nil, str("idp.example.com")},
	}

	run := func(t *testing.T, enabled bool, issuer *string) map[string]*string {
		t.Helper()
		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, `UPDATE settings SET oidc_enabled = $1, oidc_issuer_url = $2`, enabled, issuer); err != nil {
			t.Fatal(err)
		}
		for _, f := range fixtures {
			if _, err := tx.Exec(ctx, `INSERT INTO users (id, username, email, updated_at, oidc_sub, oidc_provider)
				VALUES ($1, $1, $1 || '@example.com', NOW(), $2, $3)`, *f.id, f.sub, f.provider); err != nil {
				t.Fatal(err)
			}
		}
		for i := 0; i < 2; i++ {
			if _, err := tx.Exec(ctx, string(up)); err != nil {
				t.Fatalf("run %d: %v", i+1, err)
			}
		}
		got := map[string]*string{}
		rows, err := tx.Query(ctx, `SELECT id, oidc_provider FROM users WHERE id = ANY($1)`,
			[]string{"match", "other-host", "full-url", "null-provider", "no-sub"})
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var p *string
			if err := rows.Scan(&id, &p); err != nil {
				t.Fatal(err)
			}
			got[id] = p
		}
		return got
	}
	val := func(p *string) string {
		if p == nil {
			return "<nil>"
		}
		return *p
	}
	unchanged := func(t *testing.T, got map[string]*string, ids ...string) {
		t.Helper()
		for _, f := range fixtures {
			for _, id := range ids {
				if *f.id == id && val(got[id]) != val(f.provider) {
					t.Errorf("%s: provider %q, want unchanged %q", id, val(got[id]), val(f.provider))
				}
			}
		}
	}

	t.Run("binds hostname-only rows whose host matches the configured issuer", func(t *testing.T) {
		got := run(t, true, str(" https://idp.example.com/realms/pm/.well-known/openid-configuration "))
		if val(got["match"]) != "https://idp.example.com/realms/pm" {
			t.Errorf("match: provider %q", val(got["match"]))
		}
		unchanged(t, got, "other-host", "full-url", "null-provider", "no-sub")
	})

	t.Run("leaves everything alone without a usable configured issuer", func(t *testing.T) {
		all := []string{"match", "other-host", "full-url", "null-provider", "no-sub"}
		unchanged(t, run(t, false, str("https://idp.example.com")), all...)
		unchanged(t, run(t, true, nil), all...)
		unchanged(t, run(t, true, str("not a url")), all...)
	})
}
