package queue

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// autoPatchFireTolerance bounds how late after the scheduled slot the
// dispatcher will still fire. Covers short server downtime around the slot
// without surprising operators with a patch run hours after the window.
const autoPatchFireTolerance = 30 * time.Minute

// AutoPatchDispatchHandler fires patch_all runs for policies whose automated
// schedule (auto_patch_enabled + auto_patch_days + auto_patch_time) is due.
// Registered on a 1-minute cron; each policy fires at most once per slot via
// the auto_patch_last_run_at stamp.
type AutoPatchDispatchHandler struct {
	policies    *store.PatchPoliciesStore
	patchRuns   *store.PatchRunsStore
	hosts       *store.HostsStore
	db          *database.DB
	queueClient *asynq.Client
	log         *slog.Logger
}

// NewAutoPatchDispatchHandler creates the dispatcher.
func NewAutoPatchDispatchHandler(policies *store.PatchPoliciesStore, patchRuns *store.PatchRunsStore, hosts *store.HostsStore, db *database.DB, queueClient *asynq.Client, log *slog.Logger) *AutoPatchDispatchHandler {
	return &AutoPatchDispatchHandler{policies: policies, patchRuns: patchRuns, hosts: hosts, db: db, queueClient: queueClient, log: log}
}

// parseAutoPatchDays parses the csv weekday list ("0,3" → {0,3}; 0=Sunday).
func parseAutoPatchDays(s string) map[int]struct{} {
	out := make(map[int]struct{})
	for _, part := range strings.Split(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil && n >= 0 && n <= 6 {
			out[n] = struct{}{}
		}
	}
	return out
}

// autoPatchDueSlot returns the scheduled instant for today's slot and whether
// the policy is due right now: today is a configured weekday, the slot time
// has passed (within tolerance), and the slot hasn't been fired yet.
func autoPatchDueSlot(p db.PatchPolicy, now time.Time, loc *time.Location) (time.Time, bool) {
	if p.AutoPatchDays == nil || p.AutoPatchTime == nil {
		return time.Time{}, false
	}
	days := parseAutoPatchDays(*p.AutoPatchDays)
	if len(days) == 0 {
		return time.Time{}, false
	}
	hh, mm, _, err := store.ParseHHMM(*p.AutoPatchTime)
	if err != nil {
		return time.Time{}, false
	}
	localNow := now.In(loc)
	if _, ok := days[int(localNow.Weekday())]; !ok {
		return time.Time{}, false
	}
	slot := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), hh, mm, 0, 0, loc)
	if now.Before(slot) || now.After(slot.Add(autoPatchFireTolerance)) {
		return time.Time{}, false
	}
	if p.AutoPatchLastRunAt.Valid && !p.AutoPatchLastRunAt.Time.UTC().Before(slot.UTC()) {
		return time.Time{}, false // already fired this slot
	}
	return slot, true
}

// ProcessTask implements asynq.Handler.
func (h *AutoPatchDispatchHandler) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	policies, err := h.policies.ListAutoPatchEnabled(ctx)
	if err != nil {
		h.log.Error("auto_patch: list policies failed", "error", err)
		return err
	}
	if len(policies) == 0 {
		return nil
	}

	// Org timezone: same resolution chain the manual trigger path uses
	// (TZ/TIMEZONE env, settings row, UTC fallback).
	tz := "UTC"
	if settings, sErr := h.db.Queries.GetFirstSettings(ctx); sErr == nil {
		tz = config.ResolveTimezone(settings.Timezone, nil)
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now()

	var activeHosts map[string]struct{}
	for _, p := range policies {
		slot, due := autoPatchDueSlot(p, now, loc)
		if !due {
			continue
		}
		// Stamp BEFORE enqueueing: a crash mid-dispatch then means a missed
		// slot, not a double fire. An unexpected duplicate patch (+ reboot)
		// is worse than a skipped window the operator can re-run manually.
		if err := h.policies.MarkAutoPatchFired(ctx, p.ID); err != nil {
			h.log.Error("auto_patch: mark fired failed, skipping policy", "policy", p.Name, "error", err)
			continue
		}

		if activeHosts == nil {
			if activeHosts, err = h.patchRuns.HostIDsWithActiveRuns(ctx); err != nil {
				h.log.Warn("auto_patch: active-run lookup failed; proceeding without dedup", "error", err)
				activeHosts = map[string]struct{}{}
			}
		}

		hostIDs, err := h.policies.ListTargetHostIDs(ctx, p.ID)
		if err != nil {
			h.log.Error("auto_patch: target host lookup failed", "policy", p.Name, "error", err)
			continue
		}
		hosts, err := h.hosts.GetByIDs(ctx, hostIDs)
		if err != nil {
			h.log.Error("auto_patch: host load failed", "policy", p.Name, "error", err)
			continue
		}

		queued, skipped := 0, 0
		for i := range hosts {
			host := &hosts[i]
			// Precedence + exclusions: the candidate list is a superset; the
			// host's effective policy must actually be this one (a direct
			// assignment to another policy, or a group exclusion, wins).
			effective, rErr := h.policies.ResolveEffectivePolicy(ctx, host.ID)
			if rErr != nil || effective == nil || effective.ID != p.ID {
				skipped++
				continue
			}
			if strings.Contains(strings.ToLower(host.OSType), "windows") {
				skipped++
				continue
			}
			if _, busy := activeHosts[host.ID]; busy {
				skipped++
				continue
			}

			runID := uuid.New().String()
			snapshot, _ := json.Marshal(map[string]interface{}{
				"name":             p.Name,
				"patch_delay_type": "immediate",
				"auto_patch":       true,
				"auto_reboot":      p.AutoReboot,
				"scheduled_slot":   slot.UTC().Format(time.RFC3339),
			})
			// triggeredByUserID nil = system-initiated; visible in Runs &
			// History without a username, with the snapshot marking it auto.
			if _, cErr := h.patchRuns.CreateRun(ctx, runID, host.ID, "patch-run-"+runID, "patch_all", nil, nil, nil, false, nil, &p.ID, &p.Name, snapshot, nil); cErr != nil {
				h.log.Error("auto_patch: create run failed", "policy", p.Name, "host_id", host.ID, "error", cErr)
				continue
			}
			task, tErr := NewRunPatchTask(RunPatchPayload{
				HostID:           host.ID,
				ApiID:            host.ApiID,
				PatchRunID:       runID,
				PatchType:        "patch_all",
				RebootIfRequired: p.AutoReboot,
			})
			if tErr != nil {
				h.log.Error("auto_patch: task create failed", "policy", p.Name, "host_id", host.ID, "error", tErr)
				continue
			}
			if _, qErr := h.queueClient.Enqueue(task); qErr != nil {
				h.log.Error("auto_patch: enqueue failed", "policy", p.Name, "host_id", host.ID, "error", qErr)
				continue
			}
			activeHosts[host.ID] = struct{}{}
			queued++
		}
		h.log.Info("auto_patch: policy fired", "policy", p.Name, "slot", slot.UTC().Format(time.RFC3339), "queued", queued, "skipped", skipped, "auto_reboot", p.AutoReboot)
	}
	return nil
}
