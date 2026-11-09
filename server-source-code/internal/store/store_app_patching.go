package store

import "github.com/PatchMon/PatchMon/server-source-code/internal/util"

// MinStoreAppPatchAgentVersion is the oldest agent that updates Microsoft Store apps in a patch run.
const MinStoreAppPatchAgentVersion = "2.1.9"

const minStoreAppPatchAgentFloor = MinStoreAppPatchAgentVersion + "-0"

// NotPatchableStoreApp marks a Store app on a host whose agent cannot update it.
const NotPatchableStoreApp = "store_app"

// AgentPatchesStoreApps reports whether an agent of this version updates Store apps.
func AgentPatchesStoreApps(agentVersion *string) bool {
	return agentVersion != nil && *agentVersion != "" && util.CompareVersions(*agentVersion, minStoreAppPatchAgentFloor) >= 0
}

// StoreAppPatchability lifts the Store app block for a host whose agent can update Store apps.
func StoreAppPatchability(patchable bool, reason, patchSource string, agentVersion *string) (bool, string) {
	if reason != NotPatchableStoreApp || !AgentPatchesStoreApps(agentVersion) {
		return patchable, reason
	}
	if patchSource != "winget" {
		return false, "no_winget_id"
	}
	return true, ""
}

func (p *Patchability) applyStoreAppGate(agentVersion *string) {
	p.Patchable, p.NotPatchableReason = StoreAppPatchability(p.Patchable, p.NotPatchableReason, p.PatchSource, agentVersion)
}
