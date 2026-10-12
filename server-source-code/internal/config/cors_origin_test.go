package config

import (
	"context"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
)

func TestResolveConfig_CORSOriginPrecedence(t *testing.T) {
	cases := []struct {
		name string
		env  string
		db   *string
		want string
	}{
		{name: "env list wins over a db value", env: "http://a.example,http://b.example", db: strPtr("http://c.example"), want: "http://a.example,http://b.example"},
		{name: "db list applies when env is unset", db: strPtr("http://a.example,http://b.example"), want: "http://a.example,http://b.example"},
		{name: "db single origin applies when env is unset", db: strPtr("http://a.example"), want: "http://a.example"},
		{name: "empty db value falls back to the default", db: strPtr(""), want: "http://localhost:3000"},
		{name: "blank db value falls back to the default", db: strPtr("   "), want: "http://localhost:3000"},
		{name: "nil db value falls back to the default", want: "http://localhost:3000"},
		{name: "env value is trimmed", env: "  http://a.example ", want: "http://a.example"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CORS_ORIGIN", tc.env)
			cfg := &Config{CORSOrigin: "http://localhost:3000"}
			got := ResolveConfig(context.Background(), cfg, &models.Settings{CorsOrigin: tc.db})
			if got.CORSOrigin != tc.want {
				t.Errorf("CORSOrigin = %q, want %q", got.CORSOrigin, tc.want)
			}
		})
	}
}
