package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
)

type AutoEnrollmentStore struct {
	db database.DBProvider
}

func NewAutoEnrollmentStore(db database.DBProvider) *AutoEnrollmentStore {
	return &AutoEnrollmentStore{db: db}
}

func (s *AutoEnrollmentStore) List(ctx context.Context) ([]db.ListAutoEnrollmentTokensRow, error) {
	d := s.db.DB(ctx)
	return d.Queries.ListAutoEnrollmentTokens(ctx)
}

func (s *AutoEnrollmentStore) GetByID(ctx context.Context, id string) (db.GetAutoEnrollmentTokenByIDRow, error) {
	d := s.db.DB(ctx)
	return d.Queries.GetAutoEnrollmentTokenByID(ctx, id)
}

func (s *AutoEnrollmentStore) GetRaw(ctx context.Context, id string) (db.AutoEnrollmentToken, error) {
	d := s.db.DB(ctx)
	return d.Queries.GetAutoEnrollmentTokenRaw(ctx, id)
}

func (s *AutoEnrollmentStore) GetByKey(ctx context.Context, tokenKey string) (db.AutoEnrollmentToken, error) {
	d := s.db.DB(ctx)
	return d.Queries.GetAutoEnrollmentTokenByKey(ctx, tokenKey)
}

func (s *AutoEnrollmentStore) UpdateLastUsedAt(ctx context.Context, id string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateAutoEnrollmentTokenLastUsedAt(ctx, id)
}

func (s *AutoEnrollmentStore) IncrementHostsCreated(ctx context.Context, id string) error {
	d := s.db.DB(ctx)
	return d.Queries.IncrementAutoEnrollmentHostsCreated(ctx, id)
}

func (s *AutoEnrollmentStore) Create(ctx context.Context, arg db.CreateAutoEnrollmentTokenParams) error {
	d := s.db.DB(ctx)
	return d.Queries.CreateAutoEnrollmentToken(ctx, arg)
}

func (s *AutoEnrollmentStore) Update(ctx context.Context, arg db.UpdateAutoEnrollmentTokenParams) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateAutoEnrollmentToken(ctx, arg)
}

func (s *AutoEnrollmentStore) Delete(ctx context.Context, id string) error {
	d := s.db.DB(ctx)
	return d.Queries.DeleteAutoEnrollmentToken(ctx, id)
}

// TokenListItem is the JSON response shape for listing tokens, matching the Node.js Prisma response.
type TokenListItem struct {
	ID                 string          `json:"id"`
	TokenName          string          `json:"token_name"`
	TokenKey           string          `json:"token_key"`
	IsActive           bool            `json:"is_active"`
	AllowedIPRanges    []string        `json:"allowed_ip_ranges"`
	MaxHostsPerDay     int32           `json:"max_hosts_per_day"`
	HostsCreatedToday  int32           `json:"hosts_created_today"`
	LastUsedAt         *time.Time      `json:"last_used_at"`
	ExpiresAt          *time.Time      `json:"expires_at"`
	CreatedAt          time.Time       `json:"created_at"`
	DefaultHostGroupID *string         `json:"default_host_group_id"`
	Metadata           json.RawMessage `json:"metadata"`
	Scopes             json.RawMessage `json:"scopes"`
	HostGroups         *HostGroupBrief `json:"host_groups"`
	Users              *UserBrief      `json:"users"`
}

type HostGroupBrief struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type UserBrief struct {
	ID        string  `json:"id"`
	Username  string  `json:"username"`
	FirstName *string `json:"first_name"`
	LastName  *string `json:"last_name"`
}

func RowToTokenListItem(r db.ListAutoEnrollmentTokensRow) TokenListItem {
	item := TokenListItem{
		ID:                 r.ID,
		TokenName:          r.TokenName,
		TokenKey:           r.TokenKey,
		IsActive:           r.IsActive,
		AllowedIPRanges:    r.AllowedIpRanges,
		MaxHostsPerDay:     r.MaxHostsPerDay,
		HostsCreatedToday:  r.HostsCreatedToday,
		LastUsedAt:         pgTimePtr(r.LastUsedAt),
		ExpiresAt:          pgTimePtr(r.ExpiresAt),
		CreatedAt:          pgTime(r.CreatedAt),
		DefaultHostGroupID: r.DefaultHostGroupID,
		Metadata:           nullableJSON(r.Metadata),
		Scopes:             nullableJSON(r.Scopes),
	}
	if item.AllowedIPRanges == nil {
		item.AllowedIPRanges = []string{}
	}
	if r.HgID != nil {
		item.HostGroups = &HostGroupBrief{
			ID:    *r.HgID,
			Name:  deref(r.HgName),
			Color: deref(r.HgColor),
		}
	}
	if r.UID != nil {
		item.Users = &UserBrief{
			ID:        *r.UID,
			Username:  deref(r.UUsername),
			FirstName: r.UFirstName,
			LastName:  r.ULastName,
		}
	}
	return item
}

func IDRowToTokenListItem(r db.GetAutoEnrollmentTokenByIDRow) TokenListItem {
	item := TokenListItem{
		ID:                 r.ID,
		TokenName:          r.TokenName,
		TokenKey:           r.TokenKey,
		IsActive:           r.IsActive,
		AllowedIPRanges:    r.AllowedIpRanges,
		MaxHostsPerDay:     r.MaxHostsPerDay,
		HostsCreatedToday:  r.HostsCreatedToday,
		LastUsedAt:         pgTimePtr(r.LastUsedAt),
		ExpiresAt:          pgTimePtr(r.ExpiresAt),
		CreatedAt:          pgTime(r.CreatedAt),
		DefaultHostGroupID: r.DefaultHostGroupID,
		Metadata:           nullableJSON(r.Metadata),
		Scopes:             nullableJSON(r.Scopes),
	}
	if item.AllowedIPRanges == nil {
		item.AllowedIPRanges = []string{}
	}
	if r.HgID != nil {
		item.HostGroups = &HostGroupBrief{
			ID:    *r.HgID,
			Name:  deref(r.HgName),
			Color: deref(r.HgColor),
		}
	}
	if r.UID != nil {
		item.Users = &UserBrief{
			ID:        *r.UID,
			Username:  deref(r.UUsername),
			FirstName: r.UFirstName,
			LastName:  r.ULastName,
		}
	}
	return item
}

func nullableJSON(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("null")
	}
	return json.RawMessage(b)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
