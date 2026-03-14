import { useQuery } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowLeft,
	CheckCircle,
	Container,
	Globe,
	Network,
	RefreshCw,
	Server,
	Tag,
	XCircle,
} from "lucide-react";
import { Link, useParams } from "react-router-dom";
import api, { formatRelativeTime } from "../../utils/api";

const NetworkDetail = () => {
	const { id } = useParams();

	const { data, isLoading, error } = useQuery({
		queryKey: ["docker", "network", id],
		queryFn: async () => {
			const response = await api.get(`/docker/networks/${id}`);
			return response.data;
		},
		refetchInterval: 30000,
	});

	const network = data?.network;
	const host = data?.host;

	if (isLoading) {
		return (
			<div className="flex items-center justify-center min-h-screen">
				<RefreshCw className="h-8 w-8 animate-spin text-secondary-400" />
			</div>
		);
	}

	if (error || !network) {
		return (
			<div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
				<div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4">
					<div className="flex">
						<AlertTriangle className="h-5 w-5 text-red-400" />
						<div className="ml-3">
							<h3 className="text-sm font-medium text-red-800 dark:text-red-200">
								Network not found
							</h3>
							<p className="mt-2 text-sm text-red-700 dark:text-red-300">
								The network you're looking for doesn't exist or has been
								removed.
							</p>
						</div>
					</div>
				</div>
				<Link
					to="/docker"
					className="mt-4 inline-flex items-center text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300"
				>
					<ArrowLeft className="h-4 w-4 mr-2" />
					Back to Docker
				</Link>
			</div>
		);
	}

	const BooleanBadge = ({ value, trueLabel = "Yes", falseLabel = "No" }) => {
		return value ? (
			<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
				<CheckCircle className="h-3 w-3 mr-1" />
				{trueLabel}
			</span>
		) : (
			<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200">
				<XCircle className="h-3 w-3 mr-1" />
				{falseLabel}
			</span>
		);
	};

	return (
		<div className="space-y-6">
			{/* Header */}
			<div>
				<Link
					to="/docker"
					className="inline-flex items-center text-sm text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 mb-4"
				>
					<ArrowLeft className="h-4 w-4 mr-2" />
					Back to Docker
				</Link>
				<div className="flex items-center">
					<Network className="h-8 w-8 text-secondary-400 mr-3" />
					<div>
						<h1 className="text-2xl font-bold text-secondary-900 dark:text-white">
							{network.name}
						</h1>
						<p className="mt-1 text-sm text-secondary-600 dark:text-white">
							Network ID: {network.network_id.substring(0, 12)}
						</p>
					</div>
				</div>
			</div>

			{/* Overview Cards */}
			<div className="grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-4">
				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<Network className="h-5 w-5 text-blue-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Driver
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{network.driver}
							</p>
						</div>
					</div>
				</div>

				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<Globe className="h-5 w-5 text-purple-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Scope
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{network.scope}
							</p>
						</div>
					</div>
				</div>

				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<Container className="h-5 w-5 text-green-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Containers
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{network.container_count || 0}
							</p>
						</div>
					</div>
				</div>

				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<RefreshCw className="h-5 w-5 text-secondary-400 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Last Checked
							</p>
							<p className="text-sm font-medium text-secondary-900 dark:text-white">
								{formatRelativeTime(network.last_checked)}
							</p>
						</div>
					</div>
				</div>
			</div>

			{/* Network Information Card */}
			<div className="card">
				<div className="px-6 py-5 border-b border-secondary-200 dark:border-secondary-700">
					<h3 className="text-lg leading-6 font-medium text-secondary-900 dark:text-white">
						Network Information
					</h3>
				</div>
				<div className="px-6 py-5">
					<dl className="grid grid-cols-1 gap-x-4 gap-y-6 sm:grid-cols-2">
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Network ID
							</dt>
							<dd className="mt-1 text-sm text-secondary-900 dark:text-white font-mono break-all">
								{network.network_id}
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Name
							</dt>
							<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
								{network.name}
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Driver
							</dt>
							<dd className="mt-1">
								<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">
									{network.driver}
								</span>
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Scope
							</dt>
							<dd className="mt-1">
								<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200">
									{network.scope}
								</span>
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Containers Attached
							</dt>
							<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
								{network.container_count || 0}
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								IPv6 Enabled
							</dt>
							<dd className="mt-1">
								<BooleanBadge value={network.ipv6_enabled} />
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Internal
							</dt>
							<dd className="mt-1">
								<BooleanBadge value={network.internal} />
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Attachable
							</dt>
							<dd className="mt-1">
								<BooleanBadge value={network.attachable} />
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Ingress
							</dt>
							<dd className="mt-1">
								<BooleanBadge value={network.ingress} />
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Config Only
							</dt>
							<dd className="mt-1">
								<BooleanBadge value={network.config_only} />
							</dd>
						</div>
						{network.created_at && (
							<div>
								<dt className="text-sm font-medium text-secondary-500 dark:text-white">
									Created
								</dt>
								<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
									{formatRelativeTime(network.created_at)}
								</dd>
							</div>
						)}
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Last Checked
							</dt>
							<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
								{formatRelativeTime(network.last_checked)}
							</dd>
						</div>
					</dl>
				</div>
			</div>

			{/* IPAM Configuration */}
			{network.ipam && (
				<div className="card">
					<div className="px-6 py-5 border-b border-secondary-200 dark:border-secondary-700">
						<h3 className="text-lg leading-6 font-medium text-secondary-900 dark:text-white">
							IPAM Configuration
						</h3>
						<p className="mt-1 text-sm text-secondary-500 dark:text-white">
							IP Address Management settings
						</p>
					</div>
					<div className="px-6 py-5">
						{network.ipam.driver && (
							<div className="mb-4">
								<dt className="text-sm font-medium text-secondary-500 dark:text-white mb-1">
									Driver
								</dt>
								<dd>
									<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">
										{network.ipam.driver}
									</span>
								</dd>
							</div>
						)}
						{network.ipam.config && network.ipam.config.length > 0 && (
							<div>
								<dt className="text-sm font-medium text-secondary-500 dark:text-white mb-3">
									Subnet Configuration
								</dt>
								<div className="space-y-4">
									{network.ipam.config.map((config, index) => (
										<div
											key={config.subnet || `config-${index}`}
											className="bg-secondary-50 dark:bg-secondary-900/50 rounded-lg p-4"
										>
											<dl className="grid grid-cols-1 gap-x-4 gap-y-3 sm:grid-cols-2">
												{config.subnet && (
													<div>
														<dt className="text-xs font-medium text-secondary-500 dark:text-white">
															Subnet
														</dt>
														<dd className="mt-1 text-sm text-secondary-900 dark:text-white font-mono">
															{config.subnet}
														</dd>
													</div>
												)}
												{config.gateway && (
													<div>
														<dt className="text-xs font-medium text-secondary-500 dark:text-white">
															Gateway
														</dt>
														<dd className="mt-1 text-sm text-secondary-900 dark:text-white font-mono">
															{config.gateway}
														</dd>
													</div>
												)}
												{config.ip_range && (
													<div>
														<dt className="text-xs font-medium text-secondary-500 dark:text-white">
															IP Range
														</dt>
														<dd className="mt-1 text-sm text-secondary-900 dark:text-white font-mono">
															{config.ip_range}
														</dd>
													</div>
												)}
												{config.aux_addresses &&
													Object.keys(config.aux_addresses).length > 0 && (
														<div className="sm:col-span-2">
															<dt className="text-xs font-medium text-secondary-500 dark:text-white mb-2">
																Auxiliary Addresses
															</dt>
															<dd className="space-y-1">
																{Object.entries(config.aux_addresses).map(
																	([key, value]) => (
																		<div
																			key={key}
																			className="flex items-center text-sm"
																		>
																			<span className="text-secondary-500 dark:text-white min-w-[120px]">
																				{key}:
																			</span>
																			<span className="text-secondary-900 dark:text-white font-mono">
																				{value}
																			</span>
																		</div>
																	),
																)}
															</dd>
														</div>
													)}
											</dl>
										</div>
									))}
								</div>
							</div>
						)}
						{network.ipam.options &&
							Object.keys(network.ipam.options).length > 0 && (
								<div className="mt-4">
									<dt className="text-sm font-medium text-secondary-500 dark:text-white mb-2">
										IPAM Options
									</dt>
									<dd className="space-y-1">
										{Object.entries(network.ipam.options).map(
											([key, value]) => (
												<div
													key={key}
													className="flex items-start py-2 border-b border-secondary-100 dark:border-secondary-700 last:border-0"
												>
													<span className="text-sm font-medium text-secondary-500 dark:text-white min-w-[200px]">
														{key}
													</span>
													<span className="text-sm text-secondary-900 dark:text-white break-all">
														{value}
													</span>
												</div>
											),
										)}
									</dd>
								</div>
							)}
					</div>
				</div>
			)}

			{/* Host Information */}
			{host && (
				<div className="card">
					<div className="px-6 py-5 border-b border-secondary-200 dark:border-secondary-700">
						<h3 className="text-lg leading-6 font-medium text-secondary-900 dark:text-white flex items-center">
							<Server className="h-5 w-5 mr-2" />
							Host Information
						</h3>
					</div>
					<div className="px-6 py-5">
						<dl className="grid grid-cols-1 gap-x-4 gap-y-6 sm:grid-cols-2">
							<div>
								<dt className="text-sm font-medium text-secondary-500 dark:text-white">
									Hostname
								</dt>
								<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
									<Link
										to={`/hosts/${host.id}`}
										className="text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300"
									>
										{host.hostname}
									</Link>
								</dd>
							</div>
							<div>
								<dt className="text-sm font-medium text-secondary-500 dark:text-white">
									Operating System
								</dt>
								<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
									{host.os_name} {host.os_version}
								</dd>
							</div>
						</dl>
					</div>
				</div>
			)}

			{/* Labels */}
			{network.labels && Object.keys(network.labels).length > 0 && (
				<div className="card">
					<div className="px-6 py-5 border-b border-secondary-200 dark:border-secondary-700">
						<h3 className="text-lg leading-6 font-medium text-secondary-900 dark:text-white flex items-center">
							<Tag className="h-5 w-5 mr-2" />
							Labels
						</h3>
					</div>
					<div className="px-6 py-5">
						<div className="space-y-2">
							{Object.entries(network.labels).map(([key, value]) => (
								<div
									key={key}
									className="flex items-start py-2 border-b border-secondary-100 dark:border-secondary-700 last:border-0"
								>
									<span className="text-sm font-medium text-secondary-500 dark:text-white min-w-[200px]">
										{key}
									</span>
									<span className="text-sm text-secondary-900 dark:text-white break-all">
										{value}
									</span>
								</div>
							))}
						</div>
					</div>
				</div>
			)}
		</div>
	);
};

export default NetworkDetail;
