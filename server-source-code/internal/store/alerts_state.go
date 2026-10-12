package store

import (
	"context"
	"strings"
)

// ActionSilenced is the alert action that the Alerts list shows as the Silenced status.
const ActionSilenced = "silenced"

// IsSilencedState reports whether a latest-history action puts an alert in the Silenced status.
func IsSilencedState(action string) bool {
	return strings.EqualFold(action, ActionSilenced)
}

// ActiveAlertState is an active alert with the action of its latest history row.
type ActiveAlertState struct {
	ID       string
	Metadata []byte
	Action   string
}

// ActiveByTypeWithState returns the active alerts of a type with their current state, as the Alerts list derives it.
func (s *AlertsStore) ActiveByTypeWithState(ctx context.Context, alertType string) ([]ActiveAlertState, error) {
	d := s.db.DB(ctx)
	rows, err := d.Raw(ctx, `SELECT a.id, a.metadata, COALESCE(h.action, '')
FROM alerts a`+latestHistoryJoinSQL+`
WHERE a.type = $1 AND a.is_active = true`, alertType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ActiveAlertState
	for rows.Next() {
		var a ActiveAlertState
		if err := rows.Scan(&a.ID, &a.Metadata, &a.Action); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
