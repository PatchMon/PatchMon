package store

import "github.com/PatchMon/PatchMon/server-source-code/internal/util"

const MinStoreAppPatchAgentVersion = "2.1.9"

const minStoreAppPatchAgentFloor = MinStoreAppPatchAgentVersion + "-0"

const NotPatchableStoreApp = "store_app"

func AgentPatchesStoreApps(agentVersion *string) bool {
	return agentVersion != nil && *agentVersion != "" && util.CompareVersions(*agentVersion, minStoreAppPatchAgentFloor) >= 0
}

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
