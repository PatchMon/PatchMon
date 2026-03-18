package store

import (
	"context"
	"encoding/json"
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
	key := integrationStatusKeyPrefix + apiID + ":" + integrationName
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
	key := integrationStatusKeyPrefix + apiID + ":" + integrationName
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
	key := complianceInstallJobPrefix + hostID
	return rdb.Set(ctx, key, jobID, time.Duration(complianceInstallJobTTL)*time.Second).Err()
}

// GetComplianceInstallJob returns the install job ID for a host.
func (s *IntegrationStatusStore) GetComplianceInstallJob(ctx context.Context, hostID string) (string, error) {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return "", nil
	}
	key := complianceInstallJobPrefix + hostID
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
	key := complianceInstallCancelPrefix + jobID
	return rdb.Set(ctx, key, "1", 5*time.Minute).Err()
}

// SetComplianceScanCancel sets the cancel flag for a host's scan (used when user cancels).
func (s *IntegrationStatusStore) SetComplianceScanCancel(ctx context.Context, hostID string) error {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return nil
	}
	key := complianceScanCancelPrefix + hostID
	return rdb.Set(ctx, key, "1", time.Duration(complianceScanCancelTTL)*time.Second).Err()
}

// ClearComplianceScanCancel clears the cancel flag (call when triggering a new scan).
func (s *IntegrationStatusStore) ClearComplianceScanCancel(ctx context.Context, hostID string) error {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return nil
	}
	return rdb.Del(ctx, complianceScanCancelPrefix+hostID).Err()
}

// IsComplianceScanCancelled returns true if the user has cancelled the scan for this host.
func (s *IntegrationStatusStore) IsComplianceScanCancelled(ctx context.Context, hostID string) bool {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return false
	}
	_, err := rdb.Get(ctx, complianceScanCancelPrefix+hostID).Result()
	return err != redis.Nil
}

// SetSSGUpgradeJob stores the SSG upgrade job ID for a host.
func (s *IntegrationStatusStore) SetSSGUpgradeJob(ctx context.Context, hostID, jobID string) error {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return nil
	}
	key := ssgUpgradeJobPrefix + hostID
	return rdb.Set(ctx, key, jobID, time.Duration(ssgUpgradeJobTTL)*time.Second).Err()
}

// GetSSGUpgradeJob returns the SSG upgrade job ID for a host.
func (s *IntegrationStatusStore) GetSSGUpgradeJob(ctx context.Context, hostID string) (string, error) {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return "", nil
	}
	key := ssgUpgradeJobPrefix + hostID
	val, err := rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}
