package config

import (
	"regexp"
	"strings"

	"patchmon-agent/internal/pkgversion"
	"patchmon-agent/pkg/models"
)

var hostIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// ValidHostID reports whether id has the shape of a PatchMon host ID.
func ValidHostID(id string) bool {
	return hostIDPattern.MatchString(id)
}

type configMigration struct {
	version string
	apply   func(c *models.Config) bool
}

// Steps are keyed by the release that introduced them and must be idempotent:
// an rc config reruns its release's step when the final release loads it.
var configMigrations = []configMigration{
	{"2.1.4", migrate214},
}

func migrate214(c *models.Config) bool {
	id := strings.TrimSpace(c.HostID)
	if id != "" && !ValidHostID(id) {
		id = ""
	}
	changed := id != c.HostID
	c.HostID = id
	return changed
}

// Step selection deliberately ignores binaryVersion, so a dev build still
// applies the steps it carries.
func migrateConfig(c *models.Config, binaryVersion string, steps []configMigration) []string {
	if c.ConfigVersion == binaryVersion {
		return nil
	}
	var ran []string
	for _, step := range steps {
		if pkgversion.Compare(step.version, c.ConfigVersion) > 0 {
			step.apply(c)
			ran = append(ran, step.version)
		}
	}
	c.ConfigVersion = binaryVersion
	return ran
}
