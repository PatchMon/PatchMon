package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/redis/go-redis/v9"
)

const integrationStatusKeyPrefix = "integration_status:"

// IntegrationStatusStore reads/writes integration status in Redis.
// Key format: integration_status:{api_id}:{integration_name}
type IntegrationStatusStore struct {
	rdb *hostctx.RedisResolver
}

// NewIntegrationStatusStore creates a new integration status store.
func NewIntegrationStatusStore(rdb *hostctx.RedisResolver) *IntegrationStatusStore {
	return &IntegrationStatusStore{rdb: rdb}
}

// Get returns the integration status for a host. Returns nil if not found.
func (s *IntegrationStatusStore) Get(ctx context.Context, apiID, integrationName string) (map[string]interface{}, error) {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return nil, nil
	}
	key := hostctx.TenantKey(ctx, integrationStatusKeyPrefix+apiID+":"+integrationName)
	val, err := rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(val), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Set stores integration status in Redis.
func (s *IntegrationStatusStore) Set(ctx context.Context, apiID, integrationName string, status map[string]interface{}) error {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return nil
	}
	b, err := json.Marshal(status)
	if err != nil {
		return err
	}
	key := hostctx.TenantKey(ctx, integrationStatusKeyPrefix+apiID+":"+integrationName)
	return rdb.Set(ctx, key, b, 0).Err()
}

const (
	complianceInstallJobPrefix    = "compliance_install_job:"
	complianceInstallCancelPrefix = "compliance_install_cancel:"
	complianceScanCancelPrefix    = "compliance_scan_cancel:"
	ssgUpgradeJobPrefix           = "ssg_upgrade_job:"
	complianceInstallJobTTL       = 3600 // 1 hour
	complianceScanCancelTTL       = 3600 // 1 hour - cancel flag expires so new scans aren't blocked
	ssgUpgradeJobTTL              = 3600 // 1 hour
)

// SetComplianceInstallJob stores the install job ID for a host.
func (s *IntegrationStatusStore) SetComplianceInstallJob(ctx context.Context, hostID, jobID string) error {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return nil
	}
	key := hostctx.TenantKey(ctx, complianceInstallJobPrefix+hostID)
	return rdb.Set(ctx, key, jobID, time.Duration(complianceInstallJobTTL)*time.Second).Err()
}

// ClearInstallEvents drops the stored install_events for a host's integration,
// leaving the rest of the status blob alone.
//
// Called when a new install is queued. Without it the previous install's
// terminal event survives in Redis, and the first poll of the new job reports
// that older run's outcome as if it were this one's. The blob is not cleared
// wholesale because the agent's periodic availability report legitimately owns
// the other fields.
func (s *IntegrationStatusStore) ClearInstallEvents(ctx context.Context, apiID, integrationName string) error {
	return s.update(ctx, apiID, integrationName, func(current map[string]interface{}) (map[string]interface{}, bool) {
		if current == nil {
			return nil, false
		}
		if _, ok := current["install_events"]; !ok {
			return nil, false
		}
		current["install_events"] = []interface{}{}
		delete(current, "install_job_id")
		return current, true
	})
}

// SetKeepingInstallEvents stores status, carrying over the stored
// install_events when status has none, and returns what was stored.
func (s *IntegrationStatusStore) SetKeepingInstallEvents(ctx context.Context, apiID, integrationName string, status map[string]interface{}) (map[string]interface{}, error) {
	if _, ok := status["install_events"]; ok {
		return status, s.Set(ctx, apiID, integrationName, status)
	}
	stored := status
	err := s.update(ctx, apiID, integrationName, func(current map[string]interface{}) (map[string]interface{}, bool) {
		next := make(map[string]interface{}, len(status)+1)
		for k, v := range status {
			next[k] = v
		}
		next["install_events"] = []interface{}{}
		if prev, ok := current["install_events"]; ok && prev != nil {
			next["install_events"] = prev
		}
		if prev, ok := current["install_job_id"]; ok {
			next["install_job_id"] = prev
		}
		stored = next
		return next, true
	})
	return stored, err
}

const integrationStatusUpdateAttempts = 10

