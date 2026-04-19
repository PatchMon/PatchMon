package store

import (
	"context"
	"errors"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SessionsStore provides user session access.
type SessionsStore struct {
	db database.DBProvider
}

// NewSessionsStore creates a new sessions store.
func NewSessionsStore(db database.DBProvider) *SessionsStore {
	return &SessionsStore{db: db}
}

// EnforceTfaRememberLimit removes oldest TFA remember sessions until count < max.
// Call before Create when tfaRememberMe is true.
func (s *SessionsStore) EnforceTfaRememberLimit(ctx context.Context, userID string, max int) error {
	d := s.db.DB(ctx)
	if max <= 0 {
		return nil
	}
	for {
		count, err := d.Queries.CountTfaRememberSessionsByUser(ctx, userID)
		if err != nil {
			return err
		}
		if int(count) < max {
			return nil
		}
		if err := d.Queries.DeleteOldestTfaRememberSession(ctx, userID); err != nil {
			return err
		}
	}
}

// Create creates a new session.
// tfaBypassUntil is set when tfaRememberMe is true (until when TFA can be bypassed on this device).
// deviceID is the X-Device-ID from the client (stable across IP changes).
func (s *SessionsStore) Create(ctx context.Context, userID, refreshToken, accessTokenHash, ip, userAgent, deviceFingerprint, deviceID string, expiresAt time.Time, tfaRememberMe bool, tfaBypassUntil *time.Time) (*models.UserSession, error) {
	d := s.db.DB(ctx)
	id := uuid.New().String()
	now := time.Now()
	arg := db.CreateSessionParams{
		ID:                id,
		UserID:            userID,
		RefreshToken:      refreshToken,
		AccessTokenHash:   strPtr(accessTokenHash),
		IpAddress:         strPtr(ip),
		UserAgent:         strPtr(userAgent),
		DeviceFingerprint: strPtr(deviceFingerprint),
		DeviceID:          strPtr(deviceID),
		LastActivity:      pgtype.Timestamp{Time: now, Valid: true},
		ExpiresAt:         pgtype.Timestamp{Time: expiresAt, Valid: true},
		CreatedAt:         pgtype.Timestamp{Time: now, Valid: true},
		TfaRememberMe:     tfaRememberMe,
		TfaBypassUntil:    timeToPgtype(tfaBypassUntil),
		LastLoginIp:       strPtr(ip),
	}
	if err := d.Queries.CreateSession(ctx, arg); err != nil {
		return nil, err
	}
	var bypass *time.Time
	if tfaBypassUntil != nil {
		bypass = tfaBypassUntil
	}
	return &models.UserSession{
		ID: id, UserID: userID, RefreshToken: refreshToken,
		AccessTokenHash: strPtr(accessTokenHash), IPAddress: strPtr(ip), UserAgent: strPtr(userAgent),
		DeviceFingerprint: strPtr(deviceFingerprint), LastActivity: now, ExpiresAt: expiresAt,
		CreatedAt: now, IsRevoked: false, TfaRememberMe: tfaRememberMe, TfaBypassUntil: bypass, LoginCount: 1, LastLoginIP: strPtr(ip),
	}, nil
}

// CreateOrReuseSession creates a new session or reuses (updates) an existing one for the same user+device.
// deviceID (X-Device-ID) is preferred for lookup -stable across IP changes. deviceFingerprint is fallback.
// When the same device logs in again, the existing session is updated (refresh token, login_count incremented).
func (s *SessionsStore) CreateOrReuseSession(ctx context.Context, userID, refreshToken, accessTokenHash, ip, userAgent, deviceFingerprint, deviceID string, expiresAt time.Time, tfaRememberMe bool, tfaBypassUntil *time.Time) (*models.UserSession, error) {
	d := s.db.DB(ctx)
	// Prefer device_id lookup (stable when IP changes); fallback to fingerprint
	if deviceID != "" {
		existing, err := d.Queries.FindSessionByUserAndDeviceID(ctx, db.FindSessionByUserAndDeviceIDParams{
			UserID:   userID,
			DeviceID: &deviceID,
		})
		if err == nil {
			return s.updateExistingSession(ctx, existing, refreshToken, ip, userAgent, deviceID, expiresAt, tfaRememberMe, tfaBypassUntil)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	if deviceFingerprint != "" {
		existing, err := d.Queries.FindSessionByUserAndDevice(ctx, db.FindSessionByUserAndDeviceParams{
			UserID:            userID,
			DeviceFingerprint: &deviceFingerprint,
		})
		if err == nil {
			return s.updateExistingSession(ctx, existing, refreshToken, ip, userAgent, deviceID, expiresAt, tfaRememberMe, tfaBypassUntil)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	return s.Create(ctx, userID, refreshToken, accessTokenHash, ip, userAgent, deviceFingerprint, deviceID, expiresAt, tfaRememberMe, tfaBypassUntil)
}

func (s *SessionsStore) updateExistingSession(ctx context.Context, existing db.UserSession, refreshToken, ip, userAgent, deviceID string, expiresAt time.Time, tfaRememberMe bool, tfaBypassUntil *time.Time) (*models.UserSession, error) {
	d := s.db.DB(ctx)
	now := time.Now()
	var devID *string
	if deviceID != "" {
		devID = &deviceID
	}
	updateArg := db.UpdateSessionOnLoginParams{
		ID:             existing.ID,
		RefreshToken:   refreshToken,
		LastActivity:   pgtype.Timestamp{Time: now, Valid: true},
		ExpiresAt:      pgtype.Timestamp{Time: expiresAt, Valid: true},
		IpAddress:      strPtr(ip),
		UserAgent:      strPtr(userAgent),
		LastLoginIp:    strPtr(ip),
		TfaRememberMe:  tfaRememberMe,
		TfaBypassUntil: timeToPgtype(tfaBypassUntil),
		DeviceID:       devID,
		UserID:         existing.UserID,
	}
	if err := d.Queries.UpdateSessionOnLogin(ctx, updateArg); err != nil {
		return nil, err
	}
	out := dbUserSessionToModel(existing)
	out.RefreshToken = refreshToken
	out.LastActivity = now
	out.ExpiresAt = expiresAt
	out.IPAddress = strPtr(ip)
	out.UserAgent = strPtr(userAgent)
	out.LastLoginIP = strPtr(ip)
	out.TfaRememberMe = tfaRememberMe
	out.TfaBypassUntil = tfaBypassUntil
	out.LoginCount = int(existing.LoginCount) + 1
	return &out, nil
}

func timeToPgtype(t *time.Time) pgtype.Timestamp {
	if t == nil {
		return pgtype.Timestamp{Valid: false}
	}
	return pgtype.Timestamp{Time: *t, Valid: true}
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// GetByRefreshToken returns a session by refresh token.
func (s *SessionsStore) GetByRefreshToken(ctx context.Context, refreshToken string) (*models.UserSession, error) {
	d := s.db.DB(ctx)
	u, err := d.Queries.GetSessionByRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, err
	}
	out := dbUserSessionToModel(u)
	return &out, nil
}

// GetByID returns a session by ID and user ID.
func (s *SessionsStore) GetByID(ctx context.Context, id, userID string) (*models.UserSession, error) {
	d := s.db.DB(ctx)
	u, err := d.Queries.GetSessionByID(ctx, db.GetSessionByIDParams{ID: id, UserID: userID})
	if err != nil {
		return nil, err
	}
	out := dbUserSessionToModel(u)
	return &out, nil
}

// ListByUserID returns all sessions for a user.
func (s *SessionsStore) ListByUserID(ctx context.Context, userID string) ([]models.UserSession, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.ListSessionsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]models.UserSession, len(rows))
	for i := range rows {
		out[i] = dbUserSessionToModel(rows[i])
	}
	return out, nil
}

// RevokeByID revokes a session by ID.
func (s *SessionsStore) RevokeByID(ctx context.Context, id, userID string) error {
	d := s.db.DB(ctx)
	return d.Queries.RevokeSessionByID(ctx, db.RevokeSessionByIDParams{ID: id, UserID: userID})
}

// RevokeAllForUser revokes all sessions for a user except the given session ID.
func (s *SessionsStore) RevokeAllForUser(ctx context.Context, userID, exceptSessionID string) error {
	d := s.db.DB(ctx)
	if exceptSessionID != "" {
		return d.Queries.RevokeAllSessionsForUserExcept(ctx, db.RevokeAllSessionsForUserExceptParams{UserID: userID, ID: exceptSessionID})
	}
	return d.Queries.RevokeAllSessionsForUser(ctx, userID)
}

// UpdateActivity updates last_activity for a session.
func (s *SessionsStore) UpdateActivity(ctx context.Context, id string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateSessionActivity(ctx, id)
}

// FindSessionWithTfaBypass returns a valid remember-me session for the user+device, if any.
// Returns nil when deviceFingerprint is empty.
//
// Deprecated: the fingerprint-based TFA bypass is replaced by the cookie-backed
// user_trusted_devices table. Use TrustedDevicesStore.FindValid instead.
// Scheduled for removal with migration 000037 alongside the
// tfa_remember_me / tfa_bypass_until / device_fingerprint columns.
func (s *SessionsStore) FindSessionWithTfaBypass(ctx context.Context, userID, deviceFingerprint string) (*models.UserSession, error) {
	d := s.db.DB(ctx)
	if deviceFingerprint == "" {
		return nil, nil
	}
	u, err := d.Queries.FindSessionWithTfaBypass(ctx, db.FindSessionWithTfaBypassParams{
		UserID:            userID,
		DeviceFingerprint: &deviceFingerprint,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	out := dbFindSessionWithTfaBypassRowToModel(u)
	return &out, nil
}
