package server

import (
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	redisclient "github.com/redis/go-redis/v9"
)

// NewRouter registers the /auth/discord routes only when the handler exists,
// and it exists only when discordAuthActive is true.
func TestDiscordAuthActive(t *testing.T) {
	rdb := redisclient.NewClient(&redisclient.Options{Addr: "127.0.0.1:0"}) // never dialled
	t.Cleanup(func() { _ = rdb.Close() })

	tests := []struct {
		name string
		cfg  *config.Config
		rdb  *redisclient.Client
		want bool
	}{
		{"enabled with redis", &config.Config{}, rdb, true},
		{"DISABLE_DISCORD_AUTH", &config.Config{DisableDiscordAuth: true}, rdb, false},
		{"no redis", &config.Config{}, nil, false},
	}
	for _, tt := range tests {
		if got := discordAuthActive(tt.cfg, tt.rdb); got != tt.want {
			t.Errorf("%s: discordAuthActive = %v, want %v", tt.name, got, tt.want)
		}
	}
}
