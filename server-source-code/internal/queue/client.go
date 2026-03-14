package queue

import (
	"fmt"
	"os"
	"strconv"

	"github.com/hibiken/asynq"
)

const (
	defaultRedisHost = "localhost"
	defaultRedisPort = 6379
	defaultRedisDB   = 0
)

// RedisOpts returns Asynq Redis options from environment.
func RedisOpts() asynq.RedisClientOpt {
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = defaultRedisHost
	}
	port := getEnvInt("REDIS_PORT", defaultRedisPort)
	db := getEnvInt("REDIS_DB", defaultRedisDB)
	password := os.Getenv("REDIS_PASSWORD")

	return asynq.RedisClientOpt{
		Addr:     fmt.Sprintf("%s:%d", host, port),
		Password: password,
		DB:       db,
	}
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

// NewClient creates an Asynq client for enqueueing jobs.
func NewClient(opts asynq.RedisClientOpt) *asynq.Client {
	return asynq.NewClient(opts)
}
