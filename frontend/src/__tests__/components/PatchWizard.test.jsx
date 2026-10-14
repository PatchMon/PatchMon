/**
 * Unit tests for PatchWizard's partial-failure contract.
 *
 * Three invariants are pinned here:
 *   1. A host that already produced a run is never submitted again.
 *   2. onSuccess fires exactly once per session, with every run that was
 *      actually created, whichever way the wizard is closed, and flags whether
 *      the report was deferred to a close or came straight off a clean submit.
 *   3. Approve mode keys everything by run, so two pending runs on one host
 *      are both approved and both reported against that host.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
	act,
	fireEvent,
	render,
	screen,
	waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { toastWarning } = vi.hoisted(() => ({ toastWarning: vi.fn() }));

vi.mock("../../contexts/ToastContext", () => ({
	useToast: () => ({
		warning: toastWarning,
		success: vi.fn(),
		error: vi.fn(),
		info: vi.fn(),
	}),
}));

vi.mock("../../utils/api", () => ({
	formatDate: (value) => String(value),
	packagesAPI: { getHosts: vi.fn() },
}));

vi.mock("../../utils/patchingApi", () => ({
	patchingAPI: {
		trigger: vi.fn(),
		approveRun: vi.fn(),
		getPreviewRun: vi.fn(),
	},
	pollDryRunUntilDone: vi.fn(),
}));

import PatchWizard from "../../components/PatchWizard";
import { patchingAPI } from "../../utils/patchingApi";
import {
	extraDependencies,
	hasExtraDependencies,
	runMessageNotice,
	skippedItemsSummary,
} from "../../utils/patchRun";

const HOSTS = [
	{ id: "h1", friendly_name: "alpha", hostname: "alpha.local" },
	{ id: "h2", friendly_name: "bravo", hostname: "bravo.local" },
	{ id: "h3", friendly_name: "charlie", hostname: "charlie.local" },
];

// patch_all + lockHosts collapses the wizard to Timing / Approval / Submit,
// so initialStep clamps straight onto Submit with no queries to satisfy
// beyond the per-host schedule preview.
const renderWizard = (presetHosts = HOSTS, extraProps = {}) => {
	const onSuccess = vi.fn();
	const onClose = vi.fn();
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	const view = render(
		<QueryClientProvider client={queryClient}>
			<PatchWizard
				isOpen
				onClose={onClose}
				onSuccess={onSuccess}
				mode="trigger"
				patchType="patch_all"
				lockHosts
				presetHosts={presetHosts}
				initialStep={5}
				{...extraProps}
			/>
		</QueryClientProvider>,
	);
	return { ...view, onSuccess, onClose };
};

// Mirrors the approve-mode prop shape used by pages/Patching.jsx. Approve mode
// also collapses to Timing / Submit, so initialStep still clamps onto Submit.
const renderApproveWizard = (presetHosts, validationRunIds) =>
	renderWizard(presetHosts, {
		mode: "approve",
		patchType: "patch_package",
		packageNames: [],
		lockPackages: true,
		validationRunIds,
		packagesByRun: Object.fromEntries(
			validationRunIds.map((id) => [id, [`pkg-${id}`]]),
		),
		patchTypeByRun: Object.fromEntries(
			validationRunIds.map((id) => [id, "patch_package"]),
		),
	});

const clickAndSettle = async (element) => {
	await act(async () => {
		fireEvent.click(element);
	});
};

const confirmButton = () =>
	screen.getByRole("button", {
		name: /Queue & patch|Approve & patch|Retry \d+ failed (host|run)/,
	});

const triggeredHosts = () => patchingAPI.trigger.mock.calls.map((c) => c[0]);

const approvedRunIds = () => patchingAPI.approveRun.mock.calls.map((c) => c[0]);

describe("PatchWizard partial-failure handling", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		patchingAPI.getPreviewRun.mockResolvedValue({});
	});

	it("portals the overlay above the sidebar and removes it on unmount", () => {
		const { container, unmount, onClose } = renderWizard();
		const backdrop = screen.getByRole("button", { name: "Close modal" });
		const overlay = backdrop.parentElement;
		expect(container).not.toContainElement(overlay);
		expect(overlay.parentElement).toBe(document.body);
		expect(overlay).toHaveClass("z-[120]");
		fireEvent.click(backdrop);
		expect(onClose).toHaveBeenCalledTimes(1);
		unmount();
		expect(overlay).not.toBeInTheDocument();
	});

	it("does not render an overlay when closed", () => {
		renderWizard(HOSTS, { isOpen: false });
		expect(screen.queryByRole("button", { name: "Close modal" })).toBeNull();
	});

	it("never re-submits a host that already produced a run", async () => {
		let bravoAttempts = 0;
		patchingAPI.trigger.mockImplementation(async (hostId) => {
			if (hostId === "h2") {
				bravoAttempts += 1;
				if (bravoAttempts === 1) throw new Error("agent offline");
			}
			return { patch_run_id: `run-${hostId}` };
		});

		const { onSuccess, onClose } = renderWizard();

		await clickAndSettle(confirmButton());
		await waitFor(() =>
			expect(
				screen.getByRole("button", { name: /Retry 1 failed host/ }),
			).toBeInTheDocument(),
		);
		expect(onSuccess).not.toHaveBeenCalled();
		expect(onClose).not.toHaveBeenCalled();

		await clickAndSettle(confirmButton());

		expect(triggeredHosts()).toEqual(["h1", "h2", "h3", "h2"]);
		expect(onSuccess).toHaveBeenCalledTimes(1);
		expect(onSuccess).toHaveBeenCalledWith("patch", {
			runs: [
				{ hostId: "h1", runId: "run-h1", immediate: false },
				{ hostId: "h2", runId: "run-h2", immediate: false },
				{ hostId: "h3", runId: "run-h3", immediate: false },
			],
			deferred: false,
		});
		expect(onClose).toHaveBeenCalledTimes(1);
	});

	it("reports a clean submit as not deferred", async () => {
		patchingAPI.trigger.mockImplementation(async (hostId) => ({
			patch_run_id: `run-${hostId}`,
		}));

		const { onSuccess } = renderWizard(HOSTS.slice(0, 1));

		await clickAndSettle(confirmButton());
		await waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1));

		expect(onSuccess).toHaveBeenCalledWith("patch", {
			runs: [{ hostId: "h1", runId: "run-h1", immediate: false }],
			deferred: false,
		});
	});

	it("issues no further requests when every host has already succeeded", async () => {
		patchingAPI.trigger.mockImplementation(async (hostId) => ({
			patch_run_id: `run-${hostId}`,
		}));

		const { onSuccess, onClose } = renderWizard(HOSTS.slice(0, 2));

		await clickAndSettle(confirmButton());
		await waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1));
		expect(triggeredHosts()).toEqual(["h1", "h2"]);

		await clickAndSettle(confirmButton());

		expect(triggeredHosts()).toEqual(["h1", "h2"]);
		expect(onSuccess).toHaveBeenCalledTimes(1);
		expect(onClose).toHaveBeenCalledTimes(2);
	});

	describe("closing after a partial failure", () => {
		const submitWithOneFailure = async () => {
			patchingAPI.trigger.mockImplementation(async (hostId) => {
				if (hostId === "h2") throw new Error("agent offline");
				return { patch_run_id: `run-${hostId}` };
			});
			const handles = renderWizard();
			await clickAndSettle(confirmButton());
			await waitFor(() =>
				expect(
					screen.getByRole("button", { name: /Retry 1 failed host/ }),
				).toBeInTheDocument(),
			);
			return handles;
		};

		// deferred: true is what stops the caller deep-linking the user into a
		// run when all they did was close the wizard.
		const expectReportedOnce = (onSuccess) => {
			expect(onSuccess).toHaveBeenCalledTimes(1);
			expect(onSuccess).toHaveBeenCalledWith("patch", {
				runs: [
					{ hostId: "h1", runId: "run-h1", immediate: false },
					{ hostId: "h3", runId: "run-h3", immediate: false },
				],
				deferred: true,
			});
		};

		it("reports the created runs when closed from the footer", async () => {
			const { onSuccess, onClose } = await submitWithOneFailure();

			await clickAndSettle(screen.getByRole("button", { name: "Done" }));

			expectReportedOnce(onSuccess);
			expect(onClose).toHaveBeenCalledTimes(1);
		});

		it("reports the created runs when closed from the backdrop", async () => {
			const { onSuccess, onClose } = await submitWithOneFailure();

			await clickAndSettle(screen.getByRole("button", { name: "Close modal" }));

			expectReportedOnce(onSuccess);
			expect(onClose).toHaveBeenCalledTimes(1);
		});

		it("reports the created runs when closed from the header X", async () => {
			const { onSuccess, onClose } = await submitWithOneFailure();
			const closeIcon = screen
				.getByRole("heading", { level: 3 })
				.parentElement.querySelector("button");

			await clickAndSettle(closeIcon);

			expectReportedOnce(onSuccess);
			expect(onClose).toHaveBeenCalledTimes(1);
		});

		it("reports the created runs when closed with Escape", async () => {
			const { onSuccess, onClose } = await submitWithOneFailure();

			await act(async () => {
				fireEvent.keyDown(window, { key: "Escape" });
			});

			expectReportedOnce(onSuccess);
			expect(onClose).toHaveBeenCalledTimes(1);
		});

		it("does not report a second time when closed twice", async () => {
			const { onSuccess, onClose } = await submitWithOneFailure();

			await clickAndSettle(screen.getByRole("button", { name: "Done" }));
			await clickAndSettle(screen.getByRole("button", { name: "Done" }));

			expectReportedOnce(onSuccess);
			expect(onClose).toHaveBeenCalledTimes(2);
		});
	});

	it("does not report anything when the whole batch failed", async () => {
		patchingAPI.trigger.mockRejectedValue(new Error("agent offline"));

		const { onSuccess, onClose } = renderWizard(HOSTS.slice(0, 2));

		await clickAndSettle(confirmButton());
		await waitFor(() =>
			expect(
				screen.getByRole("button", { name: /Retry 2 failed hosts/ }),
			).toBeInTheDocument(),
		);

		expect(onSuccess).not.toHaveBeenCalled();
		expect(onClose).not.toHaveBeenCalled();
		expect(toastWarning).not.toHaveBeenCalled();

		await clickAndSettle(screen.getByRole("button", { name: "Cancel" }));

		expect(onSuccess).not.toHaveBeenCalled();
		expect(onClose).toHaveBeenCalledTimes(1);
	});
});

describe("PatchWizard approve mode", () => {
	let consoleError;

	beforeEach(() => {
		vi.clearAllMocks();
		patchingAPI.getPreviewRun.mockResolvedValue({});
		consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
	});

	afterEach(() => {
		consoleError.mockRestore();
	});

	const duplicateKeyWarnings = () =>
		consoleError.mock.calls.filter((args) =>
			args.some((a) => /same key/i.test(String(a))),
		);

	it("approves both pending runs when they share a host", async () => {
		patchingAPI.approveRun.mockImplementation(async (runId) => ({
			patch_run_id: `run-${runId}`,
		}));

		const { onSuccess } = renderApproveWizard(
			[HOSTS[0], HOSTS[0]],
			["r1", "r2"],
		);

		expect(screen.getAllByText("alpha")).toHaveLength(2);
		expect(duplicateKeyWarnings()).toEqual([]);

		await clickAndSettle(confirmButton());
		await waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1));

		expect(approvedRunIds()).toEqual(["r1", "r2"]);
		expect(duplicateKeyWarnings()).toEqual([]);
	});

	it("re-approves only the failed run after a partial failure", async () => {
		let secondRunAttempts = 0;
		patchingAPI.approveRun.mockImplementation(async (runId) => {
			if (runId === "r2") {
				secondRunAttempts += 1;
				if (secondRunAttempts === 1) throw new Error("agent offline");
			}
			return { patch_run_id: `run-${runId}` };
		});

		const { onSuccess, onClose } = renderApproveWizard(
			[HOSTS[0], HOSTS[0]],
			["r1", "r2"],
		);

		await clickAndSettle(confirmButton());
		await waitFor(() =>
			expect(
				screen.getByRole("button", { name: /Retry 1 failed run/ }),
			).toBeInTheDocument(),
		);
		expect(onSuccess).not.toHaveBeenCalled();

		await clickAndSettle(confirmButton());

		expect(approvedRunIds()).toEqual(["r1", "r2", "r2"]);
		expect(onSuccess).toHaveBeenCalledTimes(1);
		expect(onSuccess).toHaveBeenCalledWith("patch", {
			runs: [
				{ hostId: "h1", runId: "run-r1", immediate: false },
				{ hostId: "h1", runId: "run-r2", immediate: false },
			],
			deferred: false,
		});
		expect(onClose).toHaveBeenCalledTimes(1);
	});

	it("reports every created run against its own host", async () => {
		patchingAPI.approveRun.mockImplementation(async (runId) => ({
			patch_run_id: `run-${runId}`,
		}));

		const { onSuccess } = renderApproveWizard(
			[HOSTS[0], HOSTS[0], HOSTS[1]],
			["r1", "r2", "r3"],
		);

		await clickAndSettle(confirmButton());
		await waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1));

		expect(onSuccess).toHaveBeenCalledWith("patch", {
			runs: [
				{ hostId: "h1", runId: "run-r1", immediate: false },
				{ hostId: "h1", runId: "run-r2", immediate: false },
				{ hostId: "h2", runId: "run-r3", immediate: false },
			],
			deferred: false,
		});
	});
});

describe("extraDependencies", () => {
	it("detects a same-size affected set with different membership", () => {
		expect(
			extraDependencies({
				package_names: ["foo", "bar"],
				packages_affected: ["foo", "baz"],
			}),
		).toEqual(["baz"]);
	});

	it("honours the legacy package_name field", () => {
		expect(
			extraDependencies({
				package_name: "foo",
				packages_affected: ["foo", "libc6"],
			}),
		).toEqual(["libc6"]);
	});

	it("prefers package_names over the legacy field when both are present", () => {
		expect(
			extraDependencies({
				package_name: "legacy",
				package_names: ["foo"],
				packages_affected: ["foo", "legacy"],
			}),
		).toEqual(["legacy"]);
	});

	it("compares case-insensitively", () => {
		expect(
			extraDependencies({
				package_names: ["OpenSSL"],
				packages_affected: ["openssl", "libssl3"],
			}),
		).toEqual(["libssl3"]);
	});

	it("returns an empty list for missing or empty inputs", () => {
		expect(extraDependencies(null)).toEqual([]);
		expect(extraDependencies({ package_names: ["foo"] })).toEqual([]);
		expect(extraDependencies({ packages_affected: ["foo"] })).toEqual(["foo"]);
	});
});

describe("hasExtraDependencies", () => {
	const run = {
		package_names: ["foo"],
		packages_affected: ["foo", "libc6"],
	};

	it("is true only for validated runs", () => {
		expect(hasExtraDependencies({ ...run, status: "validated" })).toBe(true);
		expect(hasExtraDependencies({ ...run, status: "completed" })).toBe(false);
		expect(hasExtraDependencies({ ...run, status: "pending_validation" })).toBe(
			false,
		);
		expect(hasExtraDependencies(run)).toBe(false);
	});

	it("is false for patch_all and Windows runs, which have no dependency resolution", () => {
		const validated = {
			status: "validated",
			package_names: [],
			packages_affected: ["2026-10 Cumulative Update (KB5044284)"],
		};
		expect(
			hasExtraDependencies({ ...validated, patch_type: "patch_all" }),
		).toBe(false);
		expect(
			hasExtraDependencies({
				...validated,
				windows_scope: { updates: true, apps: true },
			}),
		).toBe(false);
		expect(
			hasExtraDependencies({
				...validated,
				patch_type: "patch_package",
				package_names: ["7-Zip 24.08 (x64)"],
				hosts: { os_type: "Windows 11 Pro" },
			}),
		).toBe(false);
		expect(
			hasExtraDependencies({
				...run,
				status: "validated",
				patch_type: "patch_package",
				hosts: { os_type: "Ubuntu" },
			}),
		).toBe(true);
	});

	it("is false for a validated run with no extras", () => {
		expect(
			hasExtraDependencies({
				status: "validated",
				package_names: ["foo"],
				packages_affected: ["foo"],
			}),
		).toBe(false);
	});
});

describe("PatchWizard on Windows hosts", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		patchingAPI.getPreviewRun.mockResolvedValue({});
	});

	const WIN = [
		{
			id: "w1",
			friendly_name: "win11",
			hostname: "win11",
			os_type: "Windows 11",
		},
	];

	const scopeButton = (name) => screen.getByRole("button", { name });

	it("offers both Patch all scopes, ticked, and blocks continuing with neither", async () => {
		renderWizard(WIN, { initialStep: 0 });
		const updates = scopeButton("Windows Updates");
		const apps = scopeButton("Applications (winget)");
		expect(updates).toHaveAttribute("aria-pressed", "true");
		expect(apps).toHaveAttribute("aria-pressed", "true");
		expect(
			screen.getByRole("button", { name: "Run validation" }),
		).toBeEnabled();

		await clickAndSettle(updates);
		await clickAndSettle(apps);
		expect(updates).toHaveAttribute("aria-pressed", "false");
		expect(
			screen.getByRole("button", { name: "Skip validation" }),
		).toBeDisabled();
		expect(
			screen.getByRole("button", { name: "Run validation" }),
		).toBeDisabled();
	});

	it("sends the chosen scope with the Patch all trigger", async () => {
		patchingAPI.trigger.mockResolvedValue({ patch_run_id: "r1" });
		renderWizard(WIN, { initialStep: 0 });
		await clickAndSettle(scopeButton("Applications (winget)"));
		await clickAndSettle(
			screen.getByRole("button", { name: "Skip validation" }),
		);
		await clickAndSettle(screen.getByRole("button", { name: /Next/ }));
		await clickAndSettle(screen.getByRole("button", { name: /Next/ }));
		expect(
			screen.getByText(/except optional, driver and feature upgrades\./),
		).toBeInTheDocument();
		await clickAndSettle(confirmButton());
		await waitFor(() => expect(patchingAPI.trigger).toHaveBeenCalledTimes(1));
		const [hostId, patchType, , , opts] = patchingAPI.trigger.mock.calls[0];
		expect(hostId).toBe("w1");
		expect(patchType).toBe("patch_all");
		expect(opts.windows_scope).toEqual({ updates: true, apps: false });
	});

	it("dry-runs Patch all on Windows with the scope", async () => {
		const { pollDryRunUntilDone } = await import("../../utils/patchingApi");
		patchingAPI.trigger.mockResolvedValue({ patch_run_id: "v1" });
		pollDryRunUntilDone.mockResolvedValue({
			status: "validated",
			packages_affected: ["7-Zip 24.08 (x64)"],
			shell_output: "",
			error: null,
		});
		renderWizard(WIN, { initialStep: 0 });
		await clickAndSettle(
			screen.getByRole("button", { name: "Run validation" }),
		);
		await waitFor(() =>
			expect(pollDryRunUntilDone).toHaveBeenCalledWith(
				"v1",
				expect.any(AbortSignal),
			),
		);
		const [, patchType, , , opts] = patchingAPI.trigger.mock.calls[0];
		expect(patchType).toBe("patch_all");
		expect(opts).toMatchObject({
			dry_run: true,
			windows_scope: { updates: true, apps: true },
		});
		expect(screen.queryByText(/extra dep/)).toBeNull();
		expect(screen.queryByText("Validated with skipped items")).toBeNull();
	});

	it("shows a long validation as still running, not offline", async () => {
		const { pollDryRunUntilDone } = await import("../../utils/patchingApi");
		patchingAPI.trigger.mockResolvedValue({ patch_run_id: "v3" });
		pollDryRunUntilDone.mockResolvedValue({
			status: "still_running",
			packages_affected: [],
			shell_output: "",
			error:
				"Validation is still running on the host. Check Runs & History for the result.",
		});
		renderWizard(WIN, { initialStep: 0 });
		await clickAndSettle(
			screen.getByRole("button", { name: "Run validation" }),
		);
		await waitFor(() =>
			expect(screen.getByText("Still running")).toBeInTheDocument(),
		);
		expect(screen.getByText(/Check Runs & History/)).toBeInTheDocument();
		expect(screen.queryByText("Offline")).toBeNull();
		expect(screen.queryByText(/may be offline/)).toBeNull();
	});

	it("shows what validation will leave alone above the install list", async () => {
		const { pollDryRunUntilDone } = await import("../../utils/patchingApi");
		patchingAPI.trigger.mockResolvedValue({ patch_run_id: "v2" });
		pollDryRunUntilDone.mockResolvedValue({
			status: "validated",
			packages_affected: ["2026-10 Cumulative Update (KB5044284)"],
			shell_output: "",
			skip_note: "1 skipped: 7zip.7zip (pinned in winget)",
			error: null,
		});
		renderWizard(WIN, { initialStep: 0 });
		await clickAndSettle(
			screen.getByRole("button", { name: "Run validation" }),
		);
		const note = await screen.findByText("Validated with skipped items");
		const box = note.closest('[role="status"]');
		expect(box).toHaveTextContent("7zip.7zip (pinned in winget)");
		const list = screen.getByText(/will be updated/);
		expect(
			box.compareDocumentPosition(list) & Node.DOCUMENT_POSITION_FOLLOWING,
		).toBeTruthy();
	});

	it("keeps Linux Patch all without a Validate step or scope", () => {
		renderWizard(HOSTS.slice(0, 1), { initialStep: 0 });
		expect(
			screen.queryByRole("button", { name: "Windows Updates" }),
		).toBeNull();
		expect(screen.queryByRole("button", { name: "Run validation" })).toBeNull();
	});

	it("warns before patching a feature upgrade", () => {
		renderWizard(WIN, {
			patchType: "patch_package",
			packageNames: ["Windows 11, version 25H2"],
			lockPackages: true,
			packageMeta: {
				"Windows 11, version 25H2": { source: "wua", featureUpgrade: true },
			},
		});
		expect(screen.getByRole("alert")).toHaveTextContent(/several GB/);
		expect(screen.getAllByText("Feature upgrade").length).toBeGreaterThan(0);
		expect(screen.getByText("Windows Updates (1)")).toBeInTheDocument();
	});

	it("lists unpatchable pairs and does not offer those hosts", async () => {
		const { packagesAPI } = await import("../../utils/api");
		packagesAPI.getHosts.mockResolvedValue({
			data: {
				hosts: [
					{
						hostId: "w2",
						friendlyName: "winsrv",
						osType: "Windows Server 2025",
						needsUpdate: true,
						patchable: false,
						notPatchableReason: "no_winget_id",
					},
				],
			},
		});
		const queryClient = new QueryClient({
			defaultOptions: { queries: { retry: false } },
		});
		render(
			<QueryClientProvider client={queryClient}>
				<PatchWizard
					isOpen
					onClose={vi.fn()}
					mode="trigger"
					patchType="patch_package"
					packageNames={["Legacy App"]}
				/>
			</QueryClientProvider>,
		);
		await waitFor(() =>
			expect(screen.getByText("Not patchable (1)")).toBeInTheDocument(),
		);
		expect(
			screen.getByText(/cannot be patched on any of the hosts/),
		).toBeInTheDocument();
		expect(screen.getByText(/not found in winget/)).toBeInTheDocument();
	});
});

describe("runMessageNotice", () => {
	it("treats a user stop as neutral, not an error", () => {
		for (const error_message of [
			"Cancelled by user",
			"Run stopped by user",
			"cancelled by the user",
			null,
		]) {
			expect(runMessageNotice({ status: "cancelled", error_message })).toEqual({
				tone: "info",
				title: "Stopped by user",
				detail: null,
			});
		}
	});

	it("keeps another cancel reason as neutral detail", () => {
		expect(
			runMessageNotice({ status: "cancelled", error_message: "Host deleted" }),
		).toEqual({ tone: "info", title: "Run cancelled", detail: "Host deleted" });
	});

	it("shows a skip summary on a successful run as a neutral note", () => {
		const detail = "1 skipped: 7zip.7zip (pinned in winget)";
		expect(
			runMessageNotice({ status: "completed", error_message: detail }),
		).toEqual({ tone: "info", title: "Completed with skipped items", detail });
		for (const status of ["validated", "dry_run_completed"]) {
			expect(runMessageNotice({ status, error_message: detail })).toEqual({
				tone: "info",
				title: "Validated with skipped items",
				detail,
			});
		}
	});

	it("keeps the error panel for a failed run with skips mixed in", () => {
		const detail =
			"1 of 2 failed: KB5044284 (0x800f0922); 1 skipped: 7zip.7zip (pinned in winget)";
		expect(
			runMessageNotice({ status: "failed", error_message: detail }),
		).toEqual({ tone: "error", title: "Run reported an error", detail });
	});

	it("reports real failures as errors, and nothing when there is no message", () => {
		expect(
			runMessageNotice({ status: "failed", error_message: "1 of 1 failed" }),
		).toEqual({
			tone: "error",
			title: "Run reported an error",
			detail: "1 of 1 failed",
		});
		expect(runMessageNotice({ status: "completed" })).toBeNull();
	});
});

describe("skippedItemsSummary", () => {
	it("prefers the server summary", () => {
		expect(
			skippedItemsSummary({
				status: "validated",
				error_message: "1 skipped: 7zip.7zip (pinned in winget)",
				items: [],
			}),
		).toBe("1 skipped: 7zip.7zip (pinned in winget)");
	});

	it("builds one from skipped items", () => {
		expect(
			skippedItemsSummary({
				status: "validated",
				items: [
					{ status: "would_install", title: "KB5044284" },
					{
						status: "skipped",
						identifier: "7zip.7zip",
						message: "pinned in winget",
					},
					{ status: "skipped", title: "Driver X" },
				],
			}),
		).toBe("2 skipped: 7zip.7zip (pinned in winget), Driver X");
	});

	it("is null when nothing is skipped", () => {
		expect(skippedItemsSummary({ status: "validated", items: [] })).toBeNull();
		expect(skippedItemsSummary({})).toBeNull();
	});
});
