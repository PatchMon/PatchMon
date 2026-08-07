package store

import (
	"context"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
)

// UserApiTokenStore manages long-lived user API tokens for automation.
type UserApiTokenStore struct {
	db database.DBProvider
}

func NewUserApiTokenStore(db database.DBProvider) *UserApiTokenStore {
	return &UserApiTokenStore{db: db}
}

func (s *UserApiTokenStore) List(ctx context.Context, userID string) ([]db.ListUserApiTokensRow, error) {
	d := s.db.DB(ctx)
	return d.Queries.ListUserApiTokens(ctx, userID)
}

func (s *UserApiTokenStore) GetByHash(ctx context.Context, tokenHash string) (db.GetUserApiTokenByHashRow, error) {
	d := s.db.DB(ctx)
	return d.Queries.GetUserApiTokenByHash(ctx, tokenHash)
}

func (s *UserApiTokenStore) Create(ctx context.Context, arg db.CreateUserApiTokenParams) (db.ListUserApiTokensRow, error) {
	d := s.db.DB(ctx)
	return d.Queries.CreateUserApiToken(ctx, arg)
}

func (s *UserApiTokenStore) Delete(ctx context.Context, id string) error {
	d := s.db.DB(ctx)
	return d.Queries.DeleteUserApiToken(ctx, id)
}

func (s *UserApiTokenStore) UpdateLastUsed(ctx context.Context, id string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateUserApiTokenLastUsed(ctx, id)
}

// UserApiTokenListItem is the safe JSON response for listing tokens (never exposes hash).
type UserApiTokenListItem struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

func RowToUserApiTokenListItem(r db.ListUserApiTokensRow) UserApiTokenListItem {
	return UserApiTokenListItem{
		ID:         r.ID,
		Name:       r.Name,
		CreatedAt:  r.CreatedAt,
		ExpiresAt:  r.ExpiresAt,
		LastUsedAt: r.LastUsedAt,
	}
}
