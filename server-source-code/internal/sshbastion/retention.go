package sshbastion

import (
	"context"
	"log/slog"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/sessionrecording"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

func StartRetention(ctx context.Context, sshStore *store.SSHStore, recordings *sessionrecording.Store, resolve ContextResolver, tenants func() []string, retentionDays int, log *slog.Logger) {
	if retentionDays < 1 {
		return
	}
	run := func() {
		allTenants := []string{""}
		if tenants != nil {
			allTenants = append(allTenants, tenants()...)
		}
		before := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
		for _, tenant := range allTenants {
			tenantCtx, err := resolve(tenant)
			if err != nil {
				log.Warn("SSH recording purge skipped tenant", "tenant", tenant, "error", err)
				continue
			}
			for {
				expired, err := sshStore.ExpiredRecordings(tenantCtx, before, 100)
				if err != nil {
					log.Warn("SSH recording purge query failed", "tenant", tenant, "error", err)
					break
				}
				if len(expired) == 0 {
					break
				}
				for _, session := range expired {
					if err := recordings.Delete(TenantStorageID(tenant), session.ID); err != nil {
						log.Warn("SSH recording purge failed", "tenant", tenant, "session_id", session.ID, "error", err)
						continue
					}
					if err := sshStore.MarkRecordingDeleted(tenantCtx, session.ID); err != nil {
						log.Warn("SSH recording purge metadata update failed", "tenant", tenant, "session_id", session.ID, "error", err)
						continue
					}
					log.Info("SSH recording purged", "tenant", tenant, "session_id", session.ID, "retention_days", retentionDays)
				}
				if len(expired) < 100 {
					break
				}
			}
		}
	}
	go func() {
		run()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}
