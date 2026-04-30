package queue

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strconv"
	"strings"

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

	opts := asynq.RedisClientOpt{
		Addr:     fmt.Sprintf("%s:%d", host, port),
		Password: password,
		DB:       db,
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

	return opts
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
