/**
 * TFA setup flow for the first-time wizard.
 * Shows QR code, verification, and backup codes. Calls onComplete when done.
 */
import { useMutation } from "@tanstack/react-query";
import { Copy, Download } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { tfaAPI } from "../utils/api";

const copyToClipboard = async (text) => {
	try {
		if (navigator.clipboard && window.isSecureContext) {
			await navigator.clipboard.writeText(text);
			return;
		}
		const textArea = document.createElement("textarea");
		textArea.value = text;
		textArea.style.position = "fixed";
		textArea.style.left = "-999999px";
		document.body.appendChild(textArea);
		textArea.focus();
		textArea.select();
		document.execCommand("copy");
		document.body.removeChild(textArea);
	} catch {
		prompt("Copy this text:", text);
	}
};

const downloadBackupCodes = (codes) => {
	const content = `PatchMon Backup Codes\n\n${codes.map((code, i) => `${i + 1}. ${code}`).join("\n")}\n\nKeep these codes safe! Each code can only be used once.`;
	const blob = new Blob([content], { type: "text/plain" });
	const url = URL.createObjectURL(blob);
	const a = document.createElement("a");
	a.href = url;
	a.download = "patchmon-backup-codes.txt";
	document.body.appendChild(a);
	a.click();
	document.body.removeChild(a);
	URL.revokeObjectURL(url);
};

