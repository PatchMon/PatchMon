import { useQuery } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowLeft,
	Container,
	ExternalLink,
	Package,
	RefreshCw,
	Server,
	Shield,
} from "lucide-react";
import { Link, useParams } from "react-router-dom";
import api, { formatRelativeTime } from "../../utils/api";

const ImageDetail = () => {
	const { id } = useParams();

	const { data, isLoading, error } = useQuery({
		queryKey: ["docker", "image", id],
		queryFn: async () => {
			const response = await api.get(`/docker/images/${id}`);
			return response.data;
		},
		refetchInterval: 30000,
	});

	const image = data?.image;
	const hosts = data?.hosts || [];
	const containers = image?.docker_containers || [];
	const updates = image?.docker_image_updates || [];

	if (isLoading) {
		return (
			<div className="flex items-center justify-center min-h-screen">
				<RefreshCw className="h-8 w-8 animate-spin text-secondary-400" />
			</div>
		);
	}

	if (error || !image) {
		return (
			<div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
				<div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4">
					<div className="flex">
						<AlertTriangle className="h-5 w-5 text-red-400" />
						<div className="ml-3">
							<h3 className="text-sm font-medium text-red-800 dark:text-red-200">
								Image not found
							</h3>
						</div>
					</div>
				</div>
				<Link
					to="/docker"
					className="mt-4 inline-flex items-center text-primary-600 hover:text-primary-900"
				>
					<ArrowLeft className="h-4 w-4 mr-2" />
					Back to Docker
				</Link>
			</div>
		);
	}

	return (
		<div className="space-y-6">
			<div>
				<Link
					to="/docker"
					className="inline-flex items-center text-sm text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 mb-4"
				>
					<ArrowLeft className="h-4 w-4 mr-2" />
					Back to Docker
				</Link>
				<div className="flex items-start justify-between">
					<div className="flex items-center">
						<Package className="h-8 w-8 text-secondary-400 mr-3" />
						<div>
							<h1 className="text-2xl font-bold text-secondary-900 dark:text-white">
								{image.repository}:{image.tag}
							</h1>
							<p className="mt-1 text-sm text-secondary-600 dark:text-white">
								Image ID: {image.image_id.substring(0, 12)}
							</p>
						</div>
					</div>
				</div>
			</div>

			{/* Overview Cards */}
			<div className="grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-4">
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
								{containers.length}
							</p>
						</div>
					</div>
				</div>

				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<Server className="h-5 w-5 text-purple-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Hosts
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{hosts.length}
							</p>
						</div>
					</div>
				</div>

				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<Package className="h-5 w-5 text-blue-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">Size</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{image.size_bytes ? (
									<>{(Number(image.size_bytes) / 1024 / 1024).toFixed(0)} MB</>
								) : (
									"N/A"
								)}
							</p>
						</div>
					</div>
				</div>

				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<AlertTriangle className="h-5 w-5 text-warning-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Updates
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{updates.length}
							</p>
						</div>
					</div>
				</div>
			</div>

			{/* Available Updates with Digest Comparison */}
			{updates.length > 0 && (
				<div className="bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg p-4">
					<div className="flex">
						<AlertTriangle className="h-5 w-5 text-yellow-400" />
						<div className="ml-3 flex-1">
							<h3 className="text-sm font-medium text-yellow-800 dark:text-yellow-200">
								Updates Available
							</h3>
							<div className="mt-2 space-y-3">
								{updates.map((update) => {
									let digestInfo = null;
									try {
										if (update.changelog_url) {
											digestInfo = JSON.parse(update.changelog_url);
										}
									} catch (_e) {
										// Ignore parse errors
									}

									return (
										<div
											key={update.id}
											className="bg-white dark:bg-secondary-800 rounded-lg p-3 border border-yellow-200 dark:border-yellow-700"
										>
											<div className="flex items-center justify-between mb-2">
												<div className="flex items-center gap-2">
													{update.is_security_update && (
														<Shield className="h-4 w-4 text-red-500" />
													)}
													<span className="text-sm font-medium text-yellow-800 dark:text-yellow-200">
														New version available:{" "}
														<span className="font-semibold">
															{update.available_tag}
														</span>
													</span>
												</div>
												{update.is_security_update && (
													<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200">
														Security
													</span>
												)}
											</div>
											{digestInfo &&
												digestInfo.method === "digest_comparison" && (
													<div className="mt-2 pt-2 border-t border-yellow-200 dark:border-yellow-700">
														<p className="text-xs text-secondary-600 dark:text-white mb-1">
															Detected via digest comparison:
														</p>
														<div className="font-mono text-xs space-y-1">
															<div className="text-red-600 dark:text-red-400">
																<span className="font-bold">- Current: </span>
																{digestInfo.current_digest}
															</div>
															<div className="text-green-600 dark:text-green-400">
																<span className="font-bold">+ Available: </span>
																{digestInfo.available_digest}
															</div>
														</div>
													</div>
												)}
										</div>
									);
								})}
							</div>
						</div>
					</div>
				</div>
			)}

			{/* Image Information */}
			<div className="card">
				<div className="px-6 py-5 border-b border-secondary-200 dark:border-secondary-700">
					<h3 className="text-lg leading-6 font-medium text-secondary-900 dark:text-white">
						Image Information
					</h3>
				</div>
				<div className="px-6 py-5 space-y-6">
					<div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Repository
							</dt>
							<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
								{image.repository}
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Tag
							</dt>
							<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
								{image.tag}
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Source
							</dt>
							<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
								{image.source}
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Created
							</dt>
							<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
								{image.created_at
									? formatRelativeTime(image.created_at)
									: "Unknown"}
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Image ID
							</dt>
							<dd className="mt-1 text-sm font-mono text-secondary-900 dark:text-white">
								{image.image_id}
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Last Checked
							</dt>
							<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
								{image.last_checked
									? formatRelativeTime(image.last_checked)
									: "Never"}
							</dd>
						</div>
						{image.digest && (
							<div className="sm:col-span-2">
								<dt className="text-sm font-medium text-secondary-500 dark:text-white">
									Digest
								</dt>
								<dd className="mt-1 text-sm font-mono text-secondary-900 dark:text-white break-all">
									{image.digest}
								</dd>
							</div>
						)}
					</div>
				</div>
			</div>

			{/* Containers using this image */}
			<div className="card">
				<div className="px-6 py-5 border-b border-secondary-200 dark:border-secondary-700">
					<h3 className="text-lg leading-6 font-medium text-secondary-900 dark:text-white">
						Containers ({containers.length})
					</h3>
				</div>
				<div className="overflow-x-auto">
					<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-700">
						<thead className="bg-secondary-50 dark:bg-secondary-900">
							<tr>
								<th
									scope="col"
									className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
								>
									Container Name
								</th>
								<th
									scope="col"
									className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
								>
									Status
								</th>
								<th
									scope="col"
									className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
								>
									Host
								</th>
								<th
									scope="col"
									className="px-6 py-3 text-right text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
								>
									Actions
								</th>
							</tr>
						</thead>
						<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-700">
							{containers.map((container) => (
								<tr key={container.id}>
									<td className="px-6 py-4 whitespace-nowrap">
										<Link
											to={`/docker/containers/${container.id}`}
											className="text-sm font-medium text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300"
										>
											{container.name}
										</Link>
									</td>
									<td className="px-6 py-4 whitespace-nowrap">
										<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200">
											{container.status}
										</span>
									</td>
									<td className="px-6 py-4 whitespace-nowrap text-sm text-secondary-500">
										{container.host_id}
									</td>
									<td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
										<Link
											to={`/docker/containers/${container.id}`}
											className="text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 inline-flex items-center"
										>
											View
											<ExternalLink className="ml-1 h-4 w-4" />
										</Link>
									</td>
								</tr>
							))}
						</tbody>
					</table>
				</div>
			</div>

			{/* Hosts using this image */}
			<div className="card">
				<div className="px-6 py-5 border-b border-secondary-200 dark:border-secondary-700">
					<h3 className="text-lg leading-6 font-medium text-secondary-900 dark:text-white">
						Hosts ({hosts.length})
					</h3>
				</div>
				<div className="overflow-x-auto">
					<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-700">
						<thead className="bg-secondary-50 dark:bg-secondary-900">
							<tr>
								<th
									scope="col"
									className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
								>
									Host Name
								</th>
								<th
									scope="col"
									className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
								>
									IP Address
								</th>
								<th
									scope="col"
									className="px-6 py-3 text-right text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
								>
									Actions
								</th>
							</tr>
						</thead>
						<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-700">
							{hosts.map((host) => (
								<tr key={host.id}>
									<td className="px-6 py-4 whitespace-nowrap">
										<Link
											to={`/hosts/${host.id}`}
											className="text-sm font-medium text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300"
										>
											{host.friendly_name || host.hostname}
										</Link>
									</td>
									<td className="px-6 py-4 whitespace-nowrap text-sm text-secondary-500">
										{host.ip}
									</td>
									<td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
										<Link
											to={`/hosts/${host.id}`}
											className="text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 inline-flex items-center"
										>
											View
											<ExternalLink className="ml-1 h-4 w-4" />
										</Link>
									</td>
								</tr>
							))}
						</tbody>
					</table>
				</div>
			</div>
		</div>
	);
};

export default ImageDetail;
