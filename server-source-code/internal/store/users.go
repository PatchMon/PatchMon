package store

import (
	"context"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// UsersStore provides user access.
type UsersStore struct {
	db database.DBProvider
}

// NewUsersStore creates a new users store.
func NewUsersStore(db database.DBProvider) *UsersStore {
	return &UsersStore{db: db}
}

// GetByID returns a user by ID.
func (s *UsersStore) GetByID(ctx context.Context, id string) (*models.User, error) {
	d := s.db.DB(ctx)
	u, err := d.Queries.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	out := dbUserToModel(u)
	return &out, nil
}

// GetByUsername returns a user by username (case-insensitive).
func (s *UsersStore) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	d := s.db.DB(ctx)
	u, err := d.Queries.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	out := dbUserToModel(u)
	return &out, nil
}

// GetByUsernameOrEmail returns a user by username or email (case-insensitive).
// Prefers username match over email to align with Node/Prisma findFirst behavior.
func (s *UsersStore) GetByUsernameOrEmail(ctx context.Context, identifier string) (*models.User, error) {
	d := s.db.DB(ctx)
	u, err := d.Queries.GetUserByUsernameOrEmail(ctx, identifier)
	if err != nil {
		return nil, err
	}
	out := dbUserToModel(u)
	return &out, nil
}

// ExistsByUsernameOrEmail returns true if a user exists with the given username or email, excluding userID if provided.
func (s *UsersStore) ExistsByUsernameOrEmail(ctx context.Context, username, email, excludeUserID string) (bool, error) {
	d := s.db.DB(ctx)
	arg := db.ExistsByUsernameOrEmailParams{Username: username, Email: email}
	if excludeUserID != "" {
		arg.ExcludeUserID = &excludeUserID
	}
	return d.Queries.ExistsByUsernameOrEmail(ctx, arg)
}