const WizardTfaSetup = ({ onComplete }) => {
	const verificationTokenId = useId();
	const [setupStep, setSetupStep] = useState("setup"); // 'setup' | 'verify' | 'backup-codes'
	const [verificationToken, setVerificationToken] = useState("");
	const [backupCodes, setBackupCodes] = useState([]);
	const [message, setMessage] = useState({ type: "", text: "" });

	const setupMutation = useMutation({
		mutationFn: () => tfaAPI.setup().then((res) => res.data),
		onSuccess: () => {
			setSetupStep("setup");
			setMessage({
				type: "info",
				text: "Scan the QR code with your authenticator app and enter the verification code below.",
			});
		},
		onError: (error) => {
			setMessage({
				type: "error",
				text: error.response?.data?.error || "Failed to setup MFA",
			});
		},
	});

	const verifyMutation = useMutation({
		mutationFn: (data) => tfaAPI.verifySetup(data).then((res) => res.data),
		onSuccess: (data) => {
			setBackupCodes(data.backupCodes);
			setSetupStep("backup-codes");
			setMessage({
				type: "success",
				text: "Two-factor authentication has been enabled successfully!",
			});
		},
		onError: (error) => {
			setMessage({
				type: "error",
				text: error.response?.data?.error || "Failed to verify MFA setup",
			});
		},
	});

	const handleSetup = () => {
		setMessage({ type: "", text: "" });
		setupMutation.mutate();
	};

	const handleVerify = (e) => {
		e.preventDefault();
		if (verificationToken.length !== 6) {
			setMessage({
				type: "error",
				text: "Please enter a 6-digit verification code",
			});
			return;
		}
		verifyMutation.mutate({ token: verificationToken });
	};

	const handleDone = () => {
		onComplete();
	};

	// Trigger setup on mount (once)
	useEffect(() => {
		setupMutation.mutate();
	}, [setupMutation.mutate]);

	// Initial: waiting for setup
	if (
		!setupMutation.data &&
		!setupMutation.isPending &&
		!setupMutation.isError
	) {
		return (
			<div className="flex justify-center py-8">
				<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
			</div>
		);
	}

	if (setupMutation.isPending) {
		return (
			<div className="flex justify-center py-8">
				<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
			</div>
		);
	}

	if (setupMutation.isError) {
		return (
			<div className="space-y-4">
				<div className="rounded-md p-4 bg-danger-50 dark:bg-danger-900 border border-danger-200 dark:border-danger-700">
					<p className="text-sm text-danger-700 dark:text-danger-300">
						{message.text}
					</p>
				</div>
				<button type="button" onClick={handleSetup} className="btn-primary">
					Retry
				</button>
			</div>
		);
	}

	return (
		<div className="space-y-6">
			{message.text && (
				<div
					className={`rounded-md p-4 ${
						message.type === "success"
							? "bg-green-50 dark:bg-green-900 border border-green-200 dark:border-green-700"
							: message.type === "error"
								? "bg-danger-50 dark:bg-danger-900 border border-danger-200 dark:border-danger-700"
								: "bg-blue-50 dark:bg-blue-900 border border-blue-200 dark:border-blue-700"
					}`}
				>
					<p className="text-sm">{message.text}</p>
				</div>
			)}

			{/* QR Code & Manual Entry */}
			{setupStep === "setup" && setupMutation.data && (
				<div className="space-y-4">
					<div className="text-center">
						<img
							src={setupMutation.data.qrCode}
							alt="QR Code"
							className="mx-auto h-40 w-40 border border-secondary-200 dark:border-secondary-600 rounded-lg"
						/>
						<p className="text-sm text-secondary-600 dark:text-secondary-400 mt-2">
							Scan this QR code with your authenticator app
						</p>
					</div>
					<div className="bg-secondary-50 dark:bg-secondary-700 p-3 rounded-lg">
						<p className="text-sm font-medium text-secondary-900 dark:text-white mb-2">
							Manual Entry Key:
						</p>
						<div className="flex items-center gap-2">
							<code className="flex-1 bg-white dark:bg-secondary-800 px-3 py-2 rounded border text-xs font-mono break-all">
								{setupMutation.data.manualEntryKey}
							</code>
							<button
								type="button"
								onClick={() =>
									copyToClipboard(setupMutation.data.manualEntryKey)
								}
								className="p-2 text-secondary-400 hover:text-secondary-600 dark:hover:text-secondary-300 flex-shrink-0"
								title="Copy"
							>
								<Copy className="h-4 w-4" />
							</button>
						</div>
					</div>
					<button
						type="button"
						onClick={() => setSetupStep("verify")}
						className="btn-primary w-full"
					>
						Continue to Verification
					</button>
				</div>
			)}

			{/* Verification */}
			{setupStep === "verify" && (
				<form onSubmit={handleVerify} className="space-y-4">
					<div>
						<label
							htmlFor={verificationTokenId}
							className="block text-sm font-medium text-secondary-700 dark:text-white mb-1"
						>
							Verification Code
						</label>
						<input
							id={verificationTokenId}
							type="text"
							value={verificationToken}
							onChange={(e) =>
								setVerificationToken(
									e.target.value.replace(/\D/g, "").slice(0, 6),
								)
							}
							placeholder="000000"
							className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md focus:ring-2 focus:ring-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white text-center text-lg font-mono tracking-widest"
							maxLength={6}
							required
						/>
					</div>
					<button
						type="submit"
						disabled={
							verifyMutation.isPending || verificationToken.length !== 6
						}
						className="btn-primary w-full"
					>
						{verifyMutation.isPending ? "Verifying..." : "Verify & Enable"}
					</button>
				</form>
			)}

			{/* Backup Codes */}
			{setupStep === "backup-codes" && backupCodes.length > 0 && (
				<div className="space-y-4">
					<p className="text-sm text-secondary-600 dark:text-secondary-400">
						Save these backup codes in a safe place. Each code can only be used
						once.
					</p>
					<div className="bg-secondary-50 dark:bg-secondary-700 p-4 rounded-lg">
						<div className="grid grid-cols-2 gap-2 font-mono text-sm">
							{backupCodes.map((code, index) => (
								<div key={code} className="flex justify-between py-1">
									<span className="text-secondary-600 dark:text-secondary-400">
										{index + 1}.
									</span>
									<span className="text-secondary-900 dark:text-white break-all ml-2">
										{code}
									</span>
								</div>
							))}
						</div>
					</div>
					<div className="flex gap-3">
						<button
							type="button"
							onClick={() => downloadBackupCodes(backupCodes)}
							className="btn-secondary flex-1"
						>
							<Download className="h-4 w-4 mr-2 inline" />
							Download Codes
						</button>
						<button
							type="button"
							onClick={handleDone}
							className="btn-primary flex-1"
						>
							Continue
						</button>
					</div>
				</div>
			)}
		</div>
	);
};

export default WizardTfaSetup;
