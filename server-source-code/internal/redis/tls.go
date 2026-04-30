package redis

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"strings"
)

// TLSConfigFromEnv returns a *tls.Config built from REDIS_TLS / REDIS_TLS_VERIFY
// / REDIS_TLS_CA, or nil when REDIS_TLS != "true".
//
// Used by both the cache client (NewClient) and the asynq queue client
// (queue.RedisOpts) so TLS handling stays consistent across Redis call sites.
func TLSConfigFromEnv() *tls.Config {
	if os.Getenv("REDIS_TLS") != "true" {
		return nil
	}

	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: os.Getenv("REDIS_TLS_VERIFY") == "false",
	}

	if ca := strings.TrimSpace(os.Getenv("REDIS_TLS_CA")); ca != "" {
		pool := x509.NewCertPool()
		var pem []byte
		if strings.HasPrefix(ca, "-----") {
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

	return tlsCfg
}
