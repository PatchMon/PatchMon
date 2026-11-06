package store

import (
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
)

func TestSettingsAptDisableRetriesRoundTrip(t *testing.T) {
	t.Parallel()

	for _, want := range []bool{true, false} {
		params := settingsToUpdateParams(&models.Settings{AptDisableRetries: want})
		if params.AptDisableRetries != want {
			t.Errorf("update params AptDisableRetries = %v, want %v", params.AptDisableRetries, want)
		}
		if got := dbSettingToModel(db.Setting{AptDisableRetries: want}).AptDisableRetries; got != want {
			t.Errorf("model AptDisableRetries = %v, want %v", got, want)
		}
	}
}
