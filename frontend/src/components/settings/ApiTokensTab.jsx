import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertCircle,
	CheckCircle,
	Clock,
	Copy,
	Eye,
	EyeOff,
	Key,
	Plus,
	Trash2,
	X,
} from "lucide-react";
import { useEffect, useId, useState } from "react";
import api, { apiTokensAPI } from "../../utils/api";

const ApiTokensTab = () => {
	const [showCreateModal, setShowCreateModal] = useState(false);
	const [newTokenData, setNewTokenData] = useState(null); // { name, token } — shown once after creation
	const [showToken, setShowToken] = useState(false);
	const [copySuccess, setCopySuccess] = useState({});
	const [serverUrl, setServerUrl] = useState("");
	const queryClient = useQueryClient();
	const nameId = useId();
	const expiresAtId = useId();

	const [form, setForm] = useState({ name: "", expires_at: "" });

	// biome-ignore lint/correctness/useExhaustiveDependencies: Only run on mount
	useEffect(() => {
		api
			.get("/settings")
			.then((r) => setServerUrl(r.data.server_url || window.location.origin))
			.catch(() => setServerUrl(window.location.origin));
	}, []);

	const copyToClipboard = async (text, key) => {
		try {
			await navigator.clipboard.writeText(text);
		} catch {
			const el = document.createElement("textarea");
			el.value = text;
			document.body.appendChild(el);
			el.select();
			document.execCommand("copy");
			document.body.removeChild(el);
		}
		setCopySuccess((s) => ({ ...s, [key]: true }));
		setTimeout(
			() => setCopySuccess((s) => ({ ...s, [key]: false })),
			2000,
		);
	};

	const {
		data: tokens,
		isLoading,
		error,
	} = useQuery({
		queryKey: ["apiTokens"],
		queryFn: () => apiTokensAPI.list().then((r) => r.data),
	});

	const createMutation = useMutation({
		mutationFn: (data) => apiTokensAPI.create(data).then((r) => r.data),
		onSuccess: (data) => {
			const name = form.name.trim();
			queryClient.invalidateQueries(["apiTokens"]);
			setShowCreateModal(false);
			setForm({ name: "", expires_at: "" });
			setNewTokenData({ name, token: data.token });
			setShowToken(false);
		},
	});

	const revokeMutation = useMutation({
		mutationFn: (id) => apiTokensAPI.revoke(id),
		onSuccess: () => {
			queryClient.invalidateQueries(["apiTokens"]);
		},
	});

	const handleCreate = (e) => {
		e.preventDefault();
		createMutation.mutate({
			name: form.name.trim(),
			expires_at: form.expires_at || null,
		});
	};

	const formatDate = (dateStr) => {
		if (!dateStr) return "—";
		return new Date(dateStr).toLocaleString();
	};

	const closeCreatedModal = () => {
		setNewTokenData(null);
		setShowToken(false);
	};

	return (
		<div>
			{/* Header */}
			<div className="flex items-center justify-between mb-6">
				<div className="flex items-center">
					<Key className="h-6 w-6 text-primary-600 mr-3" />
					<div>
						<h2 className="text-xl font-semibold text-secondary-900 dark:text-white">
							API Tokens
						</h2>
						<p className="text-sm text-secondary-500 dark:text-secondary-400 mt-0.5">
							Long-lived tokens for automation (Ansible, CI/CD). Use{" "}
							<code className="bg-secondary-100 dark:bg-secondary-700 px-1 rounded text-xs">
								Authorization: Bearer &lt;token&gt;
							</code>{" "}
							to authenticate.
						</p>
					</div>
				</div>
				<button
					type="button"
					onClick={() => setShowCreateModal(true)}
					className="btn-primary flex items-center gap-2 w-full sm:w-auto justify-center sm:justify-start px-4 py-2 text-sm"
				>
					<Plus className="h-4 w-4" />
					New Token
				</button>
			</div>

			{/* Token list */}
			{isLoading ? (
				<div className="text-center py-8">
					<div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
				</div>
			) : error ? (
				<p className="text-red-600 dark:text-red-400 text-sm">
					Failed to load tokens.
				</p>
			) : tokens?.length === 0 ? (
				<div className="text-center py-12 text-secondary-500 dark:text-secondary-400">
					<Key className="h-10 w-10 mx-auto mb-3 opacity-40" />
					<p className="text-sm">
						No API tokens yet. Create one to get started.
					</p>
				</div>
			) : (
				<div className="space-y-3">
					{tokens?.map((t) => (
						<div
							key={t.id}
							className="border border-secondary-200 dark:border-secondary-600 rounded-lg p-4 hover:border-primary-300 dark:hover:border-primary-700 transition-colors"
						>
							<div className="flex justify-between items-start gap-3">
								<div className="flex-1 min-w-0">
									<div className="flex items-center gap-2 flex-wrap">
										<h4 className="font-medium text-secondary-900 dark:text-white">
											{t.name}
										</h4>
										{t.expires_at && new Date(t.expires_at) < new Date() ? (
											<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200">
												Expired
											</span>
										) : (
											<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
												Active
											</span>
										)}
									</div>
									<div className="mt-2 space-y-1 text-sm text-secondary-600 dark:text-secondary-400">
										<p>Created: {formatDate(t.created_at)}</p>
										<p>
											Expires:{" "}
											{t.expires_at ? (
												formatDate(t.expires_at)
											) : (
												<span className="italic">Never</span>
											)}
										</p>
										{t.last_used_at && (
											<p className="flex items-center gap-1">
												<Clock className="h-3 w-3" />
												Last used: {formatDate(t.last_used_at)}
											</p>
										)}
									</div>
								</div>
								<button
									type="button"
									onClick={() => {
										if (
											window.confirm(
												`Revoke token "${t.name}"? This action cannot be undone.`,
											)
										) {
											revokeMutation.mutate(t.id);
										}
									}}
									disabled={revokeMutation.isPending}
									className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-red-600 dark:text-red-400 border border-red-200 dark:border-red-700 rounded hover:bg-red-50 dark:hover:bg-red-900/30 transition-colors disabled:opacity-50 flex-shrink-0"
								>
									<Trash2 className="h-3.5 w-3.5" />
									Revoke
								</button>
							</div>
						</div>
					))}
				</div>
			)}

			{/* Create Token Modal */}
			{showCreateModal && (
				<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
					<div className="bg-white dark:bg-secondary-800 rounded-lg max-w-md w-full">
						<div className="p-4 md:p-6">
							<div className="flex items-center justify-between mb-5 gap-3">
								<h3 className="text-lg font-bold text-secondary-900 dark:text-white flex items-center gap-2">
									<Key className="h-5 w-5 text-primary-600" />
									Create API Token
								</h3>
								<button
									type="button"
									onClick={() => setShowCreateModal(false)}
									className="text-secondary-400 hover:text-secondary-600 dark:hover:text-secondary-200 flex-shrink-0"
								>
									<X className="h-5 w-5" />
								</button>
							</div>

							<form onSubmit={handleCreate} className="space-y-4">
								<div>
									<label
										htmlFor={nameId}
										className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1"
									>
										Token name *
									</label>
									<input
										id={nameId}
										type="text"
										required
										placeholder="e.g. ansible-prod"
										value={form.name}
										onChange={(e) =>
											setForm((f) => ({ ...f, name: e.target.value }))
										}
										className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-700 text-sm text-secondary-900 dark:text-white placeholder-secondary-400 focus:outline-none focus:ring-2 focus:ring-primary-500"
									/>
								</div>
								<div>
									<label
										htmlFor={expiresAtId}
										className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1"
									>
										Expiry date{" "}
										<span className="text-secondary-400 font-normal">
											(optional)
										</span>
									</label>
									<input
										id={expiresAtId}
										type="datetime-local"
										value={form.expires_at}
										onChange={(e) =>
											setForm((f) => ({ ...f, expires_at: e.target.value }))
										}
										className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-700 text-sm text-secondary-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
									/>
									<p className="mt-1 text-xs text-secondary-400">
										Leave empty for a non-expiring token.
									</p>
								</div>

								<div className="flex justify-end gap-3 pt-2">
									<button
										type="button"
										onClick={() => setShowCreateModal(false)}
										className="px-4 py-2 text-sm font-medium text-secondary-700 dark:text-secondary-300 border border-secondary-300 dark:border-secondary-500 rounded-lg hover:bg-secondary-50 dark:hover:bg-secondary-700 transition-colors"
									>
										Cancel
									</button>
									<button
										type="submit"
										disabled={createMutation.isPending || !form.name.trim()}
										className="btn-primary px-4 py-2 text-sm disabled:opacity-50"
									>
										{createMutation.isPending ? "Creating…" : "Create Token"}
									</button>
								</div>
							</form>
						</div>
					</div>
				</div>
			)}

			{/* Token Created Successfully Modal */}
			{newTokenData && (
				<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
					<div className="bg-white dark:bg-secondary-800 rounded-lg max-w-2xl w-full max-h-[90vh] overflow-y-auto">
						<div className="p-4 md:p-6">
							<div className="flex items-center justify-between mb-4 gap-3">
								<div className="flex items-center gap-2 min-w-0">
									<CheckCircle className="h-5 w-5 md:h-6 md:w-6 text-green-600 dark:text-green-400 flex-shrink-0" />
									<h2 className="text-base md:text-lg font-bold text-secondary-900 dark:text-white truncate">
										API Token Created Successfully
									</h2>
								</div>
								<button
									type="button"
									onClick={closeCreatedModal}
									className="text-secondary-400 hover:text-secondary-600 dark:hover:text-secondary-200 flex-shrink-0"
								>
									<X className="h-5 w-5" />
								</button>
							</div>

							<div className="bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg p-3 mb-4">
								<div className="flex items-center gap-2">
									<AlertCircle className="h-4 w-4 text-yellow-600 dark:text-yellow-400 flex-shrink-0" />
									<p className="text-xs text-yellow-800 dark:text-yellow-200">
										<strong>Important:</strong> Save this token — it won't be
										shown again.
									</p>
								</div>
							</div>

							<div className="space-y-3">
								<div>
									<label className="block text-xs font-medium text-secondary-700 dark:text-secondary-300 mb-1">
										Token Name
									</label>
									<input
										type="text"
										value={newTokenData.name}
										readOnly
										className="w-full px-3 py-2 text-sm border border-secondary-300 dark:border-secondary-600 rounded-md bg-secondary-50 dark:bg-secondary-900 text-secondary-900 dark:text-white font-mono"
									/>
								</div>

								<div>
									<label className="block text-xs font-medium text-secondary-700 dark:text-secondary-300 mb-1">
										Token
									</label>
									<div className="flex items-center gap-2">
										<input
											type={showToken ? "text" : "password"}
											value={newTokenData.token}
											readOnly
											className="flex-1 px-3 py-2 text-xs md:text-sm border border-secondary-300 dark:border-secondary-600 rounded-md bg-secondary-50 dark:bg-secondary-900 text-secondary-900 dark:text-white font-mono break-all"
										/>
										<button
											type="button"
											onClick={() => setShowToken((s) => !s)}
											className="p-2 text-secondary-600 hover:text-secondary-800 dark:text-secondary-400 flex-shrink-0"
											title={showToken ? "Hide token" : "Show token"}
										>
											{showToken ? (
												<EyeOff className="h-4 w-4" />
											) : (
												<Eye className="h-4 w-4" />
											)}
										</button>
										<button
											type="button"
											onClick={() =>
												copyToClipboard(newTokenData.token, "new-token")
											}
											className="btn-primary p-2 flex-shrink-0"
											title="Copy Token"
										>
											{copySuccess["new-token"] ? (
												<CheckCircle className="h-4 w-4" />
											) : (
												<Copy className="h-4 w-4" />
											)}
										</button>
									</div>
								</div>

								<div className="mt-4">
									<div className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-2">
										Usage Example
									</div>
									<p className="text-xs text-secondary-600 dark:text-secondary-400 mb-2">
										Pass this token in the Authorization header:
									</p>
									<div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
										<input
											type="text"
											value={`curl -s -H "Authorization: Bearer ${newTokenData.token}" ${serverUrl}/api/v1/host-groups`}
											readOnly
											className="flex-1 px-3 py-2 text-xs border border-secondary-300 dark:border-secondary-600 rounded-md bg-secondary-50 dark:bg-secondary-900 text-secondary-900 dark:text-white font-mono break-all"
										/>
										<button
											type="button"
											onClick={() =>
												copyToClipboard(
													`curl -s -H "Authorization: Bearer ${newTokenData.token}" ${serverUrl}/api/v1/host-groups`,
													"usage-curl",
												)
											}
											className="btn-primary flex items-center justify-center gap-1 px-3 py-2 whitespace-nowrap"
										>
											{copySuccess["usage-curl"] ? (
												<>
													<CheckCircle className="h-4 w-4" />
													Copied
												</>
											) : (
												<>
													<Copy className="h-4 w-4" />
													Copy
												</>
											)}
										</button>
									</div>
									<p className="text-xs text-secondary-500 dark:text-secondary-400 mt-3">
										💡 Accepted on all host-group and host-group assignment
										endpoints.
									</p>
								</div>
							</div>

							<div className="mt-4 pt-4 border-t border-secondary-200 dark:border-secondary-600">
								<button
									type="button"
									onClick={closeCreatedModal}
									className="w-full btn-primary py-2 px-4 rounded-md text-sm md:text-base"
								>
									I've Saved the Token
								</button>
							</div>
						</div>
					</div>
				</div>
			)}
		</div>
	);
};

export default ApiTokensTab;
