package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TrustedDevicesStore manages MFA "remember this device" trust tokens.
//
// The store never exposes raw token values; callers pass them in (on Create)
// or provide them from an inbound cookie (on FindValid). Persisted state is
// always the SHA-256 hash of the raw token.
type TrustedDevicesStore struct {
	db database.DBProvider
}

// NewTrustedDevicesStore creates a new trusted devices store.
func NewTrustedDevicesStore(db database.DBProvider) *TrustedDevicesStore {
	return &TrustedDevicesStore{db: db}
}

// TrustTokenBytes is the byte length of the raw trust token (256 bits).
const TrustTokenBytes = 32

// trustTokenEncodedLen is the length of a base64url-encoded TrustTokenBytes payload.
// base64.RawURLEncoding.EncodedLen(32) == 43.
const trustTokenEncodedLen = 43

// GenerateTrustToken returns (rawTokenB64, tokenHashHex). The raw token is
// placed in the HttpOnly cookie; the hash is stored server-side. Both sides
// hash the encoded form so round-tripping through the cookie cannot desync
// the two halves.
func GenerateTrustToken() (string, string, error) {
	buf := make([]byte, TrustTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw := base64.RawURLEncoding.EncodeToString(buf)
	return raw, hashEncodedToken(raw), nil
}

// HashTrustToken returns the hex SHA-256 of the base64url-encoded raw token
// carried by the patchmon_device_trust cookie. Returns empty string for any
// value that cannot be a valid token (empty, wrong length, or non-base64url)
// so the calling lookup becomes a guaranteed miss.
func HashTrustToken(raw string) string {
	if len(raw) != trustTokenEncodedLen {
		return ""
	}
	if _, err := base64.RawURLEncoding.DecodeString(raw); err != nil {
		return ""
	}
	return hashEncodedToken(raw)
}

func hashEncodedToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// CreateParams captures optional display metadata for a new trust record.
type CreateTrustedDeviceParams struct {
	UserID    string
	TokenHash string
	DeviceID  string
	UserAgent string
	IPAddress string
	Label     string
	ExpiresAt time.Time
}

// Create inserts a new trusted-device row. The caller is responsible for
// generating the raw token, setting the cookie, and passing in the hash.
func (s *TrustedDevicesStore) Create(ctx context.Context, p CreateTrustedDeviceParams) (string, error) {
	d := s.db.DB(ctx)
	id := uuid.New().String()
	now := time.Now()
	arg := db.CreateTrustedDeviceParams{
		ID:         id,
		UserID:     p.UserID,
		TokenHash:  p.TokenHash,
		DeviceID:   strPtr(p.DeviceID),
		UserAgent:  strPtr(p.UserAgent),
		IpAddress:  strPtr(p.IPAddress),
		Label:      strPtr(p.Label),
		CreatedAt:  pgtime.From(now),
		LastUsedAt: pgtime.From(now),
		ExpiresAt:  pgtime.From(p.ExpiresAt),
	}
	if err := d.Queries.CreateTrustedDevice(ctx, arg); err != nil {
		return "", err
	}
	return id, nil
}

// FindValid returns a non-revoked, non-expired trust record matching
// (userID, tokenHash). Returns (nil, nil) on no match.
func (s *TrustedDevicesStore) FindValid(ctx context.Context, userID, tokenHash string) (*models.TrustedDevice, error) {
	if userID == "" || tokenHash == "" {
		return nil, nil
	}
	d := s.db.DB(ctx)
	row, err := d.Queries.FindValidTrustedDevice(ctx, db.FindValidTrustedDeviceParams{
		UserID:    userID,
		TokenHash: tokenHash,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	td := trustedDeviceFromDB(row)
	return &td, nil
}

// TouchLastUsed updates last_used_at. Best-effort; errors are returned but
// callers typically ignore them since a missed update is not user-visible.
func (s *TrustedDevicesStore) TouchLastUsed(ctx context.Context, id string) error {
	d := s.db.DB(ctx)
	return d.Queries.TouchTrustedDeviceLastUsed(ctx, db.TouchTrustedDeviceLastUsedParams{
		ID:         id,
		LastUsedAt: pgtime.Now(),
	})
}

// RevokeByID revokes a single trust record. The userID guard prevents one
// user from revoking another user's device.
func (s *TrustedDevicesStore) RevokeByID(ctx context.Context, id, userID string) error {
	d := s.db.DB(ctx)
	return d.Queries.RevokeTrustedDeviceByID(ctx, db.RevokeTrustedDeviceByIDParams{
		ID:     id,
		UserID: userID,
	})
}

// RevokeAllForUser revokes every non-revoked trust record for the user.
// Called on password change, TFA disable, and explicit "forget all devices".
func (s *TrustedDevicesStore) RevokeAllForUser(ctx context.Context, userID string) error {
	d := s.db.DB(ctx)
	return d.Queries.RevokeAllTrustedDevicesForUser(ctx, userID)
}

// ListForUser returns all non-revoked, non-expired trust records for the user,
// most-recently-used first.
func (s *TrustedDevicesStore) ListForUser(ctx context.Context, userID string) ([]models.TrustedDevice, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.ListTrustedDevicesForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]models.TrustedDevice, 0, len(rows))
	for _, r := range rows {
		out = append(out, trustedDeviceFromDB(r))
	}
	return out, nil
}

// DeleteExpired prunes all expired or revoked rows. Run periodically by a
// background sweep job.
func (s *TrustedDevicesStore) DeleteExpired(ctx context.Context) error {
	d := s.db.DB(ctx)
	return d.Queries.DeleteExpiredTrustedDevices(ctx)
}

func trustedDeviceFromDB(r db.UserTrustedDevice) models.TrustedDevice {
	return models.TrustedDevice{
		ID:         r.ID,
		UserID:     r.UserID,
		TokenHash:  r.TokenHash,
		DeviceID:   r.DeviceID,
		UserAgent:  r.UserAgent,
		IPAddress:  r.IpAddress,
		Label:      r.Label,
		CreatedAt:  r.CreatedAt.Time,
		LastUsedAt: r.LastUsedAt.Time,
		ExpiresAt:  r.ExpiresAt.Time,
		IsRevoked:  r.IsRevoked,
	}
}
