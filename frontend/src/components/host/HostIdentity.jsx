import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CopyX, Key, ShieldAlert, X } from "lucide-react";
import { useToast } from "../../contexts/ToastContext";
import { adminHostsAPI, formatDate } from "../../utils/api";
import {
	CONFIG_VERSION_COPY,
	CONFIG_VERSION_MIN_AGENT,
	CONFIG_VERSION_STATE,
	getConfigVersionState,
	hasHostIdMismatch,
	hasIdentityConflict,
} from "../../utils/hostIdentity";
import InfoTooltip from "../ui/InfoTooltip";
import Tooltip from "../ui/Tooltip";

export const ConfigVersionValue = ({ host, emptyLabel = "Unknown" }) => {
	const state = getConfigVersionState(
		host?.agent_version,
		host?.config_version,
	);
	const value = host?.config_version;

	if (state === CONFIG_VERSION_STATE.MATCH) {
		return <span>{value}</span>;
	}
	if (state === CONFIG_VERSION_STATE.MISMATCH) {
		return (
			<span className="inline-flex items-center gap-1.5 flex-wrap">
				<span>{value}</span>
				<span className="badge-warning">Out of date</span>
				<InfoTooltip
					content={CONFIG_VERSION_COPY.mismatch(host.agent_version, value)}
					label="About config version out of date"
				/>
			</span>
		);
	}
	if (state === CONFIG_VERSION_STATE.NOT_REPORTED) {
		return (
			<span className="inline-flex items-center gap-1.5">
				<span className="badge-warning">Not reported</span>
				<InfoTooltip
					content={CONFIG_VERSION_COPY.notReported}
					label="About config version not reported"
				/>
			</span>
		);
	}
	if (state === CONFIG_VERSION_STATE.REQUIRES_UPGRADE) {
		return (
			<span className="inline-flex items-center gap-1.5">
				<span className="text-secondary-500 dark:text-secondary-400 font-normal">
					Requires agent {CONFIG_VERSION_MIN_AGENT}
				</span>
				<InfoTooltip
					content={CONFIG_VERSION_COPY.requiresUpgrade}
					label="About config version on older agents"
				/>
			</span>
		);
	}
	return <span>{value || emptyLabel}</span>;
};

const indicatorButton = (label, icon) => (
	<button
		type="button"
		aria-label={label}
		className="relative inline-flex flex-shrink-0 items-center justify-center rounded p-0.5 align-middle cursor-help focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 before:absolute before:-inset-3 md:before:-inset-1"
	>
		{icon}
	</button>
);

export const AgentVersionIndicators = ({ host }) => {
	const configState = getConfigVersionState(
		host?.agent_version,
		host?.config_version,
	);
	const idMismatch = hasHostIdMismatch(host);
	const conflict = hasIdentityConflict(host);

	if (configState !== CONFIG_VERSION_STATE.MISMATCH && !idMismatch && !conflict)
		return null;

	const identityCopy = [
		conflict && CONFIG_VERSION_COPY.identityConflict,
		idMismatch && CONFIG_VERSION_COPY.hostIdMismatch,
	]
		.filter(Boolean)
		.join(" ");

	return (
		<span className="inline-flex items-center gap-1">
			{configState === CONFIG_VERSION_STATE.MISMATCH && (
				<Tooltip
					content={CONFIG_VERSION_COPY.mismatch(
						host.agent_version,
						host.config_version,
					)}
				>
					{indicatorButton(
						"Config version out of date",
						<AlertTriangle
							className="h-4 w-4 text-warning-500 dark:text-warning-400"
							aria-hidden="true"
						/>,
					)}
				</Tooltip>
			)}
			{(idMismatch || conflict) && (
				<Tooltip content={identityCopy}>
					{indicatorButton(
						conflict
							? "Possible cloned machine"
							: "Host ID mismatch in config.yml",
						<ShieldAlert
							className="h-4 w-4 text-danger-600 dark:text-danger-400"
							aria-hidden="true"
						/>,
					)}
				</Tooltip>
			)}
		</span>
	);
};

