import { useQuery } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowLeft,
	Database,
	HardDrive,
	RefreshCw,
	Server,
	Tag,
} from "lucide-react";
import { Link, useParams } from "react-router-dom";
import api, { formatRelativeTime } from "../../utils/api";

const VolumeDetail = () => {
	const { id } = useParams();

	const { data, isLoading, error } = useQuery({
		queryKey: ["docker", "volume", id],
		queryFn: async () => {
			const response = await api.get(`/docker/volumes/${id}`);
			return response.data;
		},
		refetchInterval: 30000,
	});

	const volume = data?.volume;
	const host = data?.host;

	if (isLoading) {
		return (
			<div className="flex items-center justify-center min-h-screen">
				<RefreshCw className="h-8 w-8 animate-spin text-secondary-400" />
			</div>
		);
	}

	if (error || !volume) {
		return (
			<div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
				<div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4">
					<div className="flex">
						<AlertTriangle className="h-5 w-5 text-red-400" />
						<div className="ml-3">
							<h3 className="text-sm font-medium text-red-800 dark:text-red-200">
								Volume not found
							</h3>
							<p className="mt-2 text-sm text-red-700 dark:text-red-300">
								The volume you're looking for doesn't exist or has been removed.
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

	const formatBytes = (bytes) => {
		if (bytes === null || bytes === undefined) return "N/A";
		const sizes = ["Bytes", "KB", "MB", "GB", "TB"];
		if (bytes === 0) return "0 Bytes";
		const i = Math.floor(Math.log(bytes) / Math.log(1024));
		return `${Math.round((bytes / 1024 ** i) * 100) / 100} ${sizes[i]}`;
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
					<HardDrive className="h-8 w-8 text-secondary-400 mr-3" />
					<div>
						<h1 className="text-2xl font-bold text-secondary-900 dark:text-white">
							{volume.name}
						</h1>
						<p className="mt-1 text-sm text-secondary-600 dark:text-white">
							Volume ID: {volume.volume_id}
						</p>
					</div>
				</div>
			</div>

			{/* Overview Cards */}
			<div className="grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-4">
				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<HardDrive className="h-5 w-5 text-blue-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Driver
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{volume.driver}
							</p>
						</div>
					</div>
				</div>

				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<Database className="h-5 w-5 text-purple-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">Size</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{formatBytes(volume.size_bytes)}
							</p>
						</div>
					</div>
				</div>

				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<Server className="h-5 w-5 text-green-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Containers
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{volume.ref_count || 0}
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
								{formatRelativeTime(volume.last_checked)}
							</p>
						</div>
					</div>
				</div>
			</div>

			{/* Volume Information Card */}
			<div className="card">
				<div className="px-6 py-5 border-b border-secondary-200 dark:border-secondary-700">
					<h3 className="text-lg leading-6 font-medium text-secondary-900 dark:text-white">
						Volume Information
					</h3>
				</div>
				<div className="px-6 py-5">
					<dl className="grid grid-cols-1 gap-x-4 gap-y-6 sm:grid-cols-2">
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Volume ID
							</dt>
							<dd className="mt-1 text-sm text-secondary-900 dark:text-white font-mono">
								{volume.volume_id}
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Name
							</dt>
							<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
								{volume.name}
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Driver
							</dt>
							<dd className="mt-1">
								<span className="inline-flex items-center px-2.5 py-0.5 rounded text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">
									{volume.driver}
								</span>
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Scope
							</dt>
							<dd className="mt-1">
								<span className="inline-flex items-center px-2.5 py-0.5 rounded text-xs font-medium bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200">
									{volume.scope}
								</span>
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Size
							</dt>
							<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
								{formatBytes(volume.size_bytes)}
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Containers Using
							</dt>
							<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
								{volume.ref_count || 0}
							</dd>
						</div>
						{volume.mountpoint && (
							<div className="sm:col-span-2">
								<dt className="text-sm font-medium text-secondary-500 dark:text-white">
									Mount Point
								</dt>
								<dd className="mt-1 text-sm text-secondary-900 dark:text-white font-mono break-all">
									{volume.mountpoint}
								</dd>
							</div>
						)}
						{volume.renderer && (
							<div>
								<dt className="text-sm font-medium text-secondary-500 dark:text-white">
									Renderer
								</dt>
								<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
									{volume.renderer}
								</dd>
							</div>
						)}
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Created
							</dt>
							<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
								{formatRelativeTime(volume.created_at)}
							</dd>
						</div>
						<div>
							<dt className="text-sm font-medium text-secondary-500 dark:text-white">
								Last Checked
							</dt>
							<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
								{formatRelativeTime(volume.last_checked)}
							</dd>
						</div>
					</dl>
				</div>
			</div>

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
			{volume.labels && Object.keys(volume.labels).length > 0 && (
				<div className="card">
					<div className="px-6 py-5 border-b border-secondary-200 dark:border-secondary-700">
						<h3 className="text-lg leading-6 font-medium text-secondary-900 dark:text-white flex items-center">
							<Tag className="h-5 w-5 mr-2" />
							Labels
						</h3>
					</div>
					<div className="px-6 py-5">
						<div className="space-y-2">
							{Object.entries(volume.labels).map(([key, value]) => (
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

			{/* Options */}
			{volume.options && Object.keys(volume.options).length > 0 && (
				<div className="card">
					<div className="px-6 py-5 border-b border-secondary-200 dark:border-secondary-700">
						<h3 className="text-lg leading-6 font-medium text-secondary-900 dark:text-white">
							Volume Options
						</h3>
					</div>
					<div className="px-6 py-5">
						<div className="space-y-2">
							{Object.entries(volume.options).map(([key, value]) => (
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

export default VolumeDetail;
