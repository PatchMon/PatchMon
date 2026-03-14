import { useQuery } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowLeft,
	CheckCircle,
	Container,
	ExternalLink,
	RefreshCw,
	Server,
} from "lucide-react";
import { Link, useParams } from "react-router-dom";
import api, { formatRelativeTime } from "../../utils/api";

const ContainerDetail = () => {
	const { id } = useParams();

	const { data, isLoading, error } = useQuery({
		queryKey: ["docker", "container", id],
		queryFn: async () => {
			const response = await api.get(`/docker/containers/${id}`);
			return response.data;
		},
		refetchInterval: 30000,
	});

	const container = data?.container;
	const similarContainers = data?.similarContainers || [];

	if (isLoading) {
		return (
			<div className="flex items-center justify-center min-h-screen">
				<RefreshCw className="h-8 w-8 animate-spin text-secondary-400" />
			</div>
		);
	}

	if (error || !container) {
		return (
			<div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
				<div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4">
					<div className="flex">
						<AlertTriangle className="h-5 w-5 text-red-400" />
						<div className="ml-3">
							<h3 className="text-sm font-medium text-red-800 dark:text-red-200">
								Container not found
							</h3>
							<p className="mt-2 text-sm text-red-700 dark:text-red-300">
								The container you're looking for doesn't exist or has been
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

	const getStatusBadge = (status) => {
		const statusClasses = {
			running:
				"bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200",
			exited: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200",
			paused:
				"bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200",
			restarting:
				"bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200",
		};
		return (
			<span
				className={`inline-flex items-center px-3 py-1 rounded-full text-sm font-medium ${
					statusClasses[status] ||
					"bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200"
				}`}
			>
				{status}
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
					<Container className="h-8 w-8 text-secondary-400 mr-3" />
					<div>
						<div className="flex items-center gap-3">
							<h1 className="text-2xl font-bold text-secondary-900 dark:text-white">
								{container.name}
							</h1>
							{getStatusBadge(container.status)}
						</div>
						<p className="mt-1 text-sm text-secondary-600 dark:text-white">
							Container ID: {container.container_id.substring(0, 12)}
						</p>
					</div>
				</div>
			</div>

			{/* Overview Cards */}
			<div className="grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-4">
				{/* Update Status Card */}
				{container.docker_images?.docker_image_updates &&
				container.docker_images.docker_image_updates.length > 0 ? (
					<div className="card p-4 bg-yellow-50 dark:bg-yellow-900/20 border-yellow-200 dark:border-yellow-800">
						<div className="flex items-center">
							<div className="flex-shrink-0">
								<AlertTriangle className="h-5 w-5 text-yellow-600 dark:text-yellow-400 mr-2" />
							</div>
							<div className="w-0 flex-1">
								<p className="text-sm text-secondary-500 dark:text-yellow-200">
									Update Available
								</p>
								<p className="text-sm font-medium text-secondary-900 dark:text-yellow-100 truncate">
									{
										container.docker_images.docker_image_updates[0]
											.available_tag
									}
								</p>
							</div>
						</div>
					</div>
				) : (
					<div className="card p-4 bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800">
						<div className="flex items-center">
							<div className="flex-shrink-0">
								<CheckCircle className="h-5 w-5 text-green-600 dark:text-green-400 mr-2" />
							</div>
							<div className="w-0 flex-1">
								<p className="text-sm text-secondary-500 dark:text-green-200">
									Update Status
								</p>
								<p className="text-sm font-medium text-secondary-900 dark:text-green-100">
									Up to date
								</p>
							</div>
						</div>
					</div>
				)}

				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<Server className="h-5 w-5 text-purple-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">Host</p>
							<Link
								to={`/hosts/${container.host?.id}`}
								className="text-sm font-medium text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 truncate block"
							>
								{container.host?.friendly_name || container.host?.hostname}
							</Link>
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
								State
							</p>
							<p className="text-sm font-medium text-secondary-900 dark:text-white">
								{container.state || container.status}
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
								{formatRelativeTime(container.last_checked)}
							</p>
						</div>
					</div>
				</div>
			</div>

			{/* Container and Image Information - Side by Side */}
			<div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
				{/* Container Details */}
				<div className="card">
					<div className="px-6 py-5 border-b border-secondary-200 dark:border-secondary-700">
						<h3 className="text-lg leading-6 font-medium text-secondary-900 dark:text-white">
							Container Information
						</h3>
					</div>
					<div className="px-6 py-5">
						<div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
							<div>
								<dt className="text-sm font-medium text-secondary-500 dark:text-white">
									Container ID
								</dt>
								<dd className="mt-1 text-sm text-secondary-900 dark:text-white font-mono break-all">
									{container.container_id}
								</dd>
							</div>
							<div>
								<dt className="text-sm font-medium text-secondary-500 dark:text-white">
									Image Tag
								</dt>
								<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
									{container.image_tag}
								</dd>
							</div>
							<div>
								<dt className="text-sm font-medium text-secondary-500 dark:text-white">
									Created
								</dt>
								<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
									{formatRelativeTime(container.created_at)}
								</dd>
							</div>
							{container.started_at && (
								<div>
									<dt className="text-sm font-medium text-secondary-500 dark:text-white">
										Started
									</dt>
									<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
										{formatRelativeTime(container.started_at)}
									</dd>
								</div>
							)}
							{container.ports && Object.keys(container.ports).length > 0 && (
								<div className="sm:col-span-2">
									<dt className="text-sm font-medium text-secondary-500 dark:text-white">
										Port Mappings
									</dt>
									<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
										<div className="flex flex-wrap gap-2">
											{Object.entries(container.ports).map(([key, value]) => (
												<span
													key={key}
													className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200"
												>
													{key} → {value}
												</span>
											))}
										</div>
									</dd>
								</div>
							)}
						</div>
					</div>
				</div>

				{/* Image Information */}
				{container.docker_images && (
					<div className="card">
						<div className="px-6 py-5 border-b border-secondary-200 dark:border-secondary-700">
							<h3 className="text-lg leading-6 font-medium text-secondary-900 dark:text-white">
								Image Information
							</h3>
						</div>
						<div className="px-6 py-5">
							<div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
								<div>
									<dt className="text-sm font-medium text-secondary-500 dark:text-white">
										Repository
									</dt>
									<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
										<Link
											to={`/docker/images/${container.docker_images.id}`}
											className="text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 inline-flex items-center"
										>
											{container.docker_images.repository}
											<ExternalLink className="ml-1 h-4 w-4" />
										</Link>
									</dd>
								</div>
								<div>
									<dt className="text-sm font-medium text-secondary-500 dark:text-white">
										Tag
									</dt>
									<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
										{container.docker_images.tag}
									</dd>
								</div>
								<div>
									<dt className="text-sm font-medium text-secondary-500 dark:text-white">
										Source
									</dt>
									<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
										{container.docker_images.source}
									</dd>
								</div>
								{container.docker_images.size_bytes && (
									<div>
										<dt className="text-sm font-medium text-secondary-500 dark:text-white">
											Size
										</dt>
										<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
											{(
												Number(container.docker_images.size_bytes) /
												1024 /
												1024
											).toFixed(2)}{" "}
											MB
										</dd>
									</div>
								)}
								<div>
									<dt className="text-sm font-medium text-secondary-500 dark:text-white">
										Image ID
									</dt>
									<dd className="mt-1 text-xs text-secondary-900 dark:text-white font-mono break-all">
										{container.docker_images.image_id?.substring(0, 12)}...
									</dd>
								</div>
								<div>
									<dt className="text-sm font-medium text-secondary-500 dark:text-white">
										Created
									</dt>
									<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
										{formatRelativeTime(container.docker_images.created_at)}
									</dd>
								</div>
							</div>
						</div>
					</div>
				)}
			</div>

			{/* Similar Containers */}
			{similarContainers.length > 0 && (
				<div className="card">
					<div className="px-6 py-5 border-b border-secondary-200 dark:border-secondary-700">
						<h3 className="text-lg leading-6 font-medium text-secondary-900 dark:text-white">
							Similar Containers (Same Image)
						</h3>
					</div>
					<div className="px-6 py-5">
						<ul className="divide-y divide-secondary-200 dark:divide-secondary-700">
							{similarContainers.map((similar) => (
								<li
									key={similar.id}
									className="py-4 flex items-center justify-between"
								>
									<div className="flex items-center">
										<Container className="h-5 w-5 text-secondary-400 mr-3" />
										<div>
											<Link
												to={`/docker/containers/${similar.id}`}
												className="text-sm font-medium text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300"
											>
												{similar.name}
											</Link>
											<p className="text-sm text-secondary-500 dark:text-white">
												{similar.status}
											</p>
										</div>
									</div>
								</li>
							))}
						</ul>
					</div>
				</div>
			)}
		</div>
	);
};

export default ContainerDetail;
