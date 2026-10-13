import { useMutation, useQuery } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowLeft,
	Container,
	ExternalLink,
	Package,
	RefreshCw,
	Search,
	Server,
} from "lucide-react";
import { Link, useParams } from "react-router-dom";
import UpdateStatusBadge from "../../components/docker/UpdateStatusBadge";
import { useAuth } from "../../contexts/AuthContext";
import { useToast } from "../../contexts/ToastContext";
import api, { adminHostsAPI, formatError } from "../../utils/api";
import { getSourceDisplayName, shortImageId } from "../../utils/docker";

const thClass =
	"px-3 sm:px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider whitespace-nowrap";
const tdClass = "px-3 sm:px-4 py-2 whitespace-nowrap text-sm";
const linkClass =
	"font-medium text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300";

const HostDetail = () => {
	const { id } = useParams();
	const { canManageHosts } = useAuth();
	const toast = useToast();

	const { data, isLoading, error } = useQuery({
		queryKey: ["docker", "host", id],
		queryFn: async () => {
			const response = await api.get(`/docker/hosts/${id}`);
			return response.data;
		},
		refetchInterval: 30000,
	});

	const imageCheckMutation = useMutation({
		mutationFn: () => adminHostsAPI.checkDockerImages(id),
		onSuccess: () => {
			toast.success(
				"Update check requested. Results appear once the agent reports back.",
			);
		},
		onError: (err) => {
			toast.error(formatError(err) || "Failed to request an update check");
		},
	});

	const host = data?.host;
	const containers = data?.containers || [];
	const images = data?.images || [];
	const stats = data?.stats;

	if (isLoading) {
		return (
			<div className="flex items-center justify-center min-h-[calc(100vh-var(--app-main-inset))]">
				<RefreshCw className="h-8 w-8 animate-spin text-secondary-400" />
			</div>
		);
	}

	if (error || !host) {
		return (
			<div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
				<div className="bg-danger-50 dark:bg-danger-900/20 border border-danger-200 dark:border-danger-800 rounded-lg p-4">
					<div className="flex">
						<AlertTriangle className="h-5 w-5 text-danger-400" />
						<div className="ml-3">
							<h3 className="text-sm font-medium text-danger-800 dark:text-danger-200">
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
				<div className="flex flex-col md:flex-row md:items-start md:justify-between gap-4">
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
					<div className="flex flex-wrap items-center gap-3">
						{canManageHosts() && (
							<button
								type="button"
								onClick={() => imageCheckMutation.mutate()}
								disabled={imageCheckMutation.isPending}
								className="btn-outline flex items-center gap-2 min-h-[44px] disabled:opacity-50 disabled:cursor-not-allowed"
							>
								{imageCheckMutation.isPending ? (
									<RefreshCw className="h-4 w-4 animate-spin" />
								) : (
									<Search className="h-4 w-4" />
								)}
								Check for updates
							</button>
						)}
						<Link
							to={`/hosts/${id}`}
							className="inline-flex items-center min-h-[44px] text-sm text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300"
						>
							View Full Host Details
							<ExternalLink className="ml-2 h-4 w-4" />
						</Link>
					</div>
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
							<Container className="h-5 w-5 text-danger-600 mr-2" />
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
								{stats?.totalImages || images.length}
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
				<div className="md:hidden p-4 space-y-3">
					{containers.map((container) => (
						<div key={container.id} className="card p-4 space-y-2">
							<div className="flex items-center justify-between gap-2">
								<Link
									to={`/docker/containers/${container.id}`}
									className={`text-base ${linkClass} min-h-[44px] inline-flex items-center`}
								>
									{container.name}
								</Link>
								<span className="badge-secondary w-fit">
									{container.status}
								</span>
							</div>
							<p
								className="text-sm text-secondary-500 dark:text-white font-mono truncate"
								title={`${container.image_name}:${container.image_tag}`}
							>
								{container.image_name}:{container.image_tag}
							</p>
							<UpdateStatusBadge status={container.update_status} />
						</div>
					))}
				</div>
				<div className="hidden md:block overflow-x-auto">
					<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
						<thead className="bg-secondary-50 dark:bg-secondary-700">
							<tr>
								<th scope="col" className={thClass}>
									Container Name
								</th>
								<th scope="col" className={thClass}>
									Image
								</th>
								<th scope="col" className={thClass}>
									Status
								</th>
								<th scope="col" className={thClass}>
									Update Status
								</th>
								<th scope="col" className={thClass}>
									Actions
								</th>
							</tr>
						</thead>
						<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
							{containers.map((container) => (
								<tr
									key={container.id}
									className="hover:bg-secondary-50 dark:hover:bg-secondary-700 transition-colors"
								>
									<td className={tdClass}>
										<Link
											to={`/docker/containers/${container.id}`}
											className={linkClass}
										>
											{container.name}
										</Link>
									</td>
									<td
										className={`${tdClass} font-mono text-secondary-500 dark:text-white max-w-xs truncate`}
										title={`${container.image_name}:${container.image_tag}`}
									>
										{container.image_name}:{container.image_tag}
									</td>
									<td className={tdClass}>
										<span className="badge-secondary">{container.status}</span>
									</td>
									<td className={tdClass}>
										<UpdateStatusBadge status={container.update_status} />
									</td>
									<td className={`${tdClass} font-medium`}>
										<Link
											to={`/docker/containers/${container.id}`}
											className={`${linkClass} inline-flex items-center`}
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
				<div className="md:hidden p-4 space-y-3">
					{images.map((image) => (
						<div key={image.id} className="card p-4 space-y-2">
							<Link
								to={`/docker/images/${image.id}`}
								className={`text-base ${linkClass} min-h-[44px] inline-flex items-center [overflow-wrap:anywhere]`}
							>
								{image.repository}:{image.tag}
							</Link>
							<p className="text-sm text-secondary-500 dark:text-white font-mono">
								{shortImageId(image.image_id) || "-"}
							</p>
							<UpdateStatusBadge
								status={image.update_status}
								checkedAt={image.update_check?.checked_at}
								reason={image.update_check?.reason}
							/>
						</div>
					))}
				</div>
				<div className="hidden md:block overflow-x-auto">
					<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
						<thead className="bg-secondary-50 dark:bg-secondary-700">
							<tr>
								<th scope="col" className={thClass}>
									Repository
								</th>
								<th scope="col" className={thClass}>
									Tag
								</th>
								<th scope="col" className={thClass}>
									Image ID
								</th>
								<th scope="col" className={thClass}>
									Source
								</th>
								<th scope="col" className={thClass}>
									Update Status
								</th>
								<th scope="col" className={thClass}>
									Actions
								</th>
							</tr>
						</thead>
						<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
							{images.map((image) => (
								<tr
									key={image.id}
									className="hover:bg-secondary-50 dark:hover:bg-secondary-700 transition-colors"
								>
									<td
										className={`${tdClass} max-w-xs truncate`}
										title={image.repository}
									>
										<Link
											to={`/docker/images/${image.id}`}
											className={linkClass}
										>
											{image.repository}
										</Link>
									</td>
									<td className={tdClass}>
										<span
											className="badge-secondary font-mono max-w-[12rem] truncate inline-block"
											title={image.tag}
										>
											{image.tag}
										</span>
									</td>
									<td
										className={`${tdClass} font-mono text-secondary-500 dark:text-white`}
										title={image.image_id}
									>
										{shortImageId(image.image_id) || "-"}
									</td>
									<td
										className={`${tdClass} text-secondary-500 dark:text-white`}
									>
										{getSourceDisplayName(image.source)}
									</td>
									<td className={tdClass}>
										<UpdateStatusBadge
											status={image.update_status}
											checkedAt={image.update_check?.checked_at}
											reason={image.update_check?.reason}
										/>
									</td>
									<td className={`${tdClass} font-medium`}>
										<Link
											to={`/docker/images/${image.id}`}
											className={`${linkClass} inline-flex items-center`}
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
