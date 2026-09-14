package queue

import "testing"

// The asynq options are built separately from internal/redis.NewClient, so the
// two can drift. #745: REDIS_USER was honoured by the Redis client but dropped
// from the asynq options, leaving every background task failing WRONGPASS on a
// non-default ACL user while the rest of the server connected fine.
func TestRedisOpts_CarriesEveryCredentialFromEnv(t *testing.T) {
	t.Setenv("REDIS_HOST", "redis.internal")
	t.Setenv("REDIS_PORT", "6380")
	t.Setenv("REDIS_DB", "3")
	t.Setenv("REDIS_USER", "patchmon")
	t.Setenv("REDIS_PASSWORD", "s3cret")

	opts := RedisOpts()

	if opts.Addr != "redis.internal:6380" {
		t.Errorf("Addr = %q, want redis.internal:6380", opts.Addr)
	}
	if opts.Username != "patchmon" {
		t.Errorf("Username = %q, want patchmon", opts.Username)
	}
	if opts.Password != "s3cret" {
		t.Errorf("Password = %q, want s3cret", opts.Password)
	}
	if opts.DB != 3 {
		t.Errorf("DB = %d, want 3", opts.DB)
	}
}

func TestRedisOpts_UnsetUserStaysEmpty(t *testing.T) {
	t.Setenv("REDIS_HOST", "")
	t.Setenv("REDIS_USER", "")
	t.Setenv("REDIS_PASSWORD", "")

	opts := RedisOpts()

	if opts.Username != "" {
		t.Errorf("Username = %q, want empty so Redis applies the default ACL user", opts.Username)
	}
	if opts.Addr != "localhost:6379" {
		t.Errorf("Addr = %q, want localhost:6379", opts.Addr)
	}
}
