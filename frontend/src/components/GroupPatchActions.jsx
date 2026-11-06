import { useMutation } from "@tanstack/react-query";
import { Shield, Wrench } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../contexts/AuthContext";
import { useToast } from "../contexts/ToastContext";
import { patchingAPI } from "../utils/patchingApi";
import ModalPortal from "./ui/ModalPortal";

const MODES = {
	patch_all: {
		title: "Patch all",
		body: "Queue a Patch all run for every host currently in this group. Packages held on any of a host's groups are left unchanged. Each host still follows its own patch policy.",
	},
	security: {
		title: "Security patch",
		body: "Queue the packages flagged as security updates on each host. Hosts with none, and packages held by a group, are skipped. Pacman hosts cannot take a partial upgrade.",
	},
};

export default function GroupPatchActions({ groupId, groupName }) {
	const { canManagePatching, hasModule } = useAuth();
	const { success, error: toastError } = useToast();
	const navigate = useNavigate();
	const [mode, setMode] = useState(null);

	const mutation = useMutation({
		mutationFn: (nextMode) => patchingAPI.triggerGroup(groupId, { mode: nextMode }),
		onSuccess: (data) => {
			const results = data.results || [];
			const queued = results.filter((r) =>
				["queued", "scheduled", "pending_approval"].includes(r.status),
			).length;
			const skipped = results.filter((r) => r.status === "skipped").length;
			const failed = results.length - queued - skipped;
			success(
				`${groupName || "Group"}: ${queued} queued, ${skipped} skipped, ${failed} not queued.`,
			);
			setMode(null);
			navigate("/patching");
		},
		onError: (err) => {
			toastError(
				err?.response?.data?.error || "Failed to queue the group patch",
			);
		},
	});

	if (!groupId || !canManagePatching() || !hasModule("patching")) {
		return null;
	}

	const copy = mode ? MODES[mode] : null;

	return (
		<>
			<button
				type="button"
				className="btn-outline inline-flex items-center gap-1.5 px-3 py-2 text-xs sm:text-sm min-h-[44px]"
				onClick={() => setMode("patch_all")}
			>
				<Wrench className="h-4 w-4" />
				Patch all
			</button>
			<button
				type="button"
				className="btn-outline inline-flex items-center gap-1.5 px-3 py-2 text-xs sm:text-sm min-h-[44px]"
				onClick={() => setMode("security")}
			>
				<Shield className="h-4 w-4" />
				Security patch
			</button>
			{copy && (
				<ModalPortal>
					<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-[120]">
						<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 w-full max-w-md">
							<h3 className="text-lg font-semibold text-secondary-900 dark:text-white mb-2">
								{copy.title}
								{groupName ? `: ${groupName}` : ""}
							</h3>
							<p className="text-sm text-secondary-600 dark:text-secondary-200 mb-6">
								{copy.body}
							</p>
							<div className="flex justify-end gap-3">
								<button
									type="button"
									className="btn-outline"
									disabled={mutation.isPending}
									onClick={() => setMode(null)}
								>
									Cancel
								</button>
								<button
									type="button"
									className="btn-primary"
									disabled={mutation.isPending}
									onClick={() => mutation.mutate(mode)}
								>
									{mutation.isPending ? "Queuing..." : "Queue runs"}
								</button>
							</div>
						</div>
					</div>
				</ModalPortal>
			)}
		</>
	);
}
