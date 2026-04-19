import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowDown,
	ArrowUp,
	ArrowUpRight,
	Calendar,
	Check,
	CheckCircle,
	CreditCard,
	ExternalLink,
	HelpCircle,
	Minus,
	RefreshCw,
	Server,
	Settings,
	Sparkles,
	X,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import {
	getNextTier,
	getTier,
	TIER_FEATURES,
	TIER_ORDER,
	TIERS,
} from "../constants/tiers";
import { useSettings } from "../contexts/SettingsContext";
import { useToast } from "../contexts/ToastContext";
import { billingAPI, formatRelativeTime } from "../utils/api";

// Annual pre-commitment hard cap (matches backend fat-finger protection).
const COMMIT_HOSTS_MAX = 10000;

// --- Formatting helpers ---------------------------------------------------

const CURRENCY_SYMBOLS = {
	usd: "$",
	gbp: "£",
	eur: "€",
};

const formatMoney = (cents, currency) => {
	const symbol = CURRENCY_SYMBOLS[currency] || "";
	const value = (Number(cents || 0) / 100).toFixed(2);
	return `${symbol}${value}`;
};

const formatDate = (iso) => {
	if (!iso) return null;
	try {
		return new Date(iso).toLocaleDateString(undefined, {
			day: "numeric",
			month: "short",
			year: "numeric",
		});
	} catch {
		return null;
	}
};

const daysUntil = (iso) => {
	if (!iso) return null;
	const diff = new Date(iso).getTime() - Date.now();
	if (Number.isNaN(diff)) return null;
	return Math.max(0, Math.ceil(diff / (1000 * 60 * 60 * 24)));
};

const capitalize = (s) => (s ? s.charAt(0).toUpperCase() + s.slice(1) : s);

const intervalLabel = (interval) =>
	interval === "year" ? "Annual" : "Monthly";

// --- Status banner --------------------------------------------------------

const STATUS_BANNERS = {
	past_due: {
		cls: "bg-danger-50 dark:bg-danger-900/20 border-danger-200 dark:border-danger-800 text-danger-700 dark:text-danger-300",
		iconCls: "text-danger-600 dark:text-danger-400",
		title: "Payment past due",
		body: "Your last invoice failed. Please update your payment method in the Stripe portal to avoid service interruption.",
	},
	canceled: {
		cls: "bg-secondary-100 dark:bg-secondary-800 border-secondary-300 dark:border-secondary-600 text-secondary-700 dark:text-secondary-200",
		iconCls: "text-secondary-600 dark:text-secondary-300",
		title: "Subscription canceled",
		body: "Your subscription has been canceled. You can reactivate from the Stripe portal.",
	},
	paused: {
		cls: "bg-warning-50 dark:bg-warning-900/20 border-warning-200 dark:border-warning-800 text-warning-700 dark:text-warning-300",
		iconCls: "text-warning-600 dark:text-warning-400",
		title: "Subscription paused",
		body: "Your trial ended without a payment method on file. Add one in the Stripe portal to resume service.",
	},
	grace: {
		cls: "bg-warning-50 dark:bg-warning-900/20 border-warning-200 dark:border-warning-800 text-warning-700 dark:text-warning-300",
		iconCls: "text-warning-600 dark:text-warning-400",
		title: "Grace period active",
		body: "Your plan is in a grace period. Please resolve the outstanding issue to avoid losing access.",
	},
};

// --- Main page ------------------------------------------------------------

