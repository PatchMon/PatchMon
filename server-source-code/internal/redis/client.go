package redis

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultHost   = "localhost"
	defaultPort   = 6379
	defaultDB     = 0
	defaultConnTO = 60 * time.Second
	defaultCmdTO  = 60 * time.Second
)

// NewClient creates a Redis client from environment variables.
// Env: REDIS_HOST, REDIS_PORT, REDIS_PASSWORD, REDIS_USER, REDIS_DB,
// REDIS_TLS, REDIS_CONNECT_TIMEOUT_MS, REDIS_COMMAND_TIMEOUT_MS.
func NewClient() *redis.Client {
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = defaultHost
	}
	port := getEnvInt("REDIS_PORT", defaultPort)
	password := os.Getenv("REDIS_PASSWORD")
	username := os.Getenv("REDIS_USER")
	db := getEnvInt("REDIS_DB", defaultDB)
	connTO := time.Duration(getEnvInt("REDIS_CONNECT_TIMEOUT_MS", int(defaultConnTO.Milliseconds()))) * time.Millisecond
	cmdTO := time.Duration(getEnvInt("REDIS_COMMAND_TIMEOUT_MS", int(defaultCmdTO.Milliseconds()))) * time.Millisecond

	opts := &redis.Options{
		Addr:         fmt.Sprintf("%s:%d", host, port),
		Password:     password,
		Username:     username,
		DB:           db,
		DialTimeout:  connTO,
		ReadTimeout:  cmdTO,
		WriteTimeout: cmdTO,
	}

	if os.Getenv("REDIS_TLS") == "true" {
		tlsCfg := &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: os.Getenv("REDIS_TLS_VERIFY") == "false",
		}
		if ca := os.Getenv("REDIS_TLS_CA"); ca != "" {
			pool := x509.NewCertPool()
			var pem []byte
			trimmed := strings.TrimSpace(ca)
			if len(trimmed) >= 5 && trimmed[:5] == "-----" {
				pem = []byte(ca)
			} else {
				// Assume file path
				var err error
				pem, err = os.ReadFile(ca)
				if err != nil {
					pem = []byte(ca)
				}
			}
			if len(pem) > 0 && pool.AppendCertsFromPEM(pem) {
				tlsCfg.RootCAs = pool
			}
		}
		opts.TLSConfig = tlsCfg
	}

	return redis.NewClient(opts)
}

// Ping verifies the Redis connection.
func Ping(ctx context.Context, client *redis.Client) error {
	return client.Ping(ctx).Err()
}

// Close closes the Redis client.
func Close(client *redis.Client) error {
	return client.Close()
}

func getEnvInt(key string, defaultVal int) int {
	s := os.Getenv(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}