// CountSuperadmins returns count of superadmin users.
func (s *UsersStore) CountSuperadmins(ctx context.Context) (int, error) {
	d := s.db.DB(ctx)
	n, err := d.Queries.CountSuperadmins(ctx)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// CountActiveAdmins returns count of active admin users (excluding superadmins).
func (s *UsersStore) CountActiveAdmins(ctx context.Context) (int, error) {
	d := s.db.DB(ctx)
	n, err := d.Queries.CountActiveAdmins(ctx)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// GetByEmail returns a user by email.
func (s *UsersStore) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	d := s.db.DB(ctx)
	u, err := d.Queries.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	out := dbUserToModel(u)
	return &out, nil
}

// GetByOidcSub returns a user by OIDC subject.
func (s *UsersStore) GetByOidcSub(ctx context.Context, oidcSub string) (*models.User, error) {
	d := s.db.DB(ctx)
	u, err := d.Queries.GetUserByOidcSub(ctx, &oidcSub)
	if err != nil {
		return nil, err
	}
	out := dbUserToModel(u)
	return &out, nil
}

// GetByOidcSubOrEmail returns a user by OIDC subject or email (prefers oidc_sub match).
func (s *UsersStore) GetByOidcSubOrEmail(ctx context.Context, oidcSub, email string) (*models.User, error) {
	d := s.db.DB(ctx)
	u, err := d.Queries.GetUserByOidcSubOrEmail(ctx, db.GetUserByOidcSubOrEmailParams{
		OidcSub: &oidcSub,
		Lower:   email,
	})
	if err != nil {
		return nil, err
	}
	out := dbUserToModel(u)
	return &out, nil
}

// CreateOidcUser creates a new user from OIDC claims.
func (s *UsersStore) CreateOidcUser(ctx context.Context, u *models.User, oidcSub, oidcProvider string, avatarURL *string) error {
	d := s.db.DB(ctx)
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now
	arg := db.CreateOidcUserParams{
		ID:           u.ID,
		Username:     u.Username,
		Email:        u.Email,
		Role:         u.Role,
		CreatedAt:    pgtype.Timestamp{Time: now, Valid: true},
		UpdatedAt:    pgtype.Timestamp{Time: now, Valid: true},
		FirstName:    u.FirstName,
		LastName:     u.LastName,
		OidcSub:      &oidcSub,
		OidcProvider: &oidcProvider,
		AvatarUrl:    avatarURL,
	}
	return d.Queries.CreateOidcUser(ctx, arg)
}

// UpdateOidcLink links OIDC to an existing user.
func (s *UsersStore) UpdateOidcLink(ctx context.Context, userID, oidcSub, oidcProvider string, avatarURL *string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateUserOidcLink(ctx, db.UpdateUserOidcLinkParams{
		ID:           userID,
		OidcSub:      &oidcSub,
		OidcProvider: &oidcProvider,
		AvatarUrl:    avatarURL,
	})
}

// UpdateOidcProfile updates user profile from OIDC (last_login, avatar, name, role).
func (s *UsersStore) UpdateOidcProfile(ctx context.Context, userID string, lastLogin time.Time, avatarURL, firstName, lastName *string, role string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateUserOidcProfile(ctx, db.UpdateUserOidcProfileParams{
		ID:        userID,
		LastLogin: pgtype.Timestamp{Time: lastLogin, Valid: true},
		AvatarUrl: avatarURL,
		FirstName: firstName,
		LastName:  lastName,
		Role:      role,
	})
}

// List returns all users (for admin).
func (s *UsersStore) List(ctx context.Context, limit, offset int) ([]models.User, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.ListUsers(ctx, db.ListUsersParams{Limit: int32(limit), Offset: int32(offset)})
	if err != nil {
		return nil, err
	}
	out := make([]models.User, len(rows))
	for i := range rows {
		out[i] = dbUserToModel(rows[i])
	}
	return out, nil
}

// Count returns total user count.
func (s *UsersStore) Count(ctx context.Context) (int, error) {
	d := s.db.DB(ctx)
	n, err := d.Queries.CountUsers(ctx)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// CountAdmins returns count of admin users.
func (s *UsersStore) CountAdmins(ctx context.Context) (int, error) {
	d := s.db.DB(ctx)
	n, err := d.Queries.CountAdmins(ctx)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// Create creates a new user.
func (s *UsersStore) Create(ctx context.Context, u *models.User) error {
	d := s.db.DB(ctx)
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now
	arg := db.CreateUserParams{
		ID:              u.ID,
		Username:        u.Username,
		Email:           u.Email,
		PasswordHash:    u.PasswordHash,
		Role:            u.Role,
		IsActive:        u.IsActive,
		CreatedAt:       pgtype.Timestamp{Time: now, Valid: true},
		UpdatedAt:       pgtype.Timestamp{Time: now, Valid: true},
		TfaEnabled:      u.TfaEnabled,
		FirstName:       u.FirstName,
		LastName:        u.LastName,
		ThemePreference: u.ThemePreference,
		ColorTheme:      u.ColorTheme,
	}
	return d.Queries.CreateUser(ctx, arg)
}

// Update updates a user.
func (s *UsersStore) Update(ctx context.Context, u *models.User) error {
	d := s.db.DB(ctx)
	u.UpdatedAt = time.Now()
	arg := db.UpdateUserParams{
		Username:        u.Username,
		Email:           u.Email,
		Role:            u.Role,
		IsActive:        u.IsActive,
		UpdatedAt:       pgtype.Timestamp{Time: u.UpdatedAt, Valid: true},
		FirstName:       u.FirstName,
		LastName:        u.LastName,
		ThemePreference: u.ThemePreference,
		ColorTheme:      u.ColorTheme,
		ID:              u.ID,
	}
	return d.Queries.UpdateUser(ctx, arg)
}

// UpdatePassword updates a user's password hash.
func (s *UsersStore) UpdatePassword(ctx context.Context, userID, hash string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdatePassword(ctx, db.UpdatePasswordParams{PasswordHash: &hash, ID: userID})
}

// Delete deletes a user.
func (s *UsersStore) Delete(ctx context.Context, id string) error {
	d := s.db.DB(ctx)
	return d.Queries.DeleteUser(ctx, id)
}

// UpdatePreferences updates user preferences (theme_preference, color_theme, ui_preferences).
func (s *UsersStore) UpdatePreferences(ctx context.Context, userID string, themePref, colorTheme *string, uiPrefs []byte) error {
	d := s.db.DB(ctx)
	arg := db.UpdateUserPreferencesParams{
		ID:              userID,
		ThemePreference: themePref,
		ColorTheme:      colorTheme,
		UiPreferences:   uiPrefs,
	}
	return d.Queries.UpdateUserPreferences(ctx, arg)
}

// ListForAssignment returns active users for assignment dropdowns.
func (s *UsersStore) ListForAssignment(ctx context.Context) ([]models.User, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.ListActiveUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]models.User, len(rows))
	for i := range rows {
		out[i] = dbUserToModel(rows[i])
	}
	return out, nil
}

// UpdateTfaSecret sets the TFA secret (during setup, before enabled).
func (s *UsersStore) UpdateTfaSecret(ctx context.Context, userID string, secret *string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateTfaSecret(ctx, db.UpdateTfaSecretParams{ID: userID, TfaSecret: secret})
}

// UpdateTfaEnabled enables TFA and sets backup codes.
func (s *UsersStore) UpdateTfaEnabled(ctx context.Context, userID string, backupCodesJSON *string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateTfaEnabled(ctx, db.UpdateTfaEnabledParams{
		ID:             userID,
		TfaEnabled:     true,
		TfaBackupCodes: backupCodesJSON,
	})
}

// UpdateTfaBackupCodes replaces backup codes (e.g. after regeneration or one-time use).
func (s *UsersStore) UpdateTfaBackupCodes(ctx context.Context, userID string, backupCodesJSON *string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateTfaBackupCodes(ctx, db.UpdateTfaBackupCodesParams{
		ID:             userID,
		TfaBackupCodes: backupCodesJSON,
	})
}

// DisableTfa turns off TFA and clears secret and backup codes.
func (s *UsersStore) DisableTfa(ctx context.Context, userID string) error {
	d := s.db.DB(ctx)
	return d.Queries.DisableTfa(ctx, userID)
}

// GetByDiscordID returns a user by Discord ID.
func (s *UsersStore) GetByDiscordID(ctx context.Context, discordID string) (*models.User, error) {
	d := s.db.DB(ctx)
	u, err := d.Queries.GetUserByDiscordID(ctx, &discordID)
	if err != nil {
		return nil, err
	}
	out := dbUserToModel(u)
	return &out, nil
}

// GetByDiscordIDOrEmail returns a user by Discord ID or email (prefers discord_id match).
func (s *UsersStore) GetByDiscordIDOrEmail(ctx context.Context, discordID, email string) (*models.User, error) {
	d := s.db.DB(ctx)
	u, err := d.Queries.GetUserByDiscordIDOrEmail(ctx, db.GetUserByDiscordIDOrEmailParams{
		DiscordID: &discordID,
		Lower:     email,
	})
	if err != nil {
		return nil, err
	}
	out := dbUserToModel(u)
	return &out, nil
}

// CreateDiscordUser creates a new user from Discord OAuth.
func (s *UsersStore) CreateDiscordUser(ctx context.Context, id, username, email, role string, discordID, discordUsername, discordAvatar *string, discordLinkedAt time.Time) error {
	d := s.db.DB(ctx)
	now := time.Now()
	ts := pgtype.Timestamp{Time: now, Valid: true}
	linkedTs := pgtype.Timestamp{Time: discordLinkedAt, Valid: true}
	arg := db.CreateDiscordUserParams{
		ID:              id,
		Username:        username,
		Email:           email,
		Role:            role,
		CreatedAt:       ts,
		UpdatedAt:       ts,
		FirstName:       nil,
		LastName:        nil,
		DiscordID:       discordID,
		DiscordUsername: discordUsername,
		DiscordAvatar:   discordAvatar,
		DiscordLinkedAt: linkedTs,
	}
	return d.Queries.CreateDiscordUser(ctx, arg)
}

// UpdateDiscordLink links Discord to an existing user.
func (s *UsersStore) UpdateDiscordLink(ctx context.Context, userID, discordID, username string, avatar *string) error {
	d := s.db.DB(ctx)
	now := time.Now()
	ts := pgtype.Timestamp{Time: now, Valid: true}
	return d.Queries.UpdateUserDiscordLink(ctx, db.UpdateUserDiscordLinkParams{
		ID:              userID,
		DiscordID:       &discordID,
		DiscordUsername: &username,
		DiscordAvatar:   avatar,
		DiscordLinkedAt: ts,
	})
}

// UpdateDiscordUnlink removes Discord from a user.
func (s *UsersStore) UpdateDiscordUnlink(ctx context.Context, userID string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateUserDiscordUnlink(ctx, userID)
}

// UpdateDiscordProfile updates last_login and Discord profile fields for a user.
func (s *UsersStore) UpdateDiscordProfile(ctx context.Context, userID string, lastLogin time.Time, username string, avatar *string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateUserDiscordProfile(ctx, db.UpdateUserDiscordProfileParams{
		ID:              userID,
		LastLogin:       pgtype.Timestamp{Time: lastLogin, Valid: true},
		DiscordUsername: &username,
		DiscordAvatar:   avatar,
	})
}