const Billing = () => {
	const { settings: publicSettings } = useSettings();
	const { error: toastError } = useToast();
	const queryClient = useQueryClient();

	// Tier-change modal state. `target` is the tier id the modal opens with
	// pre-selected (null when launched from "Change plan" so the user picks).
	const [modalOpen, setModalOpen] = useState(false);
	const [modalTarget, setModalTarget] = useState(null);

	// Inline banner state for the "Sync host count" action. We deliberately
	// use a local banner inside the Host usage card rather than a toast so
	// the feedback appears next to the button that triggered the action.
	// `kind` is "success" | "error"; `message` is pre-formatted for display.
	// Auto-dismisses after 5s on success (error banners stay until the next
	// action so users can read them).
	const [syncBanner, setSyncBanner] = useState(null);

	const {
		data: billing,
		isLoading,
		isError,
		error: queryError,
		refetch,
	} = useQuery({
		queryKey: ["billing", "me"],
		queryFn: () => billingAPI.getCurrent().then((res) => res.data),
		retry: (failureCount, err) => {
			const status = err?.response?.status;
			if (status === 404 || status === 403) return false;
			return failureCount < 2;
		},
	});

	const syncMutation = useMutation({
		mutationFn: () => billingAPI.sync().then((res) => res.data),
		onSuccess: (data) => {
			// Provisioner returns {tenant_id, active_host_count, sampled_at,
			// forwarded_to_manager}. No billing_state in this shape — the
			// authoritative next-invoice figure comes from the subsequent
			// GetMyBilling refetch via the query invalidation below.
			const count = data?.active_host_count;
			const msg =
				typeof count === "number"
					? `Host count synced: ${count} active host${count === 1 ? "" : "s"}.`
					: "Host count synced.";
			setSyncBanner({ kind: "success", message: msg });
			queryClient.invalidateQueries({ queryKey: ["billing", "me"] });
		},
		onError: () => {
			setSyncBanner({
				kind: "error",
				message: "Sync failed. Try again in a moment.",
			});
		},
	});

	// Auto-dismiss the success banner after 5s. Error banners stay until the
	// next user action (triggering another sync will overwrite them).
	useEffect(() => {
		if (syncBanner?.kind !== "success") return undefined;
		const t = setTimeout(() => setSyncBanner(null), 5000);
		return () => clearTimeout(t);
	}, [syncBanner]);

	const portalMutation = useMutation({
		mutationFn: () =>
			billingAPI
				.createPortalSession(window.location.href)
				.then((res) => res.data),
		onSuccess: (data) => {
			if (data?.url) {
				window.location.assign(data.url);
			} else {
				toastError(
					"Stripe portal session did not return a URL. Please try again.",
				);
			}
		},
		onError: (err) => {
			const msg =
				err?.response?.data?.error ||
				err?.message ||
				"Failed to open the Stripe billing portal. Please try again.";
			toastError(msg);
		},
	});

	// Derived values
	const tier = getTier(billing?.tier);
	const nextTier = getNextTier(billing?.tier);
	const currency = billing?.currency || "gbp";
	const interval = billing?.interval || "month";

	const billedQuantity = useMemo(() => {
		if (!billing) return 0;
		return Number(billing.quantity || 0);
	}, [billing]);

	// --- Render branches: ADMIN_MODE gate (non-available), loading, errors ---

	// ADMIN_MODE off on this installation: do not render billing.
	if (publicSettings && publicSettings.admin_mode !== true) {
		return (
			<div className="px-4 sm:px-6 lg:px-8 py-4">
				<div className="card p-6 max-w-2xl mx-auto mt-12 text-center">
					<CreditCard className="h-12 w-12 text-secondary-400 mx-auto mb-4" />
					<h2 className="text-lg font-medium text-secondary-900 dark:text-white mb-2">
						Billing is not available on this installation
					</h2>
					<p className="text-sm text-secondary-600 dark:text-white/80">
						The Billing dashboard is only available on PatchMon Cloud. On
						self-hosted installations, Community Edition is free forever under
						AGPLv3.
					</p>
				</div>
			</div>
		);
	}

	if (isLoading) {
		return (
			<div className="px-4 sm:px-6 lg:px-8 py-4">
				<div className="flex items-center justify-center h-64">
					<RefreshCw className="h-8 w-8 animate-spin text-primary-600" />
				</div>
			</div>
		);
	}

	// 404: installation has no subscription row — treat as "not available".
	const status = queryError?.response?.status;
	if (isError && status === 404) {
		return (
			<div className="px-4 sm:px-6 lg:px-8 py-4">
				<div className="card p-6 max-w-2xl mx-auto mt-12 text-center">
					<CreditCard className="h-12 w-12 text-secondary-400 mx-auto mb-4" />
					<h2 className="text-lg font-medium text-secondary-900 dark:text-white mb-2">
						Billing is not available on this installation
					</h2>
					<p className="text-sm text-secondary-600 dark:text-white/80">
						No subscription is associated with this workspace yet. If you
						expected to see billing information here, contact your account
						owner.
					</p>
				</div>
			</div>
		);
	}

	if (isError && status === 403) {
		return (
			<div className="px-4 sm:px-6 lg:px-8 py-4">
				<div className="card p-6 max-w-2xl mx-auto mt-12 text-center">
					<AlertTriangle className="h-12 w-12 text-warning-500 mx-auto mb-4" />
					<h2 className="text-lg font-medium text-secondary-900 dark:text-white mb-2">
						You don't have permission to view this page
					</h2>
					<p className="text-sm text-secondary-600 dark:text-white/80">
						Ask an administrator to grant you the Manage Billing permission.
					</p>
				</div>
			</div>
		);
	}

	if (isError) {
		return (
			<div className="px-4 sm:px-6 lg:px-8 py-4">
				<div className="card p-6 max-w-2xl mx-auto mt-12">
					<div className="flex items-start gap-3">
						<AlertTriangle className="h-6 w-6 text-danger-500 flex-shrink-0 mt-0.5" />
						<div className="flex-1">
							<h2 className="text-lg font-medium text-secondary-900 dark:text-white mb-1">
								Could not load billing information
							</h2>
							<p className="text-sm text-secondary-600 dark:text-white/80 mb-4">
								{queryError?.response?.data?.error ||
									queryError?.message ||
									"An unexpected error occurred."}
							</p>
							<button
								type="button"
								className="btn-primary min-h-[44px]"
								onClick={() => refetch()}
							>
								Try again
							</button>
						</div>
					</div>
				</div>
			</div>
		);
	}

	// --- Derived UI state ----------------------------------------------------

	const renewalDate = formatDate(billing.current_period_end);
	const trialDays = daysUntil(billing.trial_end);
	const trialChargeDate = formatDate(billing.trial_end);
	const nextInvoiceFormatted = formatMoney(
		billing.next_invoice_cents,
		currency,
	);
	const unitPrice = formatMoney(billing.unit_amount_cents, currency);
	const totalLine = formatMoney(
		Number(billing.unit_amount_cents || 0) * billedQuantity,
		currency,
	);

	const statusBanner =
		billing.status !== "active" && billing.status !== "trialing"
			? STATUS_BANNERS[billing.status] || STATUS_BANNERS.grace
			: null;

	// billedQuantity (already memoised above) = what Stripe charges for.
	// On a healthy sync it equals max(active, committed, 1). There is no
	// tier-level minimum any more — customer pays for exactly what they run.
	// activeHosts = live local count from tenant DB (injected by the server
	// billing proxy). Distinguishing these two is the whole point of the
	// Phase 5e UX rework — "Active: 25" was ambiguous when the billed qty and
	// the actual running host count drifted apart after a downgrade.
	const activeHosts = Number(
		billing.active_host_count ?? billing.quantity ?? 0,
	);
	const committedHosts = Number(billing.committed_hosts || 0);
	const isAnnualPlan = interval === "year";
	const hasAnnualCommit = isAnnualPlan && committedHosts > 0;
	const overCommitment = Math.max(0, activeHosts - committedHosts);

	// --- Page ---------------------------------------------------------------

	return (
		<div className="px-4 sm:px-6 lg:px-8 py-4">
			{/* Page header */}
			<div className="flex flex-col sm:flex-row sm:items-center sm:justify-between mb-6 gap-3">
				<div>
					<h1 className="text-2xl font-semibold text-secondary-900 dark:text-white">
						Billing
					</h1>
					<p className="text-sm text-secondary-600 dark:text-white/80 mt-1">
						Your plan, usage, and invoices for this workspace.
					</p>
				</div>
				<div className="flex items-center gap-2">
					<button
						type="button"
						className="btn-outline min-h-[44px] flex items-center gap-2"
						onClick={() => refetch()}
					>
						<RefreshCw className="h-4 w-4" />
						<span className="hidden sm:inline">Refresh</span>
					</button>
				</div>
			</div>

			{/* Top card: Current plan */}
			<div className="card p-4 sm:p-6 mb-6">
				<div className="flex flex-col md:flex-row md:items-start md:justify-between gap-4">
					<div className="flex-1 min-w-0">
						<div className="flex items-center gap-3 flex-wrap mb-2">
							<h2 className="text-lg font-medium text-secondary-900 dark:text-white">
								Current plan
							</h2>
							{tier ? (
								<span
									className={`inline-flex items-center px-2.5 py-0.5 rounded text-xs font-medium ${tier.badgeClass}`}
								>
									{tier.name}
								</span>
							) : (
								<span className="inline-flex items-center px-2.5 py-0.5 rounded text-xs font-medium bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200">
									{capitalize(billing.tier) || "Unknown"}
								</span>
							)}
							<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">
								{intervalLabel(interval)} · {currency.toUpperCase()}
							</span>
							{billing.status === "trialing" && (
								<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200">
									Trial
								</span>
							)}
							{billing.status === "active" && (
								<span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
									<CheckCircle className="h-3 w-3" /> Active
								</span>
							)}
						</div>

						<div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-sm text-secondary-600 dark:text-white/80 mb-4">
							{renewalDate && (
								<span className="inline-flex items-center gap-1.5">
									<Calendar className="h-4 w-4" />
									{billing.cancel_at_period_end ? "Ends" : "Renews"}{" "}
									{renewalDate}
								</span>
							)}
							{tier?.tagline && (
								<span className="hidden md:inline text-secondary-500 dark:text-white/60">
									{tier.tagline}
								</span>
							)}
						</div>
					</div>

					<div className="md:text-right bg-secondary-50 dark:bg-secondary-700 rounded-lg p-4 md:min-w-[220px]">
						<p className="text-xs uppercase tracking-wider text-secondary-500 dark:text-white/70">
							Next invoice
						</p>
						<p className="text-2xl font-semibold text-secondary-900 dark:text-white mt-1">
							{nextInvoiceFormatted}
						</p>
						{renewalDate && (
							<p className="text-xs text-secondary-500 dark:text-white/70 mt-1">
								due {renewalDate}
							</p>
						)}
					</div>
				</div>

				{/* Trial banner — primary brand tone (not warning amber). A
				    trial is an informational state, not an alert. */}
				{billing.status === "trialing" && trialDays !== null && (
					<div className="mt-4 rounded-md border bg-primary-50 dark:bg-primary-900/20 border-primary-200 dark:border-primary-800 p-3 sm:p-4 flex items-start gap-3">
						<Sparkles className="h-5 w-5 text-primary-600 dark:text-primary-400 flex-shrink-0 mt-0.5" />
						<div className="flex-1 min-w-0">
							<p className="text-sm font-medium text-primary-900 dark:text-primary-100">
								{trialDays} {trialDays === 1 ? "day" : "days"} left in trial
							</p>
							<p className="text-sm text-primary-800 dark:text-primary-200 mt-0.5">
								{nextInvoiceFormatted} will be charged
								{trialChargeDate ? ` on ${trialChargeDate}` : ""} unless you
								change or cancel your plan.
							</p>
						</div>
					</div>
				)}

				{/* Cancel-at-period-end banner */}
				{billing.cancel_at_period_end && (
					<div className="mt-4 rounded-md border bg-secondary-100 dark:bg-secondary-800 border-secondary-300 dark:border-secondary-600 p-3 sm:p-4 flex items-start gap-3">
						<AlertTriangle className="h-5 w-5 text-secondary-600 dark:text-secondary-300 flex-shrink-0 mt-0.5" />
						<div className="flex-1 min-w-0">
							<p className="text-sm font-medium text-secondary-900 dark:text-white">
								Subscription ends on {renewalDate || "the next renewal date"}
							</p>
							<p className="text-sm text-secondary-700 dark:text-secondary-200 mt-0.5">
								You can reactivate any time in the Stripe portal.
							</p>
						</div>
					</div>
				)}

				{/* Status banner (past_due / canceled / paused / grace) */}
				{statusBanner && (
					<div
						className={`mt-4 rounded-md border p-3 sm:p-4 flex items-start gap-3 ${statusBanner.cls}`}
					>
						<AlertTriangle
							className={`h-5 w-5 flex-shrink-0 mt-0.5 ${statusBanner.iconCls}`}
						/>
						<div className="flex-1 min-w-0">
							<p className="text-sm font-medium">{statusBanner.title}</p>
							<p className="text-sm mt-0.5">{statusBanner.body}</p>
						</div>
					</div>
				)}
			</div>

			{/* Middle card: Host usage */}
			<div className="card p-4 sm:p-6 mb-6">
				<div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-3 mb-4">
					<div className="flex items-center gap-2">
						<Server className="h-5 w-5 text-primary-600" />
						<h2 className="text-lg font-medium text-secondary-900 dark:text-white">
							Host usage
						</h2>
					</div>
					<div className="flex flex-col items-stretch sm:items-end gap-1">
						<button
							type="button"
							onClick={() => {
								setSyncBanner(null);
								syncMutation.mutate();
							}}
							disabled={syncMutation.isPending}
							className="btn-secondary min-h-[44px] flex items-center justify-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
						>
							{syncMutation.isPending ? (
								<>
									<RefreshCw className="h-4 w-4 animate-spin" />
									Syncing...
								</>
							) : (
								<>
									<RefreshCw className="h-4 w-4" />
									Sync host count
								</>
							)}
						</button>
						<p className="text-xs text-secondary-500 dark:text-white/60 sm:text-right">
							{billing.last_synced_at
								? `Last synced ${formatRelativeTime(billing.last_synced_at)}`
								: "Never synced"}
						</p>
					</div>
				</div>

				{/* Inline banner for sync success / failure. Sits above the
				    usage numbers so it's visually tied to the Sync button. */}
				{syncBanner && (
					<div
						className={`mb-4 rounded-md border p-3 flex items-start gap-3 ${
							syncBanner.kind === "success"
								? "bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800 text-green-800 dark:text-green-200"
								: "bg-danger-50 dark:bg-danger-900/20 border-danger-200 dark:border-danger-800 text-danger-700 dark:text-danger-300"
						}`}
					>
						{syncBanner.kind === "success" ? (
							<CheckCircle className="h-5 w-5 flex-shrink-0 mt-0.5" />
						) : (
							<AlertTriangle className="h-5 w-5 flex-shrink-0 mt-0.5" />
						)}
						<p className="text-sm flex-1 min-w-0">{syncBanner.message}</p>
						<button
							type="button"
							onClick={() => setSyncBanner(null)}
							aria-label="Dismiss"
							className="flex-shrink-0 opacity-70 hover:opacity-100"
						>
							<X className="h-4 w-4" />
						</button>
					</div>
				)}

				{/* Three-row usage breakdown. Row 1 = live reality, Row 2 = annual
				    contract (hidden on monthly), Row 3 = invoice math. Billed is
				    largest/boldest per the Phase 5e UX spec — it's what the
				    customer came to this page for. */}
				<div
					className={`grid grid-cols-1 ${hasAnnualCommit ? "md:grid-cols-3" : "md:grid-cols-2"} gap-4 mb-4`}
				>
					{/* Active hosts */}
					<div className="bg-secondary-50 dark:bg-secondary-700 rounded-lg p-4">
						<p className="text-xs uppercase tracking-wider text-secondary-500 dark:text-white/70">
							Active hosts
						</p>
						<p className="text-2xl font-semibold text-secondary-900 dark:text-white mt-1">
							{activeHosts}{" "}
							<span className="text-sm font-normal text-secondary-500 dark:text-white/70">
								host{activeHosts === 1 ? "" : "s"}
							</span>
						</p>
					</div>

					{/* Annual commitment (only on annual plans) */}
					{hasAnnualCommit && (
						<div className="bg-secondary-50 dark:bg-secondary-700 rounded-lg p-4">
							<div className="flex items-center gap-2">
								<p className="text-xs uppercase tracking-wider text-secondary-500 dark:text-white/70">
									Annual commitment
								</p>
							</div>
							<p className="text-2xl font-semibold text-secondary-900 dark:text-white mt-1">
								{committedHosts}{" "}
								<span className="text-sm font-normal text-secondary-500 dark:text-white/70">
									host{committedHosts === 1 ? "" : "s"}
								</span>
							</p>
							<p className="text-xs text-secondary-500 dark:text-white/70 mt-1">
								prepaid capacity for the year
							</p>
						</div>
					)}

					{/* Billed this period — largest, emphasis */}
					<div className="bg-primary-50 dark:bg-primary-900/20 rounded-lg p-4 border border-primary-100 dark:border-primary-800">
						<p className="text-xs uppercase tracking-wider text-primary-700 dark:text-primary-300 font-semibold">
							Billed this period
						</p>
						<p className="text-3xl font-bold text-primary-900 dark:text-primary-100 mt-1">
							{billedQuantity}{" "}
							<span className="text-base font-normal text-primary-700 dark:text-primary-300">
								× {unitPrice}
							</span>
						</p>
						<p className="text-sm text-primary-800 dark:text-primary-200 mt-1 font-medium">
							= {totalLine}/{isAnnualPlan ? "year" : "month"}
						</p>
					</div>
				</div>

				{/* Overage warning (annual only, when live count exceeds commit) */}
				{hasAnnualCommit && overCommitment > 0 && (
					<div className="mb-4 rounded-md border bg-warning-50 dark:bg-warning-900/20 border-warning-200 dark:border-warning-800 p-3 flex items-start gap-3">
						<AlertTriangle className="h-5 w-5 text-warning-600 dark:text-warning-400 flex-shrink-0 mt-0.5" />
						<p className="text-sm text-warning-800 dark:text-warning-200 flex-1 min-w-0">
							{overCommitment} host{overCommitment === 1 ? "" : "s"} above your
							annual commitment. These will be prorated on your next invoice at{" "}
							{unitPrice} per host.
						</p>
					</div>
				)}

				<div className="rounded-md bg-secondary-50 dark:bg-secondary-700 border border-secondary-200 dark:border-secondary-600 px-3 py-2 text-sm text-secondary-800 dark:text-secondary-200">
					You pay for {billedQuantity} host{billedQuantity === 1 ? "" : "s"} ×{" "}
					{unitPrice}/host/
					{interval === "year" ? "year" : "month"} = {totalLine}
				</div>
			</div>

			{/* Bottom card: Tier matrix + upgrade */}
			<div className="card p-4 sm:p-6 mb-6">
				<div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-4">
					<div>
						<h2 className="text-lg font-medium text-secondary-900 dark:text-white">
							What's included
						</h2>
						<p className="text-sm text-secondary-600 dark:text-white/80 mt-1">
							Compare tiers side by side. Your current tier is highlighted.
						</p>
					</div>
					<div className="flex flex-col items-stretch sm:flex-row sm:items-center gap-2">
						<button
							type="button"
							onClick={() => {
								setModalTarget(null);
								setModalOpen(true);
							}}
							className="btn-outline min-h-[44px] flex items-center justify-center gap-2"
						>
							<Settings className="h-4 w-4" />
							Change plan
						</button>
						{nextTier && (
							<UpgradeCta
								currentTier={tier}
								nextTier={nextTier}
								onClick={() => {
									setModalTarget(nextTier.id);
									setModalOpen(true);
								}}
							/>
						)}
					</div>
				</div>

				<TierMatrix currentTierId={billing.tier} />
			</div>

			{/* Tier change modal (upgrade / downgrade / interval swap). */}
			{modalOpen && (
				<TierChangeModal
					currentTierId={billing.tier}
					currentInterval={interval}
					currency={currency}
					currentQuantity={billedQuantity}
					currentCommittedHosts={committedHosts}
					initialTarget={modalTarget}
					onClose={() => setModalOpen(false)}
					onApplied={() => {
						// Refresh the billing card so the new tier / next
						// invoice reflect the Stripe mutation.
						refetch();
					}}
				/>
			)}

			{/* Footer actions */}
			<div className="card p-4 sm:p-6 mb-6 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
				<div>
					<h2 className="text-lg font-medium text-secondary-900 dark:text-white">
						Manage payment and invoices
					</h2>
					<p className="text-sm text-secondary-600 dark:text-white/80 mt-1">
						Update your card, view invoices, and cancel or reactivate your
						subscription in the Stripe billing portal.
					</p>
				</div>
				<div className="flex flex-wrap items-center gap-2">
					<a
						href="https://patchmon.net/pricing#faq"
						target="_blank"
						rel="noopener noreferrer"
						className="btn-outline min-h-[44px] flex items-center gap-2"
					>
						<HelpCircle className="h-4 w-4" />
						View FAQs
						<ExternalLink className="h-3 w-3" />
					</a>
					<button
						type="button"
						onClick={() => portalMutation.mutate()}
						disabled={portalMutation.isPending}
						className="btn-primary min-h-[44px] flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
					>
						{portalMutation.isPending ? (
							<>
								<RefreshCw className="h-4 w-4 animate-spin" />
								Opening portal...
							</>
						) : (
							<>
								<CreditCard className="h-4 w-4" />
								Manage billing in Stripe
								<ArrowUpRight className="h-3 w-3" />
							</>
						)}
					</button>
				</div>
			</div>
		</div>
	);
};

