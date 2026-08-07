package redis

import (
	"context"
	"fmt"
	"os"
	"strconv"
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
		TLSConfig:    TLSConfigFromEnv(),
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
