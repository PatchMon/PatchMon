package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
)

const identityFlapWindow = 24 * time.Hour

func identityFingerprint(machineID, hostnameHash string) string {
	machineID = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(machineID), "-", ""))
	hostnameHash = strings.TrimSpace(hostnameHash)
	if machineID == "" || hostnameHash == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(machineID + "|" + hostnameHash))
	return hex.EncodeToString(sum[:])
}

// newlyFlagged is true only on the first flap, so a persisting clone logs once.
func nextIdentityFingerprint(cur store.IdentityFingerprintState, f string, now time.Time) (update *store.IdentityFingerprintUpdate, newlyFlagged bool) {
	if f == "" || (cur.Fingerprint != nil && *cur.Fingerprint == f) {
		return nil, false
	}
	update = &store.IdentityFingerprintUpdate{
		Fingerprint:     f,
		PrevFingerprint: cur.Fingerprint,
		ChangedAt:       now,
	}
	flapping := cur.PrevFingerprint != nil && *cur.PrevFingerprint == f &&
		cur.ChangedAt != nil && now.Sub(*cur.ChangedAt) < identityFlapWindow
	if flapping {
		update.ConflictAt = &now
		return update, cur.ConflictAt == nil
	}
	return update, false
}

func claimedHostID(id string) string {
	if util.ValidHostID(id) {
		return id
	}
	return ""
}

func (h *InstallHandler) observeAgentIdentity(ctx context.Context, hostID string, storedReported *string, claimed string, state store.IdentityFingerprintState, machineID, hostnameHash string) bool {
	mismatch := claimed != "" && claimed != hostID
	if mismatch && (storedReported == nil || *storedReported != claimed) {
		h.log.Warn("agent reported a different host_id", "host_id", hostID, "reported_host_id", claimed)
	}
	if len(machineID) > 255 {
		return mismatch
	}
	update, flagged := nextIdentityFingerprint(state, identityFingerprint(machineID, hostnameHash), time.Now().UTC())
	if update == nil {
		return mismatch
	}
	if err := h.hosts.UpdateIdentityFingerprint(ctx, hostID, *update); err != nil {
		h.log.Warn("failed to record host identity fingerprint", "host_id", hostID, "error", err)
		return mismatch
	}
	if flagged {
		h.log.Warn("possible cloned agent: two machines are reporting with this host's credentials", "host_id", hostID)
	}
	return mismatch
}
