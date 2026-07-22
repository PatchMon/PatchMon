package store

import (
	"context"
	"errors"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type SSHStore struct{ db database.DBProvider }

func NewSSHStore(provider database.DBProvider) *SSHStore { return &SSHStore{db: provider} }

func (s *SSHStore) ListAccounts(ctx context.Context, hostID string) ([]db.SshHostAccount, error) {
	return s.db.DB(ctx).Queries.ListSSHHostAccounts(ctx, hostID)
}

func (s *SSHStore) AccountAllowed(ctx context.Context, hostID, username string) (db.SshHostAccount, bool, error) {
	account, err := s.db.DB(ctx).Queries.GetSSHHostAccount(ctx, db.GetSSHHostAccountParams{HostID: hostID, LinuxUsername: username})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.SshHostAccount{}, false, nil
	}
	return account, err == nil, err
}

func (s *SSHStore) UpsertAccount(ctx context.Context, hostID, username string, allowSudo bool) (db.SshHostAccount, error) {
	return s.db.DB(ctx).Queries.UpsertSSHHostAccount(ctx, db.UpsertSSHHostAccountParams{
		ID: uuid.NewString(), HostID: hostID, LinuxUsername: username, AllowSudo: allowSudo,
	})
}

func (s *SSHStore) DeleteAccount(ctx context.Context, hostID, username string) error {
	return s.db.DB(ctx).Queries.DeleteSSHHostAccount(ctx, db.DeleteSSHHostAccountParams{HostID: hostID, LinuxUsername: username})
}

type CreateSSHSessionParams struct {
	ID, HostID, UserID, LinuxUsername, Transport, ClientIP, UserAgent string
	Recorded                                                          bool
}

func (s *SSHStore) CreateSession(ctx context.Context, value CreateSSHSessionParams) (db.SshSession, error) {
	clientIP, userAgent := optionalString(value.ClientIP), optionalString(value.UserAgent)
	return s.db.DB(ctx).Queries.CreateSSHSession(ctx, db.CreateSSHSessionParams{
		ID: value.ID, HostID: value.HostID, UserID: value.UserID,
		LinuxUsername: value.LinuxUsername, Transport: value.Transport, Status: "opening",
		ClientIp: clientIP, UserAgent: userAgent, Recorded: value.Recorded,
	})
}

func (s *SSHStore) ActiveCounts(ctx context.Context, userID, hostID string) (int64, int64, error) {
	userCount, err := s.db.DB(ctx).Queries.CountActiveSSHSessionForUser(ctx, userID)
	if err != nil {
		return 0, 0, err
	}
	hostCount, err := s.db.DB(ctx).Queries.CountActiveSSHSessionForHost(ctx, hostID)
	return userCount, hostCount, err
}

func (s *SSHStore) UpdateSession(ctx context.Context, id, status, reason string, events, bytes int64) (db.SshSession, error) {
	return s.db.DB(ctx).Queries.UpdateSSHSessionStatus(ctx, db.UpdateSSHSessionStatusParams{
		ID: id, Status: status, FailureReason: optionalString(reason), EventCount: events, RecordingBytes: bytes,
	})
}

func (s *SSHStore) GetSession(ctx context.Context, id string) (db.SshSession, error) {
	return s.db.DB(ctx).Queries.GetSSHSession(ctx, id)
}

func (s *SSHStore) AuditRecordingAccess(ctx context.Context, sessionID, userID, action, clientIP string) error {
	_, err := s.db.DB(ctx).Queries.CreateSSHRecordingAccessAudit(ctx, db.CreateSSHRecordingAccessAuditParams{
		ID: uuid.NewString(), SessionID: sessionID, UserID: userID, Action: action, ClientIp: optionalString(clientIP),
	})
	return err
}

func (s *SSHStore) ExpiredRecordings(ctx context.Context, before time.Time, limit int32) ([]db.SshSession, error) {
	return s.db.DB(ctx).Queries.ListExpiredSSHRecordings(ctx, db.ListExpiredSSHRecordingsParams{
		StartedAt: pgtype.Timestamp{Time: before, Valid: true}, Limit: limit,
	})
}

func (s *SSHStore) MarkRecordingDeleted(ctx context.Context, id string) error {
	return s.db.DB(ctx).Queries.MarkSSHRecordingDeleted(ctx, id)
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (s *SSHStore) ListRecordings(ctx context.Context, params db.ListSSHSessionRecordingsParams) ([]db.ListSSHSessionRecordingsRow, error) {
	if params.LimitCount < 1 || params.LimitCount > 200 {
		params.LimitCount = 50
	}
	if params.OffsetCount < 0 {
		params.OffsetCount = 0
	}
	return s.db.DB(ctx).Queries.ListSSHSessionRecordings(ctx, params)
}