export const HostIdentityBanners = ({
	host,
	canManage = false,
	onRegenerateCredentials,
}) => {
	const toast = useToast();
	const queryClient = useQueryClient();
	const idMismatch = hasHostIdMismatch(host);
	const conflict = hasIdentityConflict(host);

	const dismissMutation = useMutation({
		mutationFn: () => adminHostsAPI.dismissIdentityConflict(host.id),
		onSuccess: () => {
			toast.success("Possible clone warning dismissed");
			queryClient.invalidateQueries({ queryKey: ["host", host.id] });
			queryClient.invalidateQueries({ queryKey: ["hosts"] });
		},
		onError: (error) => {
			toast.error(
				error.response?.data?.error || "Failed to dismiss the clone warning",
			);
		},
	});

	if (!idMismatch && !conflict) return null;

	return (
		<div className="space-y-3 mb-4">
			{conflict && (
				<div
					role="alert"
					className="p-3 sm:p-4 bg-danger-50 dark:bg-danger-900/20 border border-danger-200 dark:border-danger-700 rounded-lg"
				>
					<div className="flex flex-col sm:flex-row sm:items-start gap-3">
						<div className="flex items-start gap-2 flex-1 min-w-0">
							<CopyX className="h-5 w-5 text-danger-600 dark:text-danger-400 flex-shrink-0 mt-0.5" />
							<div className="min-w-0">
								<p className="text-sm font-medium text-danger-800 dark:text-danger-200">
									Two machines appear to be reporting with this host's
									credentials (possible clone)
								</p>
								<p className="text-sm text-danger-700 dark:text-danger-300 mt-1">
									Detected {formatDate(host.identity_conflict_at)}. Data on this
									page alternates between the two machines. Regenerate the
									credentials and reinstall the agent on the machine that should
									keep this host, then enrol the other machine as a new host.
									Dismiss this if both reports came from the same machine.
								</p>
							</div>
						</div>
						{canManage && (
							<div className="flex flex-col sm:flex-row gap-2 flex-shrink-0">
								<button
									type="button"
									onClick={onRegenerateCredentials}
									className="btn-danger flex items-center justify-center gap-2 text-sm whitespace-nowrap min-h-[44px] md:min-h-0"
								>
									<Key className="h-4 w-4" />
									Regenerate credentials
								</button>
								<button
									type="button"
									onClick={() => dismissMutation.mutate()}
									disabled={dismissMutation.isPending}
									className="btn-outline flex items-center justify-center gap-2 text-sm whitespace-nowrap min-h-[44px] md:min-h-0 disabled:opacity-50 disabled:cursor-not-allowed"
								>
									<X className="h-4 w-4" />
									{dismissMutation.isPending ? "Dismissing..." : "Dismiss"}
								</button>
							</div>
						)}
					</div>
				</div>
			)}
			{idMismatch && (
				<div
					role="alert"
					className="p-3 sm:p-4 bg-danger-50 dark:bg-danger-900/20 border border-danger-200 dark:border-danger-700 rounded-lg"
				>
					<div className="flex items-start gap-2">
						<ShieldAlert className="h-5 w-5 text-danger-600 dark:text-danger-400 flex-shrink-0 mt-0.5" />
						<div className="min-w-0">
							<span className="badge-danger">Host ID mismatch</span>
							<p className="text-sm text-danger-700 dark:text-danger-300 mt-2">
								The host_id in this agent's config.yml
								{host.reported_host_id ? (
									<>
										{" "}
										(
										<span className="font-mono [overflow-wrap:anywhere]">
											{host.reported_host_id}
										</span>
										)
									</>
								) : null}{" "}
								belongs to a different host from the one its credentials
								authenticate as. Reports are still accepted. Usually config.yml
								was copied from another machine or edited by hand.
							</p>
							<p className="text-sm text-danger-700 dark:text-danger-300 mt-1">
								To fix it, run{" "}
								<code className="font-mono text-xs px-1 py-0.5 rounded bg-danger-100 dark:bg-danger-900/40">
									patchmon-agent config sync-host-id
								</code>{" "}
								on the host, or reinstall the agent with the install command
								from this page.
							</p>
						</div>
					</div>
				</div>
			)}
		</div>
	);
};
