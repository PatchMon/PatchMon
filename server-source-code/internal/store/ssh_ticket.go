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

var ErrInvalidTicket = errors.New("invalid or expired ticket")

const (
	sshTicketPrefix = "ssh:ticket:"
	sshTicketTTL    = 30 * time.Second
)

// SshTicketStore stores one-time SSH terminal tickets in Redis.
type SshTicketStore struct {
	rdb *hostctx.RedisResolver
}

// NewSshTicketStore creates a new SSH ticket store.
func NewSshTicketStore(rdb *hostctx.RedisResolver) *SshTicketStore {
	return &SshTicketStore{rdb: rdb}
}

// TicketData is stored in Redis for SSH terminal auth.
type TicketData struct {
	UserID    string `json:"userId"`
	SessionID string `json:"sessionId"`
	HostID    string `json:"hostId"`
	CreatedAt int64  `json:"createdAt"`
}

// CreateTicket generates a one-time ticket for SSH terminal WebSocket auth.
func (s *SshTicketStore) CreateTicket(ctx context.Context, userID, hostID string) (ticket string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	ticket = hex.EncodeToString(b)
	key := sshTicketPrefix + ticket

	data := TicketData{
		UserID:    userID,
		SessionID: "",
		HostID:    hostID,
		CreatedAt: time.Now().UnixMilli(),
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return "", errors.New("ssh ticket: redis not available")
	}
	if err := rdb.Set(ctx, key, raw, sshTicketTTL).Err(); err != nil {
		return "", err
	}
	return ticket, nil
}

// ConsumeTicket validates and consumes a one-time ticket. Returns userID if valid.
// Ticket is deleted on consumption (one-time use).
func (s *SshTicketStore) ConsumeTicket(ctx context.Context, ticket, expectedHostID string) (userID string, err error) {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return "", ErrInvalidTicket
	}
	key := sshTicketPrefix + ticket
	raw, err := rdb.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrInvalidTicket
		}
		return "", err
	}
	if err := rdb.Del(ctx, key).Err(); err != nil {
		return "", err
	}
	var data TicketData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return "", err
	}
	if data.HostID != expectedHostID {
		return "", errors.New("ticket host mismatch")
	}
	return data.UserID, nil
}
