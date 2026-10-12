package handler

import "github.com/PatchMon/PatchMon/server-source-code/internal/config"

// Helpers for the community-button and outbound-connection switches
// (HIDE_* / DISABLE_* in config). A nil config means "no switches set", which
// keeps the pre-switch behaviour.

// showDonate reports whether donate / membership buttons may be shown.
func showDonate(cfg *config.Config) bool {
	return cfg == nil || (!cfg.AdminMode && !cfg.HideDonateButtons)
}

// discordAuthEnabled reports whether Discord login / linking is available.
func discordAuthEnabled(cfg *config.Config) bool {
	return cfg == nil || !cfg.DisableDiscordAuth
}

// telemetryEnabled reports whether anonymous metrics may be sent.
func telemetryEnabled(cfg *config.Config) bool {
	return cfg == nil || !cfg.DisableTelemetry
}

// githubVersionOnLoginAllowed reports whether the login page may query GitHub.
func githubVersionOnLoginAllowed(cfg *config.Config) bool {
	return cfg == nil || !cfg.DisableGithubVersionOnLogin
}

// newsletterEnabled reports whether newsletter sign-up may be offered.
func newsletterEnabled(cfg *config.Config) bool {
	return cfg == nil || (!cfg.AdminMode && !cfg.DisableNewsletter)
}
