package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrReleaseNotesFKViolation indicates user_id does not exist in users (e.g. user deleted or session stale).
var ErrReleaseNotesFKViolation = errors.New("user not found - session may have expired")

// ErrReleaseNotesTableMissing indicates the release_notes_acceptances table does not exist (migrations not run).
var ErrReleaseNotesTableMissing = errors.New("release_notes_acceptances table missing - migrations may not have run")

// ReleaseNotesAcceptanceStore handles release notes acceptance records.
type ReleaseNotesAcceptanceStore struct {
	db database.DBProvider
}

// NewReleaseNotesAcceptanceStore creates a new store.
func NewReleaseNotesAcceptanceStore(db database.DBProvider) *ReleaseNotesAcceptanceStore {
	return &ReleaseNotesAcceptanceStore{db: db}
}

// Upsert records that the user has accepted release notes for the given version.
// Uses INSERT; if the row already exists (unique violation), treats as success (idempotent).
func (s *ReleaseNotesAcceptanceStore) Upsert(ctx context.Context, userID, version string) error {
	d := s.db.DB(ctx)
	_, err := d.Queries.InsertReleaseNotesAcceptance(ctx, db.InsertReleaseNotesAcceptanceParams{
		UserID:  userID,
		Version: version,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23505":
				// Unique violation - already accepted, treat as success
				return nil
			case "23503":
				// Foreign key violation - user_id not in users
				return fmt.Errorf("%w: %v", ErrReleaseNotesFKViolation, err)
			case "42P01":
				// Undefined table
				return fmt.Errorf("%w: %v", ErrReleaseNotesTableMissing, err)
			}
		}
		return err
	}
	return nil
}

// GetAcceptedVersions returns the list of versions the user has accepted.
func (s *ReleaseNotesAcceptanceStore) GetAcceptedVersions(ctx context.Context, userID string) ([]string, error) {
	d := s.db.DB(ctx)
	return d.Queries.GetAcceptedVersionsByUserID(ctx, userID)
}
