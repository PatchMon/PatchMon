import { compareVersions } from "./version";

export const CONFIG_VERSION_MIN_AGENT = "2.1.4";

export const CONFIG_VERSION_STATE = {
	MATCH: "match",
	MISMATCH: "mismatch",
	NOT_REPORTED: "not_reported",
	REQUIRES_UPGRADE: "requires_upgrade",
	UNKNOWN: "unknown",
};

const normalise = (v) =>
	String(v ?? "")
		.trim()
		.replace(/^v/i, "");

export const supportsConfigVersion = (agentVersion) => {
	const v = normalise(agentVersion);
	if (!v) return false;
	return compareVersions(v, `${CONFIG_VERSION_MIN_AGENT}-0`) >= 0;
};

export const getConfigVersionState = (agentVersion, configVersion) => {
	const agent = normalise(agentVersion);
	const config = normalise(configVersion);
	if (!agent) return CONFIG_VERSION_STATE.UNKNOWN;
	if (!config) {
		return supportsConfigVersion(agent)
			? CONFIG_VERSION_STATE.NOT_REPORTED
			: CONFIG_VERSION_STATE.REQUIRES_UPGRADE;
	}
	return agent === config
		? CONFIG_VERSION_STATE.MATCH
		: CONFIG_VERSION_STATE.MISMATCH;
};

export const hasConfigVersionMismatch = (host) =>
	getConfigVersionState(host?.agent_version, host?.config_version) ===
	CONFIG_VERSION_STATE.MISMATCH;

export const hasHostIdMismatch = (host) => {
	if (!host) return false;
	if (host.host_id_mismatch === true) return true;
	const reported = String(host.reported_host_id ?? "").trim();
	return !!reported && !!host.id && reported !== host.id;
};

export const hasIdentityConflict = (host) => !!host?.identity_conflict_at;

export const CONFIG_VERSION_COPY = {
	mismatch: (agent, config) =>
		`config.yml was last written by agent ${config}, but the agent is now ${agent}. The agent has not rewritten config.yml since it was upgraded. It does so on its next start, so restart the agent service if this persists.`,
	notReported:
		"This agent is new enough to report a config version but has not sent one yet. It is reported with the next check-in.",
	requiresUpgrade: `Agents older than ${CONFIG_VERSION_MIN_AGENT} do not report a config version. Update the agent to see it.`,
	hostIdMismatch:
		"The host_id in this agent's config.yml belongs to a different host from the one its credentials authenticate as. Usually config.yml was copied from another machine or edited by hand. Run \"patchmon-agent config sync-host-id\" on the host, or reinstall the agent with the install command from this host's page.",
	identityConflict:
		"Two machines appear to be reporting with this host's credentials (possible clone), so this page alternates between their data. Regenerate the credentials and reinstall the agent on the machine that should keep this host; enrol the other machine as a new host.",
};
