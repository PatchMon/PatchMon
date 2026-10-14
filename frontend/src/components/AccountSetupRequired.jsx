import { KeyRound, LogOut, RefreshCw, ShieldCheck } from "lucide-react";
import { useId, useState } from "react";
import { useAuth } from "../contexts/AuthContext";
import WizardTfaSetup from "./WizardTfaSetup";

const INPUT_CLASS =
	"w-full px-3 py-2 bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-primary-500 placeholder-secondary-400";

const PasswordStep = ({ onChanged }) => {
	const { changePassword } = useAuth();
	const currentId = useId();
	const newId = useId();
	const confirmId = useId();
	const [current, setCurrent] = useState("");
	const [next, setNext] = useState("");
	const [confirm, setConfirm] = useState("");
	const [error, setError] = useState("");
	const [saving, setSaving] = useState(false);

	const handleSubmit = async (e) => {
		e.preventDefault();
		setError("");
		if (!current || !next) {
			setError("Enter your temporary password and a new password");
			return;
		}
		if (next !== confirm) {
			setError("The new passwords do not match");
			return;
		}
		setSaving(true);
		const result = await changePassword(current, next);
		setSaving(false);
		if (!result.success) {
			setError(result.error || "Failed to change password");
			return;
		}
		await onChanged();
	};

	const field = (id, label, value, setValue, autoComplete) => (
		<div>
			<label
				htmlFor={id}
				className="block text-sm font-medium text-secondary-700 dark:text-white mb-1"
			>
				{label}
			</label>
			<input
				id={id}
				type="password"
				autoComplete={autoComplete}
				value={value}
				onChange={(e) => setValue(e.target.value)}
				className={INPUT_CLASS}
			/>
		</div>
	);

	return (
		<form onSubmit={handleSubmit} className="space-y-4">
			{field(
				currentId,
				"Temporary password",
				current,
				setCurrent,
				"current-password",
			)}
			{field(newId, "New password", next, setNext, "new-password")}
			<p className="-mt-2 text-xs text-secondary-500 dark:text-secondary-300">
				Choose a password you haven't used before. Your previous password and
				the temporary one are not accepted.
			</p>
			{field(
				confirmId,
				"Confirm new password",
				confirm,
				setConfirm,
				"new-password",
			)}
			{error && (
				<p
					role="alert"
					className="text-sm text-danger-600 dark:text-danger-400"
				>
					{error}
				</p>
			)}
			<button
				type="submit"
				disabled={saving}
				className="btn-primary w-full min-h-[44px] md:min-h-0 disabled:opacity-50 disabled:cursor-not-allowed"
			>
				{saving && <RefreshCw className="h-4 w-4 mr-2 animate-spin" />}
				{saving ? "Saving..." : "Set new password"}
			</button>
		</form>
	);
};

const AccountSetupRequired = () => {
	const { user, logout, refreshAccountSetup } = useAuth();
	const [error, setError] = useState("");
	const needsPassword = !!user?.password_change_required;

	const handleTfaComplete = async () => {
		setError("");
		if (!(await refreshAccountSetup())) {
			setError("Could not finish signing in. Please sign in again.");
		}
	};

	const Icon = needsPassword ? KeyRound : ShieldCheck;

	return (
		<div className="min-h-screen bg-gradient-to-br from-primary-50 to-secondary-50 dark:from-secondary-900 dark:to-secondary-800 flex items-center justify-center px-4 py-8">
			<div className="card w-full max-w-md p-6 space-y-6">
				<div className="flex items-start gap-3">
					<div className="w-10 h-10 bg-primary-100 dark:bg-primary-900 rounded-full flex items-center justify-center flex-shrink-0">
						<Icon className="h-5 w-5 text-primary-600 dark:text-primary-400" />
					</div>
					<div>
						<h1 className="text-lg font-medium text-secondary-900 dark:text-white">
							{needsPassword
								? "Choose a new password"
								: "Set up two-factor authentication"}
						</h1>
						<p className="mt-1 text-sm text-secondary-600 dark:text-secondary-300">
							{needsPassword
								? `An administrator set a temporary password on ${user?.username || "your account"}. Replace it with a password only you know to continue.`
								: `An administrator reset two-factor authentication on ${user?.username || "your account"}. Set it up again with your authenticator app to continue.`}
						</p>
					</div>
				</div>

				{needsPassword ? (
					<PasswordStep onChanged={refreshAccountSetup} />
				) : (
					<WizardTfaSetup onComplete={handleTfaComplete} />
				)}

				{error && (
					<p
						role="alert"
						className="text-sm text-danger-600 dark:text-danger-400"
					>
						{error}
					</p>
				)}

				<div className="pt-4 border-t border-secondary-200 dark:border-secondary-600 flex justify-center">
					<button
						type="button"
						onClick={logout}
						className="inline-flex items-center gap-2 text-sm text-secondary-500 hover:text-secondary-700 dark:text-secondary-300 dark:hover:text-white min-h-[44px]"
					>
						<LogOut className="h-4 w-4" />
						Sign out
					</button>
				</div>
			</div>
		</div>
	);
};

export default AccountSetupRequired;
