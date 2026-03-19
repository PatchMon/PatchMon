package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/redis/go-redis/v9"
)

var ErrInvalidRDPTicket = errors.New("invalid or expired RDP ticket")

const (
	rdpTicketPrefix = "rdp:ticket:"
	rdpTicketTTL    = 5 * time.Minute
)

// RDPTicketStore stores one-time RDP tickets in Redis.
type RDPTicketStore struct {
	rdb *hostctx.RedisResolver
}

// NewRDPTicketStore creates a new RDP ticket store.
func NewRDPTicketStore(rdb *hostctx.RedisResolver) *RDPTicketStore {
	return &RDPTicketStore{rdb: rdb}
}

// RDPTicketData is stored in Redis for RDP WebSocket auth.
type RDPTicketData struct {
	UserID    string `json:"userId"`
	HostID    string `json:"hostId"`
	SessionID string `json:"sessionId"`
	ProxyPort int    `json:"proxyPort"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	CreatedAt int64  `json:"createdAt"`
}

// CreateTicket generates a one-time ticket for RDP WebSocket auth.
func (s *RDPTicketStore) CreateTicket(ctx context.Context, userID, hostID, sessionID string, proxyPort int, username, password string) (ticket string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	ticket = hex.EncodeToString(b)
	key := hostctx.TenantKey(ctx, rdpTicketPrefix+ticket)

	data := RDPTicketData{
		UserID:    userID,
		HostID:    hostID,
		SessionID: sessionID,
		ProxyPort: proxyPort,
		Username:  username,
		Password:  password,
		CreatedAt: time.Now().UnixMilli(),
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return "", errors.New("rdp ticket: redis not available")
	}
	if err := rdb.Set(ctx, key, raw, rdpTicketTTL).Err(); err != nil {
		return "", err
	}
	return ticket, nil
}

// ConsumeTicket validates and consumes a one-time ticket. Returns ticket data if valid.
// Ticket is deleted on consumption (one-time use).
func (s *RDPTicketStore) ConsumeTicket(ctx context.Context, ticket string) (*RDPTicketData, error) {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return nil, ErrInvalidRDPTicket
	}
	key := hostctx.TenantKey(ctx, rdpTicketPrefix+ticket)
	raw, err := rdb.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrInvalidRDPTicket
		}
		return nil, err
	}
	if err := rdb.Del(ctx, key).Err(); err != nil {
		return nil, err
	}
	var data RDPTicketData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, ErrInvalidRDPTicket
	}
	return &data, nil
}
