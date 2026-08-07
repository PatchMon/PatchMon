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
import { extraDependencies, hasExtraDependencies } from "../../utils/patchRun";

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
