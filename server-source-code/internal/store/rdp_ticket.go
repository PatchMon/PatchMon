package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
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
	enc *util.Encryption
}

// NewRDPTicketStore creates a new RDP ticket store.
// enc encrypts ticket data at rest in Redis; if nil, storage is rejected.
func NewRDPTicketStore(rdb *hostctx.RedisResolver, enc *util.Encryption) *RDPTicketStore {
	return &RDPTicketStore{rdb: rdb, enc: enc}
}

// RDPTicketData is stored in Redis for RDP WebSocket auth.
type RDPTicketData struct {
	UserID    string `json:"userId"`
	HostID    string `json:"hostId"`
	SessionID string `json:"sessionId"`
	ProxyPort int    `json:"proxyPort"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	CreatedAt int64  `json:"createdAt"`
}

// CreateTicket generates a one-time ticket for RDP WebSocket auth.
func (s *RDPTicketStore) CreateTicket(ctx context.Context, userID, hostID, sessionID string, proxyPort int, username, password string, width, height int) (ticket string, err error) {
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
		Width:     width,
		Height:    height,
		CreatedAt: time.Now().UnixMilli(),
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	if s.enc == nil {
		return "", errors.New("rdp ticket: encryption not available")
	}
	encrypted, err := s.enc.Encrypt(string(raw))
	if err != nil {
		return "", err
	}
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return "", errors.New("rdp ticket: redis not available")
	}
	if err := rdb.Set(ctx, key, encrypted, rdpTicketTTL).Err(); err != nil {
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
	encrypted, err := rdb.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrInvalidRDPTicket
		}
		return nil, err
	}
	if err := rdb.Del(ctx, key).Err(); err != nil {
		return nil, err
	}
	if s.enc == nil {
		return nil, errors.New("rdp ticket: encryption not available")
	}
	raw, err := s.enc.Decrypt(encrypted)
	if err != nil {
		return nil, ErrInvalidRDPTicket
	}
	var data RDPTicketData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, ErrInvalidRDPTicket
	}
	return &data, nil
}
