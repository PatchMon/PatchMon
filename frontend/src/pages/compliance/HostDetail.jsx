import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowLeft,
	ExternalLink,
	Play,
	RefreshCw,
	Shield,
} from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import HostComplianceDetail from "../../components/compliance/HostComplianceDetail";
import { dashboardAPI } from "../../utils/api";
import { complianceAPI } from "../../utils/complianceApi";

const ComplianceHostDetail = () => {
	const { id } = useParams();
	const queryClient = useQueryClient();

	const triggerScanMutation = useMutation({
		mutationFn: () => complianceAPI.triggerScan(id, { profile_type: "all" }),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["compliance-latest", id] });
		},
	});

	// Fetch host details
	const {
		data: host,
		isLoading,
		error,
	} = useQuery({
		queryKey: ["compliance-host", id],
		queryFn: () => dashboardAPI.getHostDetail(id).then((res) => res.data),
		staleTime: 5 * 60 * 1000,
		refetchOnWindowFocus: false,
	});

	// WebSocket connection status using polling (secure - uses httpOnly cookies)
	const [ws_status, set_ws_status] = useState(null);

	useEffect(() => {
		if (!host?.api_id) return;

		let is_mounted = true;

		const fetch_status = async () => {
			try {
				const response = await fetch(`/api/v1/ws/status/${host.api_id}`, {
					credentials: "include",
				});
				if (response.ok && is_mounted) {
					const result = await response.json();
					set_ws_status(result.data);
				}
			} catch (_err) {
				// Silently handle errors
			}
		};

		fetch_status();
		const poll_interval = setInterval(fetch_status, 5000);

		return () => {
			is_mounted = false;
			clearInterval(poll_interval);
		};
	}, [host?.api_id]);

	if (isLoading) {
		return (
			<div className="flex items-center justify-center min-h-screen">
				<RefreshCw className="h-8 w-8 animate-spin text-secondary-400" />
			</div>
		);
	}

	if (error || !host) {
		return (
			<div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
				<div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4">
					<div className="flex">
						<AlertTriangle className="h-5 w-5 text-red-400" />
						<div className="ml-3">
							<h3 className="text-sm font-medium text-red-800 dark:text-red-200">
								Host not found
							</h3>
						</div>
					</div>
				</div>
				<Link
					to="/compliance"
					className="mt-4 inline-flex items-center text-primary-600 hover:text-primary-900"
				>
					<ArrowLeft className="h-4 w-4 mr-2" />
					Back to Security Compliance
				</Link>
			</div>
		);
	}

	return (
		<div className="space-y-6">
			{/* Header */}
			<div>
				<Link
					to="/compliance"
					className="inline-flex items-center text-sm text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 mb-4"
				>
					<ArrowLeft className="h-4 w-4 mr-2" />
					Back to Security Compliance
				</Link>
				<div className="flex items-start justify-between">
					<div className="flex items-center">
						<Shield className="h-8 w-8 text-primary-500 mr-3" />
						<div>
							<div className="flex items-center gap-3">
								<h1 className="text-2xl font-bold text-secondary-900 dark:text-white">
									{host.friendly_name || host.hostname}
								</h1>
								<button
									type="button"
									onClick={() => triggerScanMutation.mutate()}
									disabled={
										triggerScanMutation.isPending || !ws_status?.connected
									}
									className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-white bg-green-600 hover:bg-green-700 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
									title={
										!ws_status?.connected
											? "Host is disconnected"
											: triggerScanMutation.isPending
												? "Scan in progress…"
												: "Run compliance scan now"
									}
								>
									{triggerScanMutation.isPending ? (
										<RefreshCw className="h-4 w-4 animate-spin" />
									) : (
										<Play className="h-4 w-4" />
									)}
									{triggerScanMutation.isPending ? "Scanning…" : "Run Scan"}
								</button>
							</div>
							<p className="mt-1 text-sm text-secondary-600 dark:text-secondary-400">
								{host.ip} &middot; Compliance Overview
							</p>
						</div>
					</div>
					<div className="flex items-center gap-3">
						{/* Connection Status */}
						<span
							className={`inline-flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-full ${
								ws_status?.connected
									? "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300"
									: "bg-secondary-100 text-secondary-600 dark:bg-secondary-700 dark:text-secondary-400"
							}`}
						>
							<span
								className={`h-2 w-2 rounded-full ${
									ws_status?.connected ? "bg-green-500" : "bg-secondary-400"
								}`}
							/>
							{ws_status?.connected ? "Connected" : "Disconnected"}
						</span>
						<Link
							to={`/hosts/${id}`}
							className="inline-flex items-center text-sm text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300"
						>
							View Full Host Details
							<ExternalLink className="ml-2 h-4 w-4" />
						</Link>
					</div>
				</div>
			</div>

			{/* Compliance Content */}
			<HostComplianceDetail hostId={id} />
		</div>
	);
};

export default ComplianceHostDetail;