// update applies mutate under WATCH so a concurrent writer can't be overwritten
// with stale fields.
func (s *IntegrationStatusStore) update(ctx context.Context, apiID, integrationName string, mutate func(current map[string]interface{}) (map[string]interface{}, bool)) error {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return nil
	}
	key := hostctx.TenantKey(ctx, integrationStatusKeyPrefix+apiID+":"+integrationName)
	txf := func(tx *redis.Tx) error {
		var current map[string]interface{}
		val, err := tx.Get(ctx, key).Result()
		switch {
		case err == redis.Nil:
		case err != nil:
			return err
		default:
			if err := json.Unmarshal([]byte(val), &current); err != nil {
				return err
			}
		}
		next, write := mutate(current)
		if !write {
			return nil
		}
		b, err := json.Marshal(next)
		if err != nil {
			return err
		}
		_, err = tx.TxPipelined(ctx, func(p redis.Pipeliner) error {
			p.Set(ctx, key, b, 0)
			return nil
		})
		return err
	}
	var err error
	for range integrationStatusUpdateAttempts {
		err = rdb.Watch(ctx, txf, key)
		if !errors.Is(err, redis.TxFailedErr) {
			return err
		}
	}
	return err
}

// ClearComplianceInstallJob forgets a host's install job, so a cancelled
// install neither blocks a new one nor receives its outcome.
func (s *IntegrationStatusStore) ClearComplianceInstallJob(ctx context.Context, hostID string) error {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return nil
	}
	return rdb.Del(ctx, hostctx.TenantKey(ctx, complianceInstallJobPrefix+hostID)).Err()
}

// GetComplianceInstallJob returns the install job ID for a host.
func (s *IntegrationStatusStore) GetComplianceInstallJob(ctx context.Context, hostID string) (string, error) {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return "", nil
	}
	key := hostctx.TenantKey(ctx, complianceInstallJobPrefix+hostID)
	val, err := rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// SetComplianceInstallCancel sets the cancel flag for an install job.
func (s *IntegrationStatusStore) SetComplianceInstallCancel(ctx context.Context, jobID string) error {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return nil
	}
	key := hostctx.TenantKey(ctx, complianceInstallCancelPrefix+jobID)
	return rdb.Set(ctx, key, "1", 5*time.Minute).Err()
}

// SetComplianceScanCancel sets the cancel flag for a host's scan (used when user cancels).
func (s *IntegrationStatusStore) SetComplianceScanCancel(ctx context.Context, hostID string) error {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return nil
	}
	key := hostctx.TenantKey(ctx, complianceScanCancelPrefix+hostID)
	return rdb.Set(ctx, key, "1", time.Duration(complianceScanCancelTTL)*time.Second).Err()
}

// ClearComplianceScanCancel clears the cancel flag (call when triggering a new scan).
func (s *IntegrationStatusStore) ClearComplianceScanCancel(ctx context.Context, hostID string) error {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return nil
	}
	return rdb.Del(ctx, hostctx.TenantKey(ctx, complianceScanCancelPrefix+hostID)).Err()
}

// IsComplianceScanCancelled returns true if the user has cancelled the scan for this host.
func (s *IntegrationStatusStore) IsComplianceScanCancelled(ctx context.Context, hostID string) bool {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return false
	}
	_, err := rdb.Get(ctx, hostctx.TenantKey(ctx, complianceScanCancelPrefix+hostID)).Result()
	return err != redis.Nil
}

// SetSSGUpgradeJob stores the SSG upgrade job ID for a host.
func (s *IntegrationStatusStore) SetSSGUpgradeJob(ctx context.Context, hostID, jobID string) error {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return nil
	}
	key := hostctx.TenantKey(ctx, ssgUpgradeJobPrefix+hostID)
	return rdb.Set(ctx, key, jobID, time.Duration(ssgUpgradeJobTTL)*time.Second).Err()
}

// GetSSGUpgradeJob returns the SSG upgrade job ID for a host.
func (s *IntegrationStatusStore) GetSSGUpgradeJob(ctx context.Context, hostID string) (string, error) {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return "", nil
	}
	key := hostctx.TenantKey(ctx, ssgUpgradeJobPrefix+hostID)
	val, err := rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}