// --- Upgrade CTA ----------------------------------------------------------

const UpgradeCta = ({ currentTier, nextTier, onClick }) => {
	const deltaCents =
		(nextTier?.unitAmountCents || 0) - (currentTier?.unitAmountCents || 0);
	const deltaText =
		deltaCents > 0 ? `+${(deltaCents / 100).toFixed(2)}/host/month` : "";

	return (
		<button
			type="button"
			onClick={onClick}
			className="btn-primary min-h-[44px] flex items-center justify-center gap-2"
		>
			<Sparkles className="h-4 w-4" />
			Upgrade to {nextTier.name}
			{deltaText && (
				<span className="ml-1 text-xs opacity-80">({deltaText})</span>
			)}
		</button>
	);
};

// --- Tier change modal ----------------------------------------------------

// TierChangeModal renders the confirmation dialog used by both the "Upgrade
// to <next>" CTA and the "Change plan" button. It fetches a server-side
// preview for the currently selected target tier + interval combo so the user
// sees the exact charge before confirming.
//
// Layout (desktop, stacks on mobile):
//   [ Tier radio cards: Starter / Plus / Max  ]
//   [ Interval toggle: Monthly  /  Annual     ]
//   [ Preview panel (upgrade vs downgrade copy)]
//   [ Error banner if any                     ]
//   [ Cancel (secondary)    Confirm (primary) ]
//
// Success behaviour: posts to /tier-change, shows an inline success banner
// for 1.5s, then closes and asks the parent to refetch the billing query.
const TierChangeModal = ({
	currentTierId,
	currentInterval,
	currency,
	currentQuantity,
	currentCommittedHosts,
	initialTarget,
	onClose,
	onApplied,
}) => {
	const queryClient = useQueryClient();
	const { error: toastError } = useToast();

	// Default target: user-supplied CTA target, or current tier if the modal
	// was opened from "Change plan" (so the user can just flip intervals).
	const [targetTier, setTargetTier] = useState(
		initialTarget || currentTierId || "plus",
	);
	const [targetInterval, setTargetInterval] = useState(
		currentInterval || "month",
	);
	const [applied, setApplied] = useState(false);

	// Pre-commit hosts (annual only). Default: current committed value if
	// set, otherwise the current billed quantity. We store as a string so
	// users can clear the field while typing; validation happens on submit.
	const commitFloor = Math.max(
		Number(currentQuantity || 0),
		Number(currentCommittedHosts || 0),
	);
	const commitDefault = Math.max(
		Number(currentCommittedHosts || 0) || Number(currentQuantity || 0) || 0,
		Number(currentQuantity || 0),
	);
	const [commitHosts, setCommitHosts] = useState(String(commitDefault || 0));

	// Debounced value actually used for preview fetches. We update
	// `debouncedCommit` 500ms after the user stops typing so we don't spam
	// the preview endpoint on every keystroke.
	const [debouncedCommit, setDebouncedCommit] = useState(commitDefault);
	const commitDebounceRef = useRef(null);
	useEffect(() => {
		if (commitDebounceRef.current) clearTimeout(commitDebounceRef.current);
		commitDebounceRef.current = setTimeout(() => {
			const parsed = Number.parseInt(commitHosts, 10);
			setDebouncedCommit(Number.isFinite(parsed) ? parsed : 0);
		}, 500);
		return () => {
			if (commitDebounceRef.current) clearTimeout(commitDebounceRef.current);
		};
	}, [commitHosts]);

	const isAnnual = targetInterval === "year";
	// The commit input appears on ANY annual selection — upgrade, downgrade,
	// or interval-only swap. Annual commitment is a billing concept (pre-pay
	// for N hosts for the year) independent of tier direction. A customer
	// downgrading to Starter annual should still be able to commit to a
	// higher host count than the current quantity if they expect growth.
	const showCommitInput = isAnnual;

	// Commit value we actually send to the backend. Omitted on monthly
	// (backend rejects commit_hosts with 400 there). isDowngradeTier is
	// kept out of the decision — commitment is interval-level, not
	// direction-level.
	const commitValueForRequest = showCommitInput ? debouncedCommit : undefined;

	// Client-side validation: the committed value must be at least the
	// current subscription quantity (backend enforces the same rule).
	const commitParsed = Number.parseInt(commitHosts, 10);
	const commitInvalid =
		showCommitInput &&
		(!Number.isFinite(commitParsed) ||
			commitParsed < commitFloor ||
			commitParsed > COMMIT_HOSTS_MAX);

	const preview = useQuery({
		queryKey: [
			"billing",
			"tier-change-preview",
			targetTier,
			targetInterval,
			commitValueForRequest ?? null,
		],
		queryFn: () =>
			billingAPI
				.previewTierChange({
					new_tier: targetTier,
					interval: targetInterval,
					commit_hosts: commitValueForRequest ?? undefined,
				})
				.then((res) => res.data),
		retry: false,
		// Keep stale preview on the panel while the next one loads so the
		// numbers don't flicker between radio clicks.
		staleTime: 5_000,
		// Skip the fetch while the user is typing an invalid commit value,
		// so we don't flash an error banner for a transient input state.
		enabled: !commitInvalid,
	});

	const applyMutation = useMutation({
		mutationFn: () =>
			billingAPI
				.applyTierChange({
					new_tier: targetTier,
					interval: targetInterval,
					commit_hosts: commitValueForRequest ?? undefined,
				})
				.then((res) => res.data),
		onSuccess: () => {
			setApplied(true);
			queryClient.invalidateQueries({ queryKey: ["billing", "me"] });
			// Short delay so the success banner is visible before the modal
			// unmounts. Long enough to read, short enough to feel snappy.
			// The cleanup effect below clears this if the modal unmounts early.
			setTimeout(() => {
				onApplied?.();
				onClose();
			}, 1400);
		},
		onError: (err) => {
			const msg =
				err?.response?.data?.error ||
				err?.message ||
				"Failed to apply the tier change. Please try again.";
			toastError(msg);
		},
	});

	// Close on Escape, unless we're mid-apply (don't lose the request).
	useEffect(() => {
		const handler = (e) => {
			if (e.key === "Escape" && !applyMutation.isPending) {
				onClose();
			}
		};
		window.addEventListener("keydown", handler);
		return () => window.removeEventListener("keydown", handler);
	}, [onClose, applyMutation.isPending]);

	const previewData = preview.data;
	const previewErrMsg =
		preview.error?.response?.data?.error || preview.error?.message || null;
	const applyErrMsg =
		applyMutation.error?.response?.data?.error ||
		applyMutation.error?.message ||
		null;

	const isNoop =
		currentTierId === targetTier && currentInterval === targetInterval;

	return (
		<div
			className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60"
			role="dialog"
			aria-modal="true"
			aria-labelledby="tier-change-title"
		>
			<div className="card w-full max-w-2xl max-h-[90vh] overflow-y-auto p-4 sm:p-6">
				<div className="flex items-start justify-between gap-3 mb-4">
					<div>
						<h2
							id="tier-change-title"
							className="text-lg font-medium text-secondary-900 dark:text-white"
						>
							Change plan
						</h2>
						<p className="text-sm text-secondary-600 dark:text-white/80 mt-1">
							Pick your new tier and billing interval. We'll show you the exact
							charge before confirming.
						</p>
					</div>
					<button
						type="button"
						onClick={() => !applyMutation.isPending && onClose()}
						disabled={applyMutation.isPending}
						className="text-secondary-500 hover:text-secondary-700 dark:text-white/60 dark:hover:text-white min-h-[44px] min-w-[44px] flex items-center justify-center"
						aria-label="Close"
					>
						<X className="h-5 w-5" />
					</button>
				</div>

				{/* Tier picker */}
				<div className="grid grid-cols-1 sm:grid-cols-3 gap-3 mb-5">
					{TIER_ORDER.map((tid) => {
						const t = TIERS[tid];
						const selected = targetTier === tid;
						const isCurrent = tid === currentTierId;
						return (
							<button
								key={tid}
								type="button"
								onClick={() => setTargetTier(tid)}
								className={`text-left rounded-lg border p-3 min-h-[44px] transition-colors ${
									selected
										? "border-primary-500 ring-2 ring-primary-500/30 bg-primary-50 dark:bg-primary-900/20"
										: "border-secondary-200 dark:border-secondary-600 bg-white dark:bg-secondary-800 hover:border-secondary-400 dark:hover:border-secondary-500"
								}`}
							>
								<div className="flex items-center justify-between mb-1">
									<span
										className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${t.badgeClass}`}
									>
										{t.name}
									</span>
									{isCurrent && (
										<span className="text-[10px] uppercase tracking-wider text-secondary-500 dark:text-white/60">
											Current
										</span>
									)}
								</div>
								<div className="text-sm font-semibold text-secondary-900 dark:text-white">
									{CURRENCY_SYMBOLS[currency] || ""}
									{(t.unitAmountCents / 100).toFixed(0)}/host/mo
								</div>
							</button>
						);
					})}
				</div>

				{/* Interval toggle */}
				<div className="mb-5">
					<div className="text-xs uppercase tracking-wider text-secondary-500 dark:text-white/60 mb-2">
						Billing interval
					</div>
					<div className="inline-flex rounded-md border border-secondary-200 dark:border-secondary-600 bg-secondary-50 dark:bg-secondary-800 p-1">
						{[
							{ id: "month", label: "Monthly" },
							{ id: "year", label: "Annual (save ~17%)" },
						].map((opt) => (
							<button
								key={opt.id}
								type="button"
								onClick={() => setTargetInterval(opt.id)}
								className={`px-3 py-1.5 text-sm rounded min-h-[36px] transition-colors ${
									targetInterval === opt.id
										? "bg-white dark:bg-secondary-700 shadow-sm text-secondary-900 dark:text-white font-medium"
										: "text-secondary-600 dark:text-white/70 hover:text-secondary-900 dark:hover:text-white"
								}`}
							>
								{opt.label}
							</button>
						))}
					</div>
				</div>

				{/* Pre-commit hosts (annual upgrades/renewals only). Hidden on
				    monthly and on downgrades — the new phase of a downgrade
				    starts with actual usage, so pre-commitments don't apply. */}
				{showCommitInput && (
					<div className="mb-5">
						<label
							htmlFor="commit-hosts-input"
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Pre-commit hosts (annual)
						</label>
						<input
							id="commit-hosts-input"
							type="number"
							inputMode="numeric"
							min={commitFloor}
							max={COMMIT_HOSTS_MAX}
							step={1}
							value={commitHosts}
							onChange={(e) => setCommitHosts(e.target.value)}
							className={`w-full sm:w-48 px-3 py-2 border rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white min-h-[44px] ${
								commitInvalid
									? "border-danger-400 dark:border-danger-600"
									: "border-secondary-300 dark:border-secondary-600"
							}`}
						/>
						{commitInvalid &&
							Number.isFinite(commitParsed) &&
							commitParsed < commitFloor && (
								<p className="mt-1 text-xs text-danger-600 dark:text-danger-400">
									Can't commit to fewer than your current {commitFloor} hosts.
								</p>
							)}
						{commitInvalid &&
							Number.isFinite(commitParsed) &&
							commitParsed > COMMIT_HOSTS_MAX && (
								<p className="mt-1 text-xs text-danger-600 dark:text-danger-400">
									Maximum commitment is {COMMIT_HOSTS_MAX.toLocaleString()}{" "}
									hosts.
								</p>
							)}
						<p className="mt-2 text-xs text-secondary-500 dark:text-white/70">
							Prepay for a fixed host count for the year. You're billed for this
							capacity even if actual usage is lower. Adding hosts above this
							will trigger a prorated charge against the remaining annual
							period.
						</p>
					</div>
				)}

				{/* Preview panel */}
				<PreviewPanel
					loading={preview.isLoading || preview.isFetching}
					data={previewData}
					errorMsg={previewErrMsg}
					isNoop={isNoop}
					currency={currency}
					interval={targetInterval}
				/>

				{/* Apply error banner */}
				{applyErrMsg && (
					<div className="mt-4 rounded-md border bg-danger-50 dark:bg-danger-900/20 border-danger-200 dark:border-danger-800 p-3 flex items-start gap-3">
						<AlertTriangle className="h-5 w-5 text-danger-600 dark:text-danger-400 flex-shrink-0 mt-0.5" />
						<div className="flex-1 min-w-0">
							<p className="text-sm font-medium text-danger-700 dark:text-danger-300">
								Could not apply the tier change
							</p>
							<p className="text-sm text-danger-700 dark:text-danger-300 mt-0.5">
								{applyErrMsg}
							</p>
						</div>
					</div>
				)}

				{/* Success banner */}
				{applied && (
					<div className="mt-4 rounded-md border bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800 p-3 flex items-start gap-3">
						<CheckCircle className="h-5 w-5 text-green-600 dark:text-green-400 flex-shrink-0 mt-0.5" />
						<div className="flex-1 min-w-0">
							<p className="text-sm font-medium text-green-700 dark:text-green-300">
								Plan updated successfully.
							</p>
						</div>
					</div>
				)}

				{/* Footer actions */}
				<div className="mt-5 flex flex-col-reverse sm:flex-row sm:justify-end gap-2">
					<button
						type="button"
						onClick={onClose}
						disabled={applyMutation.isPending}
						className="btn-outline min-h-[44px] disabled:opacity-50 disabled:cursor-not-allowed"
					>
						Cancel
					</button>
					<button
						type="button"
						onClick={() => applyMutation.mutate()}
						disabled={
							applyMutation.isPending ||
							applied ||
							isNoop ||
							!!previewErrMsg ||
							preview.isLoading ||
							commitInvalid
						}
						className="btn-primary min-h-[44px] flex items-center justify-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
					>
						{applyMutation.isPending ? (
							<>
								<RefreshCw className="h-4 w-4 animate-spin" />
								Applying...
							</>
						) : (
							<>
								{previewData?.direction === "downgrade" ? (
									<ArrowDown className="h-4 w-4" />
								) : (
									<ArrowUp className="h-4 w-4" />
								)}
								Confirm{" "}
								{previewData?.direction === "downgrade"
									? "downgrade"
									: "change"}
							</>
						)}
					</button>
				</div>
			</div>
		</div>
	);
};

// --- Preview panel --------------------------------------------------------

// PreviewPanel renders the plain-English explanation of what a tier change
// would cost and when it takes effect. Lives inside the modal; takes the
// preview response shape from POST /me/billing/tier-change/preview.
const PreviewPanel = ({
	loading,
	data,
	errorMsg,
	isNoop,
	currency,
	interval,
}) => {
	if (isNoop) {
		return (
			<div className="rounded-md border bg-secondary-50 dark:bg-secondary-800 border-secondary-200 dark:border-secondary-600 p-3 sm:p-4 text-sm text-secondary-700 dark:text-white/80">
				You're already on this tier and interval. Pick a different tier or
				interval to see the charge.
			</div>
		);
	}

	if (loading) {
		return (
			<div className="rounded-md border bg-secondary-50 dark:bg-secondary-800 border-secondary-200 dark:border-secondary-600 p-3 sm:p-4 flex items-center gap-3 text-sm text-secondary-600 dark:text-white/70">
				<RefreshCw className="h-4 w-4 animate-spin" />
				Calculating your new charge...
			</div>
		);
	}

	if (errorMsg) {
		return (
			<div className="rounded-md border bg-danger-50 dark:bg-danger-900/20 border-danger-200 dark:border-danger-800 p-3 sm:p-4 flex items-start gap-3">
				<AlertTriangle className="h-5 w-5 text-danger-600 dark:text-danger-400 flex-shrink-0 mt-0.5" />
				<div className="flex-1 min-w-0">
					<p className="text-sm font-medium text-danger-700 dark:text-danger-300">
						Preview unavailable
					</p>
					<p className="text-sm text-danger-700 dark:text-danger-300 mt-0.5">
						{errorMsg}
					</p>
				</div>
			</div>
		);
	}

	if (!data) {
		return null;
	}

	const fromName = TIERS[data.from_tier]?.name || capitalize(data.from_tier);
	const toName = TIERS[data.to_tier]?.name || capitalize(data.to_tier);
	const nextInvoiceStr = formatMoney(data.next_invoice_cents, currency);
	const nextInvoiceDate = formatDate(data.next_invoice_at) || "renewal";

	if (data.direction === "upgrade") {
		const proratedStr = formatMoney(data.prorated_invoice_cents, currency);
		const committed = Number(data.commit_hosts_applied || 0);
		const showCommit = interval === "year" && committed > 0;
		const unitCents =
			committed > 0
				? Math.round(Number(data.next_invoice_cents || 0) / committed)
				: 0;
		const unitStr = formatMoney(unitCents, currency);
		const totalStr = formatMoney(data.next_invoice_cents, currency);
		return (
			<div className="rounded-md border bg-primary-50 dark:bg-primary-900/20 border-primary-200 dark:border-primary-800 p-3 sm:p-4">
				<div className="flex items-center gap-2 mb-2">
					<ArrowUp className="h-4 w-4 text-primary-600 dark:text-primary-400" />
					<p className="text-sm font-medium text-primary-900 dark:text-primary-200">
						Upgrade: {fromName} {intervalLabel(data.from_interval)} to {toName}{" "}
						{intervalLabel(data.to_interval)}
					</p>
				</div>
				<ul className="text-sm text-primary-900 dark:text-primary-200 space-y-1 list-disc pl-5">
					{showCommit && (
						<li>
							Committing to{" "}
							<span className="font-semibold">{committed} hosts</span> ×{" "}
							<span className="font-semibold">{unitStr}/year</span> ={" "}
							<span className="font-semibold">{totalStr}/year</span> total.
						</li>
					)}
					<li>
						<span className="font-semibold">{proratedStr}</span> charged today
						(prorated for the remainder of your current billing cycle).
					</li>
					<li>
						Your next full invoice of{" "}
						<span className="font-semibold">{nextInvoiceStr}</span> lands on{" "}
						<span className="font-semibold">{nextInvoiceDate}</span>.
					</li>
					<li>The new tier is available immediately after confirmation.</li>
				</ul>
			</div>
		);
	}

	if (data.direction === "downgrade") {
		const effectiveDate = formatDate(data.effective_at_date) || nextInvoiceDate;
		return (
			<div className="rounded-md border bg-warning-50 dark:bg-warning-900/20 border-warning-200 dark:border-warning-800 p-3 sm:p-4">
				<div className="flex items-center gap-2 mb-2">
					<ArrowDown className="h-4 w-4 text-warning-600 dark:text-warning-400" />
					<p className="text-sm font-medium text-warning-800 dark:text-warning-200">
						Downgrade: {fromName} {intervalLabel(data.from_interval)} to{" "}
						{toName} {intervalLabel(data.to_interval)}
					</p>
				</div>
				<ul className="text-sm text-warning-800 dark:text-warning-200 space-y-1 list-disc pl-5">
					<li>
						No change to today's bill. You keep {fromName} until{" "}
						<span className="font-semibold">{effectiveDate}</span>.
					</li>
					<li>
						From <span className="font-semibold">{effectiveDate}</span> onwards
						you'll pay <span className="font-semibold">{nextInvoiceStr}</span>{" "}
						per {data.to_interval === "year" ? "year" : "month"}.
					</li>
					<li>
						Any {toName}-only features will remain active until {effectiveDate}.
					</li>
				</ul>
			</div>
		);
	}

	// Fallback: direction should only ever be upgrade or downgrade.
	return (
		<div className="rounded-md border bg-secondary-50 dark:bg-secondary-800 border-secondary-200 dark:border-secondary-600 p-3 sm:p-4 text-sm text-secondary-700 dark:text-white/80">
			Next invoice will be {nextInvoiceStr} on {nextInvoiceDate}.
		</div>
	);
};

// --- Tier matrix ----------------------------------------------------------

const TierMatrix = ({ currentTierId }) => {
	return (
		<div className="overflow-x-auto -mx-4 sm:mx-0">
			<div className="min-w-[640px] px-4 sm:px-0">
				<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
					<thead className="bg-secondary-50 dark:bg-secondary-700">
						<tr>
							<th className="px-3 sm:px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider whitespace-nowrap">
								Feature
							</th>
							{TIER_ORDER.map((tid) => {
								const t = TIERS[tid];
								const isCurrent = tid === currentTierId;
								return (
									<th
										key={tid}
										className={`px-3 sm:px-4 py-2 text-center text-xs font-medium uppercase tracking-wider whitespace-nowrap ${
											isCurrent
												? "text-primary-700 dark:text-primary-300"
												: "text-secondary-500 dark:text-white"
										}`}
									>
										<div className="flex items-center justify-center gap-2">
											<span>{t.name}</span>
											{isCurrent && (
												<span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium bg-primary-100 text-primary-800 dark:bg-primary-900 dark:text-primary-200">
													Current
												</span>
											)}
										</div>
										<div className="mt-1 text-[11px] normal-case tracking-normal text-secondary-400 dark:text-white/60 font-normal">
											{CURRENCY_SYMBOLS.gbp}
											{(t.unitAmountCents / 100).toFixed(0)}/host/mo
										</div>
									</th>
								);
							})}
						</tr>
					</thead>
					<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
						{TIER_FEATURES.map((row) => (
							<tr
								key={row.label}
								className="hover:bg-secondary-50 dark:hover:bg-secondary-700 transition-colors"
							>
								<td className="px-3 sm:px-4 py-2 text-sm text-secondary-900 dark:text-white">
									{row.label}
								</td>
								{TIER_ORDER.map((tid) => {
									const v = row[tid];
									const isCurrent = tid === currentTierId;
									return (
										<td
											key={tid}
											className={`px-3 sm:px-4 py-2 text-sm text-center whitespace-nowrap ${
												isCurrent
													? "bg-primary-50/40 dark:bg-primary-900/20"
													: ""
											}`}
										>
											<TierCell value={v} />
										</td>
									);
								})}
							</tr>
						))}
					</tbody>
				</table>
			</div>
		</div>
	);
};

const TierCell = ({ value }) => {
	if (value === true) {
		return (
			<span className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300">
				<Check className="h-3.5 w-3.5" />
			</span>
		);
	}
	if (value === false) {
		return (
			<span className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-secondary-100 text-secondary-400 dark:bg-secondary-700 dark:text-secondary-500">
				<Minus className="h-3.5 w-3.5" />
			</span>
		);
	}
	if (value == null) {
		return (
			<span className="text-secondary-300 dark:text-secondary-600">
				<X className="h-4 w-4 inline" />
			</span>
		);
	}
	return (
		<span className="text-sm font-medium text-secondary-900 dark:text-white">
			{value}
		</span>
	);
};

export default Billing;
