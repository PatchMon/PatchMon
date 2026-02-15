import { useState } from "react";
import { ExternalLink, Loader2, Monitor, X } from "lucide-react";
import { authAPI } from "../utils/api";

/**
 * RDP viewer for Windows hosts using Apache Guacamole.
 * Embeds the official Guacamole web client in an iframe - the standard,
 * production-ready approach used by Guacamole deployments.
 */
const RdpViewer = ({ host, isOpen, onClose, embedded = false }) => {
	const [username, setUsername] = useState("");
	const [password, setPassword] = useState("");
	const [domain, setDomain] = useState("");
	const [loading, setLoading] = useState(false);
	const [error, setError] = useState(null);
	const [guacamoleUrl, setGuacamoleUrl] = useState(null);

	const handleDisconnect = () => setGuacamoleUrl(null);

	const handleOpenInNewTab = () => {
		if (guacamoleUrl) {
			window.open(guacamoleUrl, "_blank", "noopener noreferrer");
			handleDisconnect();
		}
	};

	const handleSubmit = async (e) => {
		e.preventDefault();
		setError(null);
		setLoading(true);
		try {
			const res = await authAPI.rdpTicket(
				host.id,
				username.trim(),
				password,
				domain.trim(),
			);
			const data = res.data;

			if (!embedded) {
				window.open(data.guacamoleUrl, "_blank", "noopener");
				setLoading(false);
				return;
			}

			if (data.guacamoleUrl) {
				setGuacamoleUrl(data.guacamoleUrl);
				setError(null);
			} else {
				setError("RDP ticket missing required data");
			}
		} catch (err) {
			const msg =
				err.response?.data?.error ||
				err.response?.data?.message ||
				err.message ||
				"Failed to start RDP session";
			setError(msg);
		} finally {
			setLoading(false);
		}
	};

	if (!host || !isOpen) return null;

	// Embedded: show Guacamole web client in iframe
	if (embedded && guacamoleUrl) {
		return (
			<div
				className="bg-secondary-900 rounded-lg w-full flex flex-col overflow-hidden"
				style={{
					height: "480px",
					maxHeight: "calc(100vh - 320px)",
				}}
			>
				<div className="flex justify-between items-center px-4 py-2 border-b border-secondary-700 flex-shrink-0">
					<div className="flex items-center gap-2">
						<Monitor className="h-4 w-4 text-primary-400" />
						<span className="text-sm font-medium text-white">
							{host?.friendly_name || host?.ip || host?.hostname}
						</span>
						{host?.ip && (
							<span className="text-xs text-secondary-400">({host.ip})</span>
						)}
						<span className="px-2 py-0.5 text-xs font-medium bg-green-900 text-green-200 rounded">
							Connected
						</span>
					</div>
					<div className="flex gap-2">
						<button
							type="button"
							onClick={handleOpenInNewTab}
							className="px-2 py-0.5 text-xs font-medium text-secondary-300 hover:text-white hover:bg-secondary-700 rounded transition-colors inline-flex items-center gap-1"
							title="Open in new tab (replaces embedded view)"
						>
							<ExternalLink className="h-3.5 w-3.5" />
							Open in new tab
						</button>
						<button
							type="button"
							onClick={handleDisconnect}
							className="px-2 py-0.5 text-xs font-medium text-secondary-300 hover:text-white hover:bg-secondary-700 rounded transition-colors"
							title="Disconnect"
						>
							Disconnect
						</button>
						<button
							type="button"
							onClick={onClose}
							className="p-1.5 rounded text-secondary-500 hover:bg-secondary-100 dark:hover:bg-secondary-700"
							aria-label="Close"
						>
							<X className="h-4 w-4" />
						</button>
					</div>
				</div>
				<iframe
					src={guacamoleUrl}
					title="Remote Desktop"
					className="flex-1 w-full border-0 bg-secondary-950"
					allow="clipboard-read; clipboard-write"
				/>
			</div>
		);
	}

	// Credential form
	return (
		<div className="space-y-4">
			{embedded && (
				<div className="flex justify-between items-center">
					<span className="text-sm text-secondary-600 dark:text-secondary-400 flex items-center gap-1">
						<Monitor className="h-4 w-4" />
						Remote Desktop
					</span>
					<button
						type="button"
						onClick={onClose}
						className="p-1.5 rounded text-secondary-500 hover:bg-secondary-100 dark:hover:bg-secondary-700"
						aria-label="Close"
					>
						<X className="h-4 w-4" />
					</button>
				</div>
			)}
			<form onSubmit={handleSubmit} className="space-y-4">
				{error && (
					<div className="rounded-md p-3 bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 text-red-800 dark:text-red-200 text-sm">
						{error}
					</div>
				)}
				<div>
					<label
						htmlFor="rdp-username"
						className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1"
					>
						Username
					</label>
					<input
						id="rdp-username"
						type="text"
						value={username}
						onChange={(e) => setUsername(e.target.value)}
						required
						autoComplete="username"
						className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-lg bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white"
						placeholder="Administrator"
					/>
				</div>
				<div>
					<label
						htmlFor="rdp-password"
						className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1"
					>
						Password
					</label>
					<input
						id="rdp-password"
						type="password"
						value={password}
						onChange={(e) => setPassword(e.target.value)}
						required
						autoComplete="current-password"
						className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-lg bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white"
					/>
				</div>
				<div>
					<label
						htmlFor="rdp-domain"
						className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1"
					>
						Domain (optional)
					</label>
					<input
						id="rdp-domain"
						type="text"
						value={domain}
						onChange={(e) => setDomain(e.target.value)}
						autoComplete="domain"
						className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-lg bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white"
						placeholder="WORKGROUP or computer name for local"
					/>
				</div>
				<p className="text-xs text-secondary-500 dark:text-secondary-400">
					Local: Domain = computer name, Username = your username.
				</p>
				<button
					type="submit"
					disabled={loading}
					className="px-4 py-2 bg-primary-600 hover:bg-primary-700 disabled:bg-primary-400 text-white rounded-lg text-sm font-medium flex items-center gap-2"
				>
					{loading && <Loader2 className="h-4 w-4" />}
					{loading ? "Connecting..." : "Connect"}
				</button>
			</form>
		</div>
	);
};

export default RdpViewer;
