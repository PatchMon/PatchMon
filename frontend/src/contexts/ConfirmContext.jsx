import { AlertTriangle, X } from "lucide-react";
import {
	createContext,
	useCallback,
	useContext,
	useEffect,
	useId,
	useRef,
	useState,
} from "react";
import { useLocation } from "react-router-dom";

const ConfirmContext = createContext(null);

const VARIANTS = {
	danger: {
		badge: "bg-danger-100 dark:bg-danger-900",
		icon: "text-danger-600 dark:text-danger-400",
		button: "btn-danger",
	},
	primary: {
		badge: "bg-primary-100 dark:bg-primary-900",
		icon: "text-primary-600 dark:text-primary-400",
		button: "btn-primary",
	},
};

const FOCUSABLE =
	'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

export function ConfirmProvider({ children }) {
	const [request, setRequest] = useState(null);
	const resolverRef = useRef(null);
	const seqRef = useRef(0);
	const { pathname } = useLocation();

	const settle = useCallback((result) => {
		setRequest(null);
		const resolve = resolverRef.current;
		resolverRef.current = null;
		resolve?.(result);
	}, []);

	const confirm = useCallback(
		(options) =>
			new Promise((resolve) => {
				// A second request supersedes the first; resolve the old one as
				// cancelled so its caller is never left awaiting forever.
				resolverRef.current?.(false);
				resolverRef.current = resolve;
				seqRef.current += 1;
				setRequest({ id: seqRef.current, options: options ?? {} });
			}),
		[],
	);

	// A stale dialog must not survive a route change and then fire a mutation
	// owned by an unmounted page. Compared against a ref so this settles only on
	// an actual navigation, not on mount.
	const lastPathRef = useRef(pathname);
	useEffect(() => {
		if (lastPathRef.current === pathname) return;
		lastPathRef.current = pathname;
		settle(false);
	}, [pathname, settle]);

	// Never leave a caller suspended if the provider itself goes away.
	useEffect(() => () => resolverRef.current?.(false), []);

	return (
		<ConfirmContext.Provider value={confirm}>
			{children}
			{request && (
				<ConfirmDialog
					key={request.id}
					options={request.options}
					onSettle={settle}
				/>
			)}
		</ConfirmContext.Provider>
	);
}

function ConfirmDialog({ options, onSettle }) {
	const {
		title = "Are you sure?",
		subtitle = "This action cannot be undone",
		message,
		warning,
		confirmLabel = "Delete",
		cancelLabel = "Cancel",
		variant = "danger",
	} = options;

	const styles = VARIANTS[variant] ?? VARIANTS.danger;
	const panelRef = useRef(null);
	const cancelRef = useRef(null);
	const titleId = useId();

	useEffect(() => {
		cancelRef.current?.focus();
	}, []);

	// `aria-modal` promises the background is inert, so the focus ring has to
	// actually stay inside the panel.
	const handleKeyDown = (e) => {
		if (e.key === "Escape") {
			onSettle(false);
			return;
		}
		if (e.key !== "Tab") return;

		const items = panelRef.current?.querySelectorAll(FOCUSABLE);
		if (!items?.length) return;
		const first = items[0];
		const last = items[items.length - 1];

		if (e.shiftKey && document.activeElement === first) {
			e.preventDefault();
			last.focus();
		} else if (!e.shiftKey && document.activeElement === last) {
			e.preventDefault();
			first.focus();
		}
	};

	return (
		<div
			className="fixed inset-0 bg-black/50 flex items-center justify-center z-[120]"
			onKeyDown={handleKeyDown}
			role="presentation"
		>
			<button
				type="button"
				onClick={() => onSettle(false)}
				className="fixed inset-0 cursor-default"
				tabIndex={-1}
				aria-hidden="true"
			/>
			<div
				ref={panelRef}
				role="dialog"
				aria-modal="true"
				aria-labelledby={titleId}
				className="bg-white dark:bg-secondary-800 rounded-lg shadow-xl max-w-md w-full mx-4 relative z-10"
			>
				<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-600">
					<div className="flex items-center justify-between gap-3">
						<div className="flex items-center gap-3 min-w-0">
							<div
								className={`w-10 h-10 ${styles.badge} rounded-full flex items-center justify-center flex-shrink-0`}
							>
								<AlertTriangle className={`h-5 w-5 ${styles.icon}`} />
							</div>
							<div className="min-w-0">
								<h3
									id={titleId}
									className="text-lg font-medium text-secondary-900 dark:text-white"
								>
									{title}
								</h3>
								{subtitle && (
									<p className="text-sm text-secondary-600 dark:text-white">
										{subtitle}
									</p>
								)}
							</div>
						</div>
						<button
							type="button"
							onClick={() => onSettle(false)}
							className="p-1 rounded hover:bg-secondary-100 dark:hover:bg-secondary-700 text-secondary-400 hover:text-secondary-600 flex-shrink-0"
							aria-label="Close"
						>
							<X className="h-5 w-5" />
						</button>
					</div>
				</div>

				<div className="px-6 py-4">
					{message && (
						<p className="text-secondary-700 dark:text-white">{message}</p>
					)}
					{warning && (
						<div className="mt-3 p-3 bg-danger-50 dark:bg-danger-900 border border-danger-200 dark:border-danger-700 rounded-md">
							<p className="text-sm text-danger-800 dark:text-danger-200">
								{warning}
							</p>
						</div>
					)}
				</div>

				<div className="px-6 py-4 border-t border-secondary-200 dark:border-secondary-600 flex justify-end gap-3">
					<button
						type="button"
						ref={cancelRef}
						onClick={() => onSettle(false)}
						className="btn-outline"
					>
						{cancelLabel}
					</button>
					<button
						type="button"
						onClick={() => onSettle(true)}
						className={styles.button}
					>
						{confirmLabel}
					</button>
				</div>
			</div>
		</div>
	);
}

export function useConfirm() {
	const context = useContext(ConfirmContext);
	if (!context) {
		throw new Error("useConfirm must be used within a ConfirmProvider");
	}
	return context;
}
