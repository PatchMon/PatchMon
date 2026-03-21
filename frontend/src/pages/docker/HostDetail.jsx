import { useQuery } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowLeft,
	Container,
	ExternalLink,
	Package,
	RefreshCw,
	Server,
} from "lucide-react";
import { Link, useParams } from "react-router-dom";
import api from "../../utils/api";

const HostDetail = () => {
	const { id } = useParams();

	const { data, isLoading, error } = useQuery({
		queryKey: ["docker", "host", id],
		queryFn: async () => {
			const response = await api.get(`/docker/hosts/${id}`);
			return response.data;
		},
		refetchInterval: 30000,
	});

	const host = data?.host;
	const containers = data?.containers || [];
	const images = data?.images || [];
	const stats = data?.stats;

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
						<Server className="h-8 w-8 text-secondary-400 mr-3" />
						<div>
							<h1 className="text-2xl font-bold text-secondary-900 dark:text-white">
								{host.friendly_name || host.hostname}
							</h1>
							<p className="mt-1 text-sm text-secondary-600 dark:text-white">
								{host.ip}
							</p>
						</div>
					</div>
					<Link
						to={`/hosts/${id}`}
						className="inline-flex items-center text-sm text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300"
					>
						View Full Host Details
						<ExternalLink className="ml-2 h-4 w-4" />
					</Link>
				</div>
			</div>

			{/* Overview Cards */}
			<div className="grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-4">
				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<Container className="h-5 w-5 text-blue-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Total Containers
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{stats?.totalContainers || 0}
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
								Running
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{stats?.runningContainers || 0}
							</p>
						</div>
					</div>
				</div>

				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<Container className="h-5 w-5 text-red-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Stopped
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{stats?.stoppedContainers || 0}
							</p>
						</div>
					</div>
				</div>

				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<Package className="h-5 w-5 text-purple-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Images
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{stats?.totalImages || 0}
							</p>
						</div>
					</div>
				</div>
			</div>

			{/* Host Information */}
			<div className="card">
				<div className="px-6 py-5 border-b border-secondary-200 dark:border-secondary-700">
					<h3 className="text-lg leading-6 font-medium text-secondary-900 dark:text-white">
						Host Information
					</h3>
				</div>
				<div className="px-6 py-5 space-y-6">
					<div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Friendly Name
							</dt>
							<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
								{host.friendly_name}
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Hostname
							</dt>
							<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
								{host.hostname}
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								IP Address
							</dt>
							<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
								{host.ip}
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								OS
							</dt>
							<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
								{host.os_type} {host.os_version}
							</dd>
						</div>
					</div>
				</div>
			</div>

			{/* Containers */}
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
									Image
								</th>
								<th
									scope="col"
									className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
								>
									Status
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
									<td className="px-6 py-4 whitespace-nowrap text-sm text-secondary-500">
										{container.image_name}:{container.image_tag}
									</td>
									<td className="px-6 py-4 whitespace-nowrap">
										<span className="inline-flex items-center px-2.5 py-0.5 rounded text-xs font-medium bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200">
											{container.status}
										</span>
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

			{/* Images */}
			<div className="card">
				<div className="px-6 py-5 border-b border-secondary-200 dark:border-secondary-700">
					<h3 className="text-lg leading-6 font-medium text-secondary-900 dark:text-white">
						Images ({images.length})
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
									Repository
								</th>
								<th
									scope="col"
									className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
								>
									Tag
								</th>
								<th
									scope="col"
									className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
								>
									Source
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
							{images.map((image) => (
								<tr key={image.id}>
									<td className="px-6 py-4 whitespace-nowrap">
										<Link
											to={`/docker/images/${image.id}`}
											className="text-sm font-medium text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300"
										>
											{image.repository}
										</Link>
									</td>
									<td className="px-6 py-4 whitespace-nowrap">
										<span className="inline-flex items-center px-2.5 py-0.5 rounded text-xs font-medium bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200">
											{image.tag}
										</span>
									</td>
									<td className="px-6 py-4 whitespace-nowrap text-sm text-secondary-500">
										{image.source}
									</td>
									<td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
										<Link
											to={`/docker/images/${image.id}`}
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

export default HostDetail;
