import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowDown,
	ArrowUp,
	ArrowUpDown,
	Container,
	ExternalLink,
	HardDrive,
	Network,
	Package,
	RefreshCw,
	Search,
	Server,
	Trash2,
	X,
} from "lucide-react";
import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import api from "../utils/api";
import { generateRegistryLink, getSourceDisplayName } from "../utils/docker";

const Docker = () => {
	const queryClient = useQueryClient();
	const [searchTerm, setSearchTerm] = useState("");
	const [activeTab, setActiveTab] = useState("containers");
	const [sortField, setSortField] = useState("status");
	const [sortDirection, setSortDirection] = useState("asc");
	const [statusFilter, setStatusFilter] = useState("all");
	const [sourceFilter, setSourceFilter] = useState("all");
	const [updatesFilter, setUpdatesFilter] = useState("all");
	const [driverFilter, setDriverFilter] = useState("all");
	const [deleteContainerModal, setDeleteContainerModal] = useState(null);
	const [deleteImageModal, setDeleteImageModal] = useState(null);
	const [deleteVolumeModal, setDeleteVolumeModal] = useState(null);
	const [deleteNetworkModal, setDeleteNetworkModal] = useState(null);

	// Fetch Docker dashboard data
	const { data: dashboard, isLoading: dashboardLoading } = useQuery({
		queryKey: ["docker", "dashboard"],
		queryFn: async () => {
			const response = await api.get("/docker/dashboard");
			return response.data;
		},
		refetchInterval: 30000, // Refresh every 30 seconds
	});

	// Fetch containers
	const {
		data: containersData,
		isLoading: containersLoading,
		refetch: refetchContainers,
	} = useQuery({
		queryKey: ["docker", "containers", statusFilter],
		queryFn: async () => {
			const params = new URLSearchParams();
			if (statusFilter !== "all") params.set("status", statusFilter);
			params.set("limit", "1000");
			const response = await api.get(`/docker/containers?${params}`);
			return response.data;
		},
		enabled: activeTab === "containers",
	});

	// Fetch images
	const {
		data: imagesData,
		isLoading: imagesLoading,
		refetch: refetchImages,
	} = useQuery({
		queryKey: ["docker", "images", sourceFilter],
		queryFn: async () => {
			const params = new URLSearchParams();
			if (sourceFilter !== "all") params.set("source", sourceFilter);
			params.set("limit", "1000");
			const response = await api.get(`/docker/images?${params}`);
			return response.data;
		},
		enabled: activeTab === "images",
	});

	// Fetch hosts
	const { data: hostsData, isLoading: hostsLoading } = useQuery({
		queryKey: ["docker", "hosts"],
		queryFn: async () => {
			const response = await api.get("/docker/hosts?limit=1000");
			return response.data;
		},
		enabled: activeTab === "hosts",
	});

	// Fetch volumes
	const {
		data: volumesData,
		isLoading: volumesLoading,
		refetch: refetchVolumes,
	} = useQuery({
		queryKey: ["docker", "volumes", driverFilter],
		queryFn: async () => {
			const params = new URLSearchParams();
			if (driverFilter !== "all") params.set("driver", driverFilter);
			params.set("limit", "1000");
			const response = await api.get(`/docker/volumes?${params}`);
			return response.data;
		},
		enabled: activeTab === "volumes",
	});

	// Fetch networks
	const {
		data: networksData,
		isLoading: networksLoading,
		refetch: refetchNetworks,
	} = useQuery({
		queryKey: ["docker", "networks", driverFilter],
		queryFn: async () => {
			const params = new URLSearchParams();
			if (driverFilter !== "all") params.set("driver", driverFilter);
			params.set("limit", "1000");
			const response = await api.get(`/docker/networks?${params}`);
			return response.data;
		},
		enabled: activeTab === "networks",
	});

	// Delete container mutation
	const deleteContainerMutation = useMutation({
		mutationFn: async (containerId) => {
			const response = await api.delete(`/docker/containers/${containerId}`);
			return response.data;
		},
		onSuccess: () => {
			queryClient.invalidateQueries(["docker", "containers"]);
			queryClient.invalidateQueries(["docker", "dashboard"]);
			setDeleteContainerModal(null);
		},
		onError: (error) => {
			alert(
				`Failed to delete container: ${error.response?.data?.error || error.message}`,
			);
		},
	});

	// Delete image mutation
	const deleteImageMutation = useMutation({
		mutationFn: async (imageId) => {
			const response = await api.delete(`/docker/images/${imageId}`);
			return response.data;
		},
		onSuccess: () => {
			queryClient.invalidateQueries(["docker", "images"]);
			queryClient.invalidateQueries(["docker", "dashboard"]);
			setDeleteImageModal(null);
		},
		onError: (error) => {
			alert(
				`Failed to delete image: ${error.response?.data?.error || error.message}`,
			);
		},
	});

	// Delete volume mutation
	const deleteVolumeMutation = useMutation({
		mutationFn: async (volumeId) => {
			const response = await api.delete(`/docker/volumes/${volumeId}`);
			return response.data;
		},
		onSuccess: () => {
			queryClient.invalidateQueries(["docker", "volumes"]);
			queryClient.invalidateQueries(["docker", "dashboard"]);
			setDeleteVolumeModal(null);
		},
		onError: (error) => {
			alert(
				`Failed to delete volume: ${error.response?.data?.error || error.message}`,
			);
		},
	});

	// Delete network mutation
	const deleteNetworkMutation = useMutation({
		mutationFn: async (networkId) => {
			const response = await api.delete(`/docker/networks/${networkId}`);
			return response.data;
		},
		onSuccess: () => {
			queryClient.invalidateQueries(["docker", "networks"]);
			queryClient.invalidateQueries(["docker", "dashboard"]);
			setDeleteNetworkModal(null);
		},
		onError: (error) => {
			alert(
				`Failed to delete network: ${error.response?.data?.error || error.message}`,
			);
		},
	});

	// Filter and sort containers
	const filteredContainers = useMemo(() => {
		if (!containersData?.containers) return [];
		let filtered = containersData.containers;

		if (searchTerm) {
			const term = searchTerm.toLowerCase();
			filtered = filtered.filter(
				(c) =>
					c.name.toLowerCase().includes(term) ||
					c.image_name.toLowerCase().includes(term) ||
					c.host?.friendly_name?.toLowerCase().includes(term),
			);
		}

		filtered.sort((a, b) => {
			let aValue, bValue;
			if (sortField === "name") {
				aValue = a.name?.toLowerCase() || "";
				bValue = b.name?.toLowerCase() || "";
			} else if (sortField === "image") {
				aValue = a.image_name?.toLowerCase() || "";
				bValue = b.image_name?.toLowerCase() || "";
			} else if (sortField === "status") {
				// Custom status priority: running first, then others alphabetically
				const statusPriority = {
					running: 1,
					created: 2,
					restarting: 3,
					paused: 4,
					exited: 5,
					dead: 6,
				};
				const aPriority = statusPriority[a.status] || 999;
				const bPriority = statusPriority[b.status] || 999;

				if (sortDirection === "asc") {
					if (aPriority !== bPriority) return aPriority - bPriority;
					// Secondary sort by name within same status
					return (a.name?.toLowerCase() || "").localeCompare(
						b.name?.toLowerCase() || "",
					);
				} else {
					if (aPriority !== bPriority) return bPriority - aPriority;
					// Secondary sort by name within same status
					return (b.name?.toLowerCase() || "").localeCompare(
						a.name?.toLowerCase() || "",
					);
				}
			} else if (sortField === "host") {
				aValue = a.host?.friendly_name?.toLowerCase() || "";
				bValue = b.host?.friendly_name?.toLowerCase() || "";
			}

			if (sortField !== "status") {
				if (aValue < bValue) return sortDirection === "asc" ? -1 : 1;
				if (aValue > bValue) return sortDirection === "asc" ? 1 : -1;
			}
			return 0;
		});

		return filtered;
	}, [containersData, searchTerm, sortField, sortDirection]);

	// Filter and sort images
	const filteredImages = useMemo(() => {
		if (!imagesData?.images) return [];
		let filtered = imagesData.images;

		if (searchTerm) {
			const term = searchTerm.toLowerCase();
			filtered = filtered.filter(
				(img) =>
					img.repository.toLowerCase().includes(term) ||
					img.tag.toLowerCase().includes(term),
			);
		}

		// Filter by updates status
		if (updatesFilter !== "all") {
			if (updatesFilter === "available") {
				filtered = filtered.filter((img) => img.hasUpdates === true);
			} else if (updatesFilter === "none") {
				filtered = filtered.filter((img) => !img.hasUpdates);
			}
		}

		filtered.sort((a, b) => {
			let aValue, bValue;
			if (sortField === "repository") {
				aValue = a.repository?.toLowerCase() || "";
				bValue = b.repository?.toLowerCase() || "";
			} else if (sortField === "tag") {
				aValue = a.tag || "";
				bValue = b.tag || "";
			} else if (sortField === "containers") {
				aValue = a._count?.docker_containers || 0;
				bValue = b._count?.docker_containers || 0;
			}

			if (aValue < bValue) return sortDirection === "asc" ? -1 : 1;
			if (aValue > bValue) return sortDirection === "asc" ? 1 : -1;
			return 0;
		});

		return filtered;
	}, [imagesData, searchTerm, sortField, sortDirection, updatesFilter]);

	// Filter and sort hosts
	const filteredHosts = useMemo(() => {
		if (!hostsData?.hosts) return [];
		let filtered = hostsData.hosts;

		if (searchTerm) {
			const term = searchTerm.toLowerCase();
			filtered = filtered.filter(
				(h) =>
					h.friendly_name?.toLowerCase().includes(term) ||
					h.hostname?.toLowerCase().includes(term),
			);
		}

		filtered.sort((a, b) => {
			let aValue, bValue;
			if (sortField === "name") {
				aValue = a.friendly_name?.toLowerCase() || "";
				bValue = b.friendly_name?.toLowerCase() || "";
			} else if (sortField === "containers") {
				aValue = a.dockerStats?.totalContainers || 0;
				bValue = b.dockerStats?.totalContainers || 0;
			} else if (sortField === "images") {
				aValue = a.dockerStats?.totalImages || 0;
				bValue = b.dockerStats?.totalImages || 0;
			}

			if (aValue < bValue) return sortDirection === "asc" ? -1 : 1;
			if (aValue > bValue) return sortDirection === "asc" ? 1 : -1;
			return 0;
		});

		return filtered;
	}, [hostsData, searchTerm, sortField, sortDirection]);

	// Filter and sort volumes
	const filteredVolumes = useMemo(() => {
		if (!volumesData?.volumes) return [];
		let filtered = volumesData.volumes;

		if (searchTerm) {
			const term = searchTerm.toLowerCase();
			filtered = filtered.filter(
				(v) =>
					v.name.toLowerCase().includes(term) ||
					v.hosts?.friendly_name?.toLowerCase().includes(term),
			);
		}

		filtered.sort((a, b) => {
			let aValue, bValue;
			if (sortField === "name") {
				aValue = a.name?.toLowerCase() || "";
				bValue = b.name?.toLowerCase() || "";
			} else if (sortField === "driver") {
				aValue = a.driver?.toLowerCase() || "";
				bValue = b.driver?.toLowerCase() || "";
			} else if (sortField === "size") {
				aValue = a.size_bytes ? BigInt(a.size_bytes) : BigInt(0);
				bValue = b.size_bytes ? BigInt(b.size_bytes) : BigInt(0);
			} else if (sortField === "ref_count") {
				aValue = a.ref_count || 0;
				bValue = b.ref_count || 0;
			} else if (sortField === "host") {
				aValue = a.hosts?.friendly_name?.toLowerCase() || "";
				bValue = b.hosts?.friendly_name?.toLowerCase() || "";
			}

			if (sortField === "size") {
				// BigInt comparison
				if (aValue < bValue) return sortDirection === "asc" ? -1 : 1;
				if (aValue > bValue) return sortDirection === "asc" ? 1 : -1;
			} else if (sortField === "ref_count") {
				// Number comparison
				if (aValue < bValue) return sortDirection === "asc" ? -1 : 1;
				if (aValue > bValue) return sortDirection === "asc" ? 1 : -1;
			} else {
				// String comparison
				if (aValue < bValue) return sortDirection === "asc" ? -1 : 1;
				if (aValue > bValue) return sortDirection === "asc" ? 1 : -1;
			}
			return 0;
		});

		return filtered;
	}, [volumesData, searchTerm, sortField, sortDirection]);

	// Filter and sort networks
	const filteredNetworks = useMemo(() => {
		if (!networksData?.networks) return [];
		let filtered = networksData.networks;

		if (searchTerm) {
			const term = searchTerm.toLowerCase();
			filtered = filtered.filter(
				(n) =>
					n.name.toLowerCase().includes(term) ||
					n.hosts?.friendly_name?.toLowerCase().includes(term),
			);
		}

		filtered.sort((a, b) => {
			let aValue, bValue;
			if (sortField === "name") {
				aValue = a.name?.toLowerCase() || "";
				bValue = b.name?.toLowerCase() || "";
			} else if (sortField === "driver") {
				aValue = a.driver?.toLowerCase() || "";
				bValue = b.driver?.toLowerCase() || "";
			} else if (sortField === "containers") {
				aValue = a.container_count || 0;
				bValue = b.container_count || 0;
			} else if (sortField === "host") {
				aValue = a.hosts?.friendly_name?.toLowerCase() || "";
				bValue = b.hosts?.friendly_name?.toLowerCase() || "";
			}

			if (sortField === "containers") {
				// Number comparison
				if (aValue < bValue) return sortDirection === "asc" ? -1 : 1;
				if (aValue > bValue) return sortDirection === "asc" ? 1 : -1;
			} else {
				// String comparison
				if (aValue < bValue) return sortDirection === "asc" ? -1 : 1;
				if (aValue > bValue) return sortDirection === "asc" ? 1 : -1;
			}
			return 0;
		});

		return filtered;
	}, [networksData, searchTerm, sortField, sortDirection]);

	const handleSort = (field) => {
		if (sortField === field) {
			setSortDirection(sortDirection === "asc" ? "desc" : "asc");
		} else {
			setSortField(field);
			setSortDirection("asc");
		}
	};

	const getSortIcon = (field) => {
		if (sortField !== field) return <ArrowUpDown className="h-4 w-4" />;
		return sortDirection === "asc" ? (
			<ArrowUp className="h-4 w-4" />
		) : (
			<ArrowDown className="h-4 w-4" />
		);
	};

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
				className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${
					statusClasses[status] ||
					"bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200"
				}`}
			>
				{status}
			</span>
		);
	};

	const getSourceBadge = (source, repository) => {
		// Generate registry link if possible
		const registryLink = repository
			? generateRegistryLink(repository, source)
			: null;

		// Get display name
		const displayName = getSourceDisplayName(source);

		// Color schemes for different sources
		const colorSchemes = {
			"docker-hub":
				"bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200 hover:bg-blue-200 dark:hover:bg-blue-800",
			github:
				"bg-secondary-100 text-secondary-900 dark:bg-secondary-700 dark:text-white hover:bg-secondary-200 dark:hover:bg-secondary-600",
			gitlab:
				"bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200 hover:bg-orange-200 dark:hover:bg-orange-800",
			google:
				"bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200 hover:bg-red-200 dark:hover:bg-red-800",
			quay: "bg-teal-100 text-teal-800 dark:bg-teal-900 dark:text-teal-200 hover:bg-teal-200 dark:hover:bg-teal-800",
			redhat:
				"bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200 hover:bg-red-200 dark:hover:bg-red-800",
			azure:
				"bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200 hover:bg-blue-200 dark:hover:bg-blue-800",
			aws: "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200 hover:bg-yellow-200 dark:hover:bg-yellow-800",
			private:
				"bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200 hover:bg-purple-200 dark:hover:bg-purple-800",
			local:
				"bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200 hover:bg-secondary-200 dark:hover:bg-secondary-600",
			unknown:
				"bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200 hover:bg-secondary-200 dark:hover:bg-secondary-600",
		};

		const colorScheme = colorSchemes[source] || colorSchemes.unknown;

		if (registryLink) {
			// Return as clickable link
			return (
				<a
					href={registryLink}
					target="_blank"
					rel="noopener noreferrer"
					className={`inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-xs font-medium transition-colors ${colorScheme}`}
					title={`View on ${displayName}`}
				>
					{displayName}
					<ExternalLink className="h-3 w-3" />
				</a>
			);
		}

		// Return as non-clickable badge
		return (
			<span
				className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${colorScheme.split(" hover:")[0]}`}
			>
				{displayName}
			</span>
		);
	};

	return (
		<div className="md:h-[calc(100vh-7rem)] md:flex md:flex-col md:overflow-hidden">
			{/* Header */}
			<div className="flex items-center justify-between mb-6">
				<div>
					<h1 className="text-2xl font-semibold text-secondary-900 dark:text-white">
						Docker Inventory
					</h1>
					<p className="text-sm text-secondary-600 dark:text-secondary-400 mt-1">
						Monitor containers, images, and updates across your infrastructure
					</p>
				</div>
				<div className="flex items-center gap-3">
					<button
						type="button"
						onClick={() => {
							// Trigger refresh based on active tab
							if (activeTab === "containers") refetchContainers();
							else if (activeTab === "images") refetchImages();
							else if (activeTab === "volumes") refetchVolumes();
							else if (activeTab === "networks") refetchNetworks();
							else window.location.reload();
						}}
						className="btn-outline flex items-center justify-center p-2"
						title="Refresh data"
					>
						<RefreshCw className="h-4 w-4" />
					</button>
				</div>
			</div>

			{/* Stats Summary */}
			<div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<Server className="h-5 w-5 text-primary-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Hosts with Docker
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{dashboardLoading ? (
									<span className="animate-pulse">-</span>
								) : (
									dashboard?.stats?.totalHostsWithDocker || 0
								)}
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
								Running Containers
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{dashboardLoading ? (
									<span className="animate-pulse">-</span>
								) : (
									<>
										{dashboard?.stats?.runningContainers || 0}
										<span className="ml-2 text-sm text-secondary-500 dark:text-secondary-400 font-normal">
											/ {dashboard?.stats?.totalContainers || 0} total
										</span>
									</>
								)}
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
							<p className="text-sm text-secondary-500 dark:text-white">
								Total Images
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{dashboardLoading ? (
									<span className="animate-pulse">-</span>
								) : (
									dashboard?.stats?.totalImages || 0
								)}
							</p>
						</div>
					</div>
				</div>

				<button
					type="button"
					onClick={() => {
						setActiveTab("images");
						setUpdatesFilter("available");
						setSourceFilter("all"); // Reset source filter
						setSearchTerm(""); // Clear search
						setSortField("repository");
						setSortDirection("asc");
					}}
					className="card p-4 hover:shadow-lg transition-shadow cursor-pointer text-left w-full disabled:opacity-50 disabled:cursor-not-allowed"
					disabled={dashboardLoading || !dashboard?.stats?.availableUpdates}
				>
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<AlertTriangle className="h-5 w-5 text-warning-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Updates Available
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{dashboardLoading ? (
									<span className="animate-pulse">-</span>
								) : (
									dashboard?.stats?.availableUpdates || 0
								)}
							</p>
						</div>
					</div>
				</button>
			</div>

			{/* Docker List */}
			<div className="card flex-1 flex flex-col overflow-hidden min-h-0">
				{/* Tab Navigation */}
				<div className="border-b border-secondary-200 dark:border-secondary-600">
					<nav className="-mb-px flex space-x-8 px-4" aria-label="Tabs">
						{[
							{ id: "containers", label: "Containers", icon: Container },
							{ id: "images", label: "Images", icon: Package },
							{ id: "volumes", label: "Volumes", icon: HardDrive },
							{ id: "networks", label: "Networks", icon: Network },
							{ id: "hosts", label: "Hosts", icon: Server },
						].map((tab) => {
							const Icon = tab.icon;
							return (
								<button
									key={tab.id}
									type="button"
									onClick={() => {
										setActiveTab(tab.id);
										setSearchTerm("");
										setUpdatesFilter("all"); // Reset updates filter when switching tabs
										setSortField(
											tab.id === "containers"
												? "status"
												: tab.id === "images"
													? "repository"
													: tab.id === "volumes"
														? "name"
														: tab.id === "networks"
															? "name"
															: "name",
										);
										setSortDirection("asc");
									}}
									className={`${
										activeTab === tab.id
											? "border-primary-500 text-primary-600 dark:text-primary-400"
											: "border-transparent text-secondary-500 hover:text-secondary-700 hover:border-secondary-300 dark:text-secondary-400 dark:hover:text-secondary-300"
									} whitespace-nowrap py-4 px-1 border-b-2 font-medium text-sm flex items-center`}
								>
									<Icon className="h-4 w-4 mr-2" />
									{tab.label}
								</button>
							);
						})}
					</nav>
				</div>

				{/* Filters and Search */}
				<div className="p-4 border-b border-secondary-200 dark:border-secondary-600">
					<div className="flex flex-col sm:flex-row gap-4">
						<div className="hidden md:flex flex-1">
							<div className="relative w-full">
								<div className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
									<Search className="h-5 w-5 text-secondary-400" />
								</div>
								<input
									type="text"
									className="block w-full pl-10 pr-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md leading-5 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white placeholder-secondary-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500 sm:text-sm"
									placeholder={`Search ${activeTab}...`}
									value={searchTerm}
									onChange={(e) => setSearchTerm(e.target.value)}
								/>
								{searchTerm && (
									<button
										type="button"
										onClick={() => setSearchTerm("")}
										className="absolute inset-y-0 right-0 pr-3 flex items-center"
									>
										<X className="h-5 w-5 text-secondary-400 hover:text-secondary-600" />
									</button>
								)}
							</div>
						</div>
						{activeTab === "containers" && (
							<select
								value={statusFilter}
								onChange={(e) => setStatusFilter(e.target.value)}
								className="block w-full sm:w-48 pl-3 pr-10 py-2 text-base border-secondary-300 dark:border-secondary-600 focus:outline-none focus:ring-primary-500 focus:border-primary-500 sm:text-sm rounded-md bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
							>
								<option value="all">All Statuses</option>
								<option value="running">Running</option>
								<option value="exited">Exited</option>
								<option value="paused">Paused</option>
								<option value="restarting">Restarting</option>
							</select>
						)}
						{activeTab === "images" && (
							<>
								<select
									value={sourceFilter}
									onChange={(e) => setSourceFilter(e.target.value)}
									className="block w-full sm:w-48 pl-3 pr-10 py-2 text-base border-secondary-300 dark:border-secondary-600 focus:outline-none focus:ring-primary-500 focus:border-primary-500 sm:text-sm rounded-md bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
								>
									<option value="all">All Sources</option>
									<option value="docker-hub">Docker Hub</option>
									<option value="github">GitHub</option>
									<option value="gitlab">GitLab</option>
									<option value="google">Google</option>
									<option value="quay">Quay.io</option>
									<option value="redhat">Red Hat</option>
									<option value="azure">Azure</option>
									<option value="aws">AWS ECR</option>
									<option value="private">Private</option>
									<option value="local">Local</option>
								</select>
								<select
									value={updatesFilter}
									onChange={(e) => setUpdatesFilter(e.target.value)}
									className="block w-full sm:w-48 pl-3 pr-10 py-2 text-base border-secondary-300 dark:border-secondary-600 focus:outline-none focus:ring-primary-500 focus:border-primary-500 sm:text-sm rounded-md bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
								>
									<option value="all">All Updates Status</option>
									<option value="available">Has Updates</option>
									<option value="none">Up to Date</option>
								</select>
							</>
						)}
						{(activeTab === "volumes" || activeTab === "networks") && (
							<select
								value={driverFilter}
								onChange={(e) => setDriverFilter(e.target.value)}
								className="block w-full sm:w-48 pl-3 pr-10 py-2 text-base border-secondary-300 dark:border-secondary-600 focus:outline-none focus:ring-primary-500 focus:border-primary-500 sm:text-sm rounded-md bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
							>
								<option value="all">All Drivers</option>
								<option value="local">Local</option>
								<option value="bridge">Bridge</option>
								<option value="host">Host</option>
								<option value="overlay">Overlay</option>
								<option value="macvlan">Macvlan</option>
							</select>
						)}
					</div>
				</div>

				{/* Tab Content */}
				<div className="p-4 flex-1 overflow-auto">
					{/* Containers Tab */}
					{activeTab === "containers" &&
						(containersLoading ? (
							<div className="text-center py-8">
								<RefreshCw className="h-8 w-8 animate-spin mx-auto text-secondary-400" />
								<p className="mt-2 text-sm text-secondary-500">
									Loading containers...
								</p>
							</div>
						) : filteredContainers.length === 0 ? (
							<div className="text-center py-8">
								<Container className="h-12 w-12 mx-auto text-secondary-400" />
								<h3 className="mt-2 text-sm font-medium text-secondary-900 dark:text-white">
									No containers found
								</h3>
								<p className="mt-1 text-sm text-secondary-500">
									{searchTerm
										? "Try adjusting your search filters"
										: "No Docker containers detected on any hosts"}
								</p>
							</div>
						) : (
							<>
								{/* Mobile Card Layout */}
								<div className="md:hidden space-y-3 pb-4">
									{filteredContainers.map((container) => (
										<div key={container.id} className="card p-4 space-y-3">
											{/* Container Name */}
											<Link
												to={`/docker/containers/${container.id}`}
												className="flex items-center gap-3"
											>
												<Container className="h-5 w-5 text-secondary-400 flex-shrink-0" />
												<div className="text-base font-semibold text-secondary-900 dark:text-white hover:text-primary-600 dark:hover:text-primary-400 truncate">
													{container.name}
												</div>
											</Link>

											{/* Status */}
											<div className="flex items-center gap-2">
												{getStatusBadge(container.status)}
											</div>

											{/* Image and Host Info */}
											<div className="space-y-2 pt-2 border-t border-secondary-200 dark:border-secondary-600">
												<div className="text-sm">
													<span className="text-secondary-500 dark:text-secondary-400">
														Image:&nbsp;
													</span>
													<span className="text-secondary-900 dark:text-white font-mono text-sm">
														{container.image_name}:{container.image_tag}
													</span>
												</div>
												<div className="text-sm">
													<span className="text-secondary-500 dark:text-secondary-400">
														Host:&nbsp;
													</span>
													<Link
														to={`/hosts/${container.host_id}`}
														className="text-secondary-900 dark:text-white hover:text-primary-600 dark:hover:text-primary-400"
													>
														{container.host?.friendly_name ||
															container.host?.hostname ||
															"Unknown"}
													</Link>
												</div>
											</div>

											{/* Actions */}
											<div className="flex items-center justify-end gap-3 pt-2 border-t border-secondary-200 dark:border-secondary-600">
												<Link
													to={`/docker/containers/${container.id}`}
													className="text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 inline-flex items-center gap-1 text-sm"
													title="View details"
												>
													<ExternalLink className="h-4 w-4" />
													View
												</Link>
												<button
													type="button"
													onClick={() => setDeleteContainerModal(container)}
													className="text-red-600 hover:text-red-900 dark:text-red-400 dark:hover:text-red-300 inline-flex items-center gap-1 text-sm"
													title="Delete container from inventory"
												>
													<Trash2 className="h-4 w-4" />
													Delete
												</button>
											</div>
										</div>
									))}
								</div>

								{/* Desktop Table Layout */}
								<div className="hidden md:block overflow-x-auto">
									<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
										<thead className="bg-secondary-50 dark:bg-secondary-700">
											<tr>
												<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													<button
														type="button"
														onClick={() => handleSort("name")}
														className="flex items-center gap-2 hover:text-secondary-700"
													>
														Container Name
														{getSortIcon("name")}
													</button>
												</th>
												<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													<button
														type="button"
														onClick={() => handleSort("image")}
														className="flex items-center gap-2 hover:text-secondary-700"
													>
														Image
														{getSortIcon("image")}
													</button>
												</th>
												<th className="px-4 py-2 text-center text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													<button
														type="button"
														onClick={() => handleSort("status")}
														className="flex items-center gap-2 hover:text-secondary-700"
													>
														Status
														{getSortIcon("status")}
													</button>
												</th>
												<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													<button
														type="button"
														onClick={() => handleSort("host")}
														className="flex items-center gap-2 hover:text-secondary-700"
													>
														Host
														{getSortIcon("host")}
													</button>
												</th>
												<th className="px-4 py-2 text-center text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													Actions
												</th>
											</tr>
										</thead>
										<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
											{filteredContainers.map((container) => (
												<tr
													key={container.id}
													className="hover:bg-secondary-50 dark:hover:bg-secondary-700 transition-colors"
												>
													<td className="px-4 py-2 whitespace-nowrap">
														<div className="flex items-center gap-2">
															<Container className="h-4 w-4 text-secondary-400 dark:text-secondary-500 flex-shrink-0" />
															<Link
																to={`/docker/containers/${container.id}`}
																className="text-sm font-medium text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 truncate"
															>
																{container.name}
															</Link>
														</div>
													</td>
													<td className="px-4 py-2">
														<div className="text-sm text-secondary-900 dark:text-white">
															{container.image_name}:{container.image_tag}
														</div>
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-center">
														{getStatusBadge(container.status)}
													</td>
													<td className="px-4 py-2 whitespace-nowrap">
														<Link
															to={`/hosts/${container.host_id}`}
															className="text-sm text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300"
														>
															{container.host?.friendly_name ||
																container.host?.hostname ||
																"Unknown"}
														</Link>
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-center">
														<div className="flex items-center justify-center gap-3">
															<Link
																to={`/docker/containers/${container.id}`}
																className="text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 inline-flex items-center gap-1"
																title="View details"
															>
																<ExternalLink className="h-4 w-4" />
															</Link>
															<button
																type="button"
																onClick={() =>
																	setDeleteContainerModal(container)
																}
																className="text-red-600 hover:text-red-900 dark:text-red-400 dark:hover:text-red-300 inline-flex items-center"
																title="Delete container from inventory"
															>
																<Trash2 className="h-4 w-4" />
															</button>
														</div>
													</td>
												</tr>
											))}
										</tbody>
									</table>
								</div>
							</>
						))}

					{/* Images Tab */}
					{activeTab === "images" &&
						(imagesLoading ? (
							<div className="text-center py-8">
								<RefreshCw className="h-8 w-8 animate-spin mx-auto text-secondary-400" />
								<p className="mt-2 text-sm text-secondary-500">
									Loading images...
								</p>
							</div>
						) : filteredImages.length === 0 ? (
							<div className="text-center py-8">
								<Package className="h-12 w-12 mx-auto text-secondary-400" />
								<h3 className="mt-2 text-sm font-medium text-secondary-900 dark:text-white">
									No images found
								</h3>
								<p className="mt-1 text-sm text-secondary-500">
									{searchTerm
										? "Try adjusting your search filters"
										: "No Docker images detected"}
								</p>
							</div>
						) : (
							<>
								{/* Mobile Card Layout */}
								<div className="md:hidden space-y-3 pb-4">
									{filteredImages.map((image) => (
										<div key={image.id} className="card p-4 space-y-3">
											{/* Repository and Tag */}
											<Link
												to={`/docker/images/${image.id}`}
												className="flex items-center gap-3"
											>
												<Package className="h-5 w-5 text-secondary-400 flex-shrink-0" />
												<div className="flex-1 min-w-0">
													<div className="text-base font-semibold text-secondary-900 dark:text-white hover:text-primary-600 dark:hover:text-primary-400 truncate">
														{image.repository}
													</div>
													<div className="text-sm text-secondary-500 dark:text-secondary-400">
														Tag: {image.tag}
													</div>
												</div>
											</Link>

											{/* Source and Updates Status */}
											<div className="flex items-center justify-between gap-2">
												<div>
													{getSourceBadge(image.source, image.repository)}
												</div>
												{image.hasUpdates ? (
													<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200">
														<AlertTriangle className="h-3 w-3 mr-1" />
														Updates Available
													</span>
												) : (
													<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
														Up to date
													</span>
												)}
											</div>

											{/* Containers Count */}
											<div className="pt-2 border-t border-secondary-200 dark:border-secondary-600">
												<div className="text-sm">
													<span className="text-secondary-500 dark:text-secondary-400">
														Containers:&nbsp;
													</span>
													<span className="text-secondary-900 dark:text-white font-semibold">
														{image._count?.docker_containers || 0}
													</span>
												</div>
											</div>

											{/* Actions */}
											<div className="flex items-center justify-end gap-3 pt-2 border-t border-secondary-200 dark:border-secondary-600">
												<Link
													to={`/docker/images/${image.id}`}
													className="text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 inline-flex items-center gap-1 text-sm"
													title="View details"
												>
													<ExternalLink className="h-4 w-4" />
													View
												</Link>
												<button
													type="button"
													onClick={() => setDeleteImageModal(image)}
													className="text-red-600 hover:text-red-900 dark:text-red-400 dark:hover:text-red-300 inline-flex items-center gap-1 text-sm"
													title="Delete image from inventory"
												>
													<Trash2 className="h-4 w-4" />
													Delete
												</button>
											</div>
										</div>
									))}
								</div>

								{/* Desktop Table Layout */}
								<div className="hidden md:block overflow-x-auto">
									<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
										<thead className="bg-secondary-50 dark:bg-secondary-700">
											<tr>
												<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													<button
														type="button"
														onClick={() => handleSort("repository")}
														className="flex items-center gap-2 hover:text-secondary-700"
													>
														Repository
														{getSortIcon("repository")}
													</button>
												</th>
												<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													<button
														type="button"
														onClick={() => handleSort("tag")}
														className="flex items-center gap-2 hover:text-secondary-700"
													>
														Tag
														{getSortIcon("tag")}
													</button>
												</th>
												<th className="px-4 py-2 text-center text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													Source
												</th>
												<th className="px-4 py-2 text-center text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													<button
														type="button"
														onClick={() => handleSort("containers")}
														className="flex items-center gap-2 hover:text-secondary-700"
													>
														Containers
														{getSortIcon("containers")}
													</button>
												</th>
												<th className="px-4 py-2 text-center text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													Updates
												</th>
												<th className="px-4 py-2 text-center text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													Actions
												</th>
											</tr>
										</thead>
										<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
											{filteredImages.map((image) => (
												<tr
													key={image.id}
													className="hover:bg-secondary-50 dark:hover:bg-secondary-700 transition-colors"
												>
													<td className="px-4 py-2 whitespace-nowrap">
														<div className="flex items-center gap-2">
															<Package className="h-4 w-4 text-secondary-400 dark:text-secondary-500 flex-shrink-0" />
															<Link
																to={`/docker/images/${image.id}`}
																className="text-sm font-medium text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 truncate"
															>
																{image.repository}
															</Link>
														</div>
													</td>
													<td className="px-4 py-2 whitespace-nowrap">
														<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200">
															{image.tag}
														</span>
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-center">
														{getSourceBadge(image.source, image.repository)}
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-center text-sm text-secondary-900 dark:text-white">
														{image._count?.docker_containers || 0}
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-center">
														{image.hasUpdates ? (
															<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200">
																<AlertTriangle className="h-3 w-3 mr-1" />
																Available
															</span>
														) : (
															<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
																Up to date
															</span>
														)}
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-center">
														<div className="flex items-center justify-center gap-3">
															<Link
																to={`/docker/images/${image.id}`}
																className="text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 inline-flex items-center"
																title="View details"
															>
																<ExternalLink className="h-4 w-4" />
															</Link>
															<button
																type="button"
																onClick={() => setDeleteImageModal(image)}
																className="text-red-600 hover:text-red-900 dark:text-red-400 dark:hover:text-red-300 inline-flex items-center"
																title="Delete image from inventory"
															>
																<Trash2 className="h-4 w-4" />
															</button>
														</div>
													</td>
												</tr>
											))}
										</tbody>
									</table>
								</div>
							</>
						))}

					{/* Hosts Tab */}
					{activeTab === "hosts" &&
						(hostsLoading ? (
							<div className="text-center py-8">
								<RefreshCw className="h-8 w-8 animate-spin mx-auto text-secondary-400" />
								<p className="mt-2 text-sm text-secondary-500">
									Loading hosts...
								</p>
							</div>
						) : filteredHosts.length === 0 ? (
							<div className="text-center py-8">
								<Server className="h-12 w-12 mx-auto text-secondary-400" />
								<h3 className="mt-2 text-sm font-medium text-secondary-900 dark:text-white">
									No hosts found
								</h3>
								<p className="mt-1 text-sm text-secondary-500">
									{searchTerm
										? "Try adjusting your search filters"
										: "No hosts with Docker detected"}
								</p>
							</div>
						) : (
							<>
								{/* Mobile Card Layout */}
								<div className="md:hidden space-y-3 pb-4">
									{filteredHosts.map((host) => (
										<div key={host.id} className="card p-4 space-y-3">
											{/* Host Name */}
											<Link
												to={`/docker/hosts/${host.id}`}
												className="flex items-center gap-3"
											>
												<Server className="h-5 w-5 text-secondary-400 flex-shrink-0" />
												<div className="text-base font-semibold text-secondary-900 dark:text-white hover:text-primary-600 dark:hover:text-primary-400 truncate">
													{host.friendly_name || host.hostname}
												</div>
											</Link>

											{/* Stats */}
											<div className="grid grid-cols-3 gap-3 pt-2 border-t border-secondary-200 dark:border-secondary-600">
												<div className="text-center">
													<div className="text-xs text-secondary-500 dark:text-secondary-400">
														Containers
													</div>
													<div className="text-base font-semibold text-secondary-900 dark:text-white">
														{host.dockerStats?.totalContainers || 0}
													</div>
												</div>
												<div className="text-center">
													<div className="text-xs text-secondary-500 dark:text-secondary-400">
														Running
													</div>
													<div className="text-base font-semibold text-green-600 dark:text-green-400">
														{host.dockerStats?.runningContainers || 0}
													</div>
												</div>
												<div className="text-center">
													<div className="text-xs text-secondary-500 dark:text-secondary-400">
														Images
													</div>
													<div className="text-base font-semibold text-secondary-900 dark:text-white">
														{host.dockerStats?.totalImages || 0}
													</div>
												</div>
											</div>

											{/* Actions */}
											<div className="flex items-center justify-end pt-2 border-t border-secondary-200 dark:border-secondary-600">
												<Link
													to={`/docker/hosts/${host.id}`}
													className="text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 inline-flex items-center gap-1 text-sm"
													title="View details"
												>
													<ExternalLink className="h-4 w-4" />
													View
												</Link>
											</div>
										</div>
									))}
								</div>

								{/* Desktop Table Layout */}
								<div className="hidden md:block overflow-x-auto">
									<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
										<thead className="bg-secondary-50 dark:bg-secondary-700">
											<tr>
												<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													<button
														type="button"
														onClick={() => handleSort("name")}
														className="flex items-center gap-2 hover:text-secondary-700"
													>
														Host Name
														{getSortIcon("name")}
													</button>
												</th>
												<th className="px-4 py-2 text-center text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													<button
														type="button"
														onClick={() => handleSort("containers")}
														className="flex items-center gap-2 hover:text-secondary-700"
													>
														Containers
														{getSortIcon("containers")}
													</button>
												</th>
												<th className="px-4 py-2 text-center text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													Running
												</th>
												<th className="px-4 py-2 text-center text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													<button
														type="button"
														onClick={() => handleSort("images")}
														className="flex items-center gap-2 hover:text-secondary-700"
													>
														Images
														{getSortIcon("images")}
													</button>
												</th>
												<th className="px-4 py-2 text-center text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													Actions
												</th>
											</tr>
										</thead>
										<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
											{filteredHosts.map((host) => (
												<tr
													key={host.id}
													className="hover:bg-secondary-50 dark:hover:bg-secondary-700 transition-colors"
												>
													<td className="px-4 py-2 whitespace-nowrap">
														<div className="flex items-center gap-2">
															<Server className="h-4 w-4 text-secondary-400 dark:text-secondary-500 flex-shrink-0" />
															<Link
																to={`/docker/hosts/${host.id}`}
																className="text-sm font-medium text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 truncate"
															>
																{host.friendly_name || host.hostname}
															</Link>
														</div>
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-center text-sm text-secondary-900 dark:text-white">
														{host.dockerStats?.totalContainers || 0}
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-center text-sm text-green-600 dark:text-green-400 font-medium">
														{host.dockerStats?.runningContainers || 0}
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-center text-sm text-secondary-900 dark:text-white">
														{host.dockerStats?.totalImages || 0}
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-center">
														<Link
															to={`/docker/hosts/${host.id}`}
															className="text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 inline-flex items-center gap-1"
															title="View details"
														>
															<ExternalLink className="h-4 w-4" />
														</Link>
													</td>
												</tr>
											))}
										</tbody>
									</table>
								</div>
							</>
						))}

					{/* Volumes Tab */}
					{activeTab === "volumes" &&
						(volumesLoading ? (
							<div className="text-center py-8">
								<RefreshCw className="h-8 w-8 animate-spin mx-auto text-secondary-400" />
								<p className="mt-2 text-sm text-secondary-500">
									Loading volumes...
								</p>
							</div>
						) : filteredVolumes.length === 0 ? (
							<div className="text-center py-8">
								<HardDrive className="h-12 w-12 mx-auto text-secondary-400" />
								<h3 className="mt-2 text-sm font-medium text-secondary-900 dark:text-white">
									No volumes found
								</h3>
								<p className="mt-1 text-sm text-secondary-500">
									{searchTerm
										? "Try adjusting your search filters"
										: "No Docker volumes detected"}
								</p>
							</div>
						) : (
							<>
								{/* Mobile Card Layout */}
								<div className="md:hidden space-y-3 pb-4">
									{filteredVolumes.map((volume) => (
										<div key={volume.id} className="card p-4 space-y-3">
											{/* Volume Name */}
											<Link
												to={`/docker/volumes/${volume.id}`}
												className="flex items-center gap-3"
											>
												<HardDrive className="h-5 w-5 text-secondary-400 flex-shrink-0" />
												<div className="text-base font-semibold text-secondary-900 dark:text-white hover:text-primary-600 dark:hover:text-primary-400 truncate">
													{volume.name}
												</div>
											</Link>

											{/* Driver and In Use Status */}
											<div className="flex items-center justify-between gap-2">
												<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">
													{volume.driver}
												</span>
												{volume.ref_count > 0 ? (
													<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
														{volume.ref_count} container
														{volume.ref_count !== 1 ? "s" : ""}
													</span>
												) : (
													<span className="text-xs text-secondary-400 dark:text-secondary-500">
														Unused
													</span>
												)}
											</div>

											{/* Size and Host Info */}
											<div className="space-y-2 pt-2 border-t border-secondary-200 dark:border-secondary-600">
												{volume.size_bytes && (
													<div className="text-sm">
														<span className="text-secondary-500 dark:text-secondary-400">
															Size:&nbsp;
														</span>
														<span className="text-secondary-900 dark:text-white">
															{(
																Number(volume.size_bytes) /
																1024 /
																1024 /
																1024
															).toFixed(2)}{" "}
															GB
														</span>
													</div>
												)}
												<div className="text-sm">
													<span className="text-secondary-500 dark:text-secondary-400">
														Host:&nbsp;
													</span>
													<Link
														to={`/hosts/${volume.host_id}`}
														className="text-secondary-900 dark:text-white hover:text-primary-600 dark:hover:text-primary-400"
													>
														{volume.hosts?.friendly_name ||
															volume.hosts?.hostname ||
															"Unknown"}
													</Link>
												</div>
											</div>

											{/* Actions */}
											<div className="flex items-center justify-end gap-3 pt-2 border-t border-secondary-200 dark:border-secondary-600">
												<Link
													to={`/docker/volumes/${volume.id}`}
													className="text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 inline-flex items-center gap-1 text-sm"
													title="View details"
												>
													<ExternalLink className="h-4 w-4" />
													View
												</Link>
												<button
													type="button"
													onClick={() => setDeleteVolumeModal(volume)}
													className="text-red-600 hover:text-red-900 dark:text-red-400 dark:hover:text-red-300 inline-flex items-center gap-1 text-sm"
													title="Delete from inventory"
												>
													<Trash2 className="h-4 w-4" />
													Delete
												</button>
											</div>
										</div>
									))}
								</div>

								{/* Desktop Table Layout */}
								<div className="hidden md:block overflow-x-auto">
									<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
										<thead className="bg-secondary-50 dark:bg-secondary-700">
											<tr>
												<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													<button
														type="button"
														onClick={() => handleSort("name")}
														className="flex items-center gap-2 hover:text-secondary-700"
													>
														Volume Name
														{getSortIcon("name")}
													</button>
												</th>
												<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													<button
														type="button"
														onClick={() => handleSort("driver")}
														className="flex items-center gap-2 hover:text-secondary-700"
													>
														Driver
														{getSortIcon("driver")}
													</button>
												</th>
												<th className="px-4 py-2 text-center text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													<button
														type="button"
														onClick={() => handleSort("size")}
														className="flex items-center gap-2 hover:text-secondary-700"
													>
														Size
														{getSortIcon("size")}
													</button>
												</th>
												<th className="px-4 py-2 text-center text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													<button
														type="button"
														onClick={() => handleSort("ref_count")}
														className="flex items-center gap-2 hover:text-secondary-700"
													>
														In Use
														{getSortIcon("ref_count")}
													</button>
												</th>
												<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													<button
														type="button"
														onClick={() => handleSort("host")}
														className="flex items-center gap-2 hover:text-secondary-700"
													>
														Host
														{getSortIcon("host")}
													</button>
												</th>
												<th className="px-4 py-2 text-center text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													Actions
												</th>
											</tr>
										</thead>
										<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
											{filteredVolumes.map((volume) => (
												<tr
													key={volume.id}
													className="hover:bg-secondary-50 dark:hover:bg-secondary-700 transition-colors"
												>
													<td className="px-4 py-2 whitespace-nowrap">
														<div className="flex items-center gap-2">
															<HardDrive className="h-4 w-4 text-secondary-400 dark:text-secondary-500 flex-shrink-0" />
															<Link
																to={`/docker/volumes/${volume.id}`}
																className="text-sm font-medium text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 truncate"
															>
																{volume.name}
															</Link>
														</div>
													</td>
													<td className="px-4 py-2 whitespace-nowrap">
														<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">
															{volume.driver}
														</span>
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-center text-sm text-secondary-900 dark:text-white">
														{volume.size_bytes
															? `${(Number(volume.size_bytes) / 1024 / 1024 / 1024).toFixed(2)} GB`
															: "-"}
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-center text-sm text-secondary-900 dark:text-white">
														{volume.ref_count > 0 ? (
															<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
																{volume.ref_count} container
																{volume.ref_count !== 1 ? "s" : ""}
															</span>
														) : (
															<span className="text-secondary-400 dark:text-secondary-500">
																Unused
															</span>
														)}
													</td>
													<td className="px-4 py-2 whitespace-nowrap">
														<Link
															to={`/hosts/${volume.host_id}`}
															className="text-sm text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300"
														>
															{volume.hosts?.friendly_name ||
																volume.hosts?.hostname ||
																"Unknown"}
														</Link>
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-center">
														<div className="flex items-center justify-center gap-2">
															<Link
																to={`/docker/volumes/${volume.id}`}
																className="text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 inline-flex items-center gap-1"
																title="View details"
															>
																<ExternalLink className="h-4 w-4" />
															</Link>
															<button
																type="button"
																onClick={() => setDeleteVolumeModal(volume)}
																className="text-red-600 hover:text-red-900 dark:text-red-400 dark:hover:text-red-300"
																title="Delete from inventory"
															>
																<Trash2 className="h-4 w-4" />
															</button>
														</div>
													</td>
												</tr>
											))}
										</tbody>
									</table>
								</div>
							</>
						))}

					{/* Networks Tab */}
					{activeTab === "networks" &&
						(networksLoading ? (
							<div className="text-center py-8">
								<RefreshCw className="h-8 w-8 animate-spin mx-auto text-secondary-400" />
								<p className="mt-2 text-sm text-secondary-500">
									Loading networks...
								</p>
							</div>
						) : filteredNetworks.length === 0 ? (
							<div className="text-center py-8">
								<Network className="h-12 w-12 mx-auto text-secondary-400" />
								<h3 className="mt-2 text-sm font-medium text-secondary-900 dark:text-white">
									No networks found
								</h3>
								<p className="mt-1 text-sm text-secondary-500">
									{searchTerm
										? "Try adjusting your search filters"
										: "No Docker networks detected"}
								</p>
							</div>
						) : (
							<>
								{/* Mobile Card Layout */}
								<div className="md:hidden space-y-3 pb-4">
									{filteredNetworks.map((network) => (
										<div key={network.id} className="card p-4 space-y-3">
											{/* Network Name */}
											<Link
												to={`/docker/networks/${network.id}`}
												className="flex items-center gap-3"
											>
												<Network className="h-5 w-5 text-secondary-400 flex-shrink-0" />
												<div className="text-base font-semibold text-secondary-900 dark:text-white hover:text-primary-600 dark:hover:text-primary-400 truncate">
													{network.name}
												</div>
											</Link>

											{/* Driver, Scope, and Containers */}
											<div className="flex flex-wrap items-center gap-2">
												<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">
													{network.driver}
												</span>
												<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200">
													{network.scope}
												</span>
												{network.container_count > 0 && (
													<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
														{network.container_count} container
														{network.container_count !== 1 ? "s" : ""}
													</span>
												)}
											</div>

											{/* Flags */}
											{(network.internal ||
												network.ipv6_enabled ||
												network.ingress) && (
												<div className="flex items-center gap-1">
													{network.internal && (
														<span
															className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200"
															title="Internal"
														>
															I
														</span>
													)}
													{network.ipv6_enabled && (
														<span
															className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200"
															title="IPv6 Enabled"
														>
															6
														</span>
													)}
													{network.ingress && (
														<span
															className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200"
															title="Swarm Ingress"
														>
															S
														</span>
													)}
												</div>
											)}

											{/* Host Info */}
											<div className="pt-2 border-t border-secondary-200 dark:border-secondary-600">
												<div className="text-sm">
													<span className="text-secondary-500 dark:text-secondary-400">
														Host:&nbsp;
													</span>
													<Link
														to={`/hosts/${network.host_id}`}
														className="text-secondary-900 dark:text-white hover:text-primary-600 dark:hover:text-primary-400"
													>
														{network.hosts?.friendly_name ||
															network.hosts?.hostname ||
															"Unknown"}
													</Link>
												</div>
											</div>

											{/* Actions */}
											<div className="flex items-center justify-end gap-3 pt-2 border-t border-secondary-200 dark:border-secondary-600">
												<Link
													to={`/docker/networks/${network.id}`}
													className="text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 inline-flex items-center gap-1 text-sm"
													title="View details"
												>
													<ExternalLink className="h-4 w-4" />
													View
												</Link>
												<button
													type="button"
													onClick={() => setDeleteNetworkModal(network)}
													className="text-red-600 hover:text-red-900 dark:text-red-400 dark:hover:text-red-300 inline-flex items-center gap-1 text-sm"
													title="Delete from inventory"
												>
													<Trash2 className="h-4 w-4" />
													Delete
												</button>
											</div>
										</div>
									))}
								</div>

								{/* Desktop Table Layout */}
								<div className="hidden md:block overflow-x-auto">
									<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
										<thead className="bg-secondary-50 dark:bg-secondary-700">
											<tr>
												<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													<button
														type="button"
														onClick={() => handleSort("name")}
														className="flex items-center gap-2 hover:text-secondary-700"
													>
														Network Name
														{getSortIcon("name")}
													</button>
												</th>
												<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													<button
														type="button"
														onClick={() => handleSort("driver")}
														className="flex items-center gap-2 hover:text-secondary-700"
													>
														Driver
														{getSortIcon("driver")}
													</button>
												</th>
												<th className="px-4 py-2 text-center text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													Scope
												</th>
												<th className="px-4 py-2 text-center text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													<button
														type="button"
														onClick={() => handleSort("containers")}
														className="flex items-center gap-2 hover:text-secondary-700"
													>
														Containers
														{getSortIcon("containers")}
													</button>
												</th>
												<th className="px-4 py-2 text-center text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													Flags
												</th>
												<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													<button
														type="button"
														onClick={() => handleSort("host")}
														className="flex items-center gap-2 hover:text-secondary-700"
													>
														Host
														{getSortIcon("host")}
													</button>
												</th>
												<th className="px-4 py-2 text-center text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													Actions
												</th>
											</tr>
										</thead>
										<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
											{filteredNetworks.map((network) => (
												<tr
													key={network.id}
													className="hover:bg-secondary-50 dark:hover:bg-secondary-700 transition-colors"
												>
													<td className="px-4 py-2 whitespace-nowrap">
														<div className="flex items-center gap-2">
															<Network className="h-4 w-4 text-secondary-400 dark:text-secondary-500 flex-shrink-0" />
															<Link
																to={`/docker/networks/${network.id}`}
																className="text-sm font-medium text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 truncate"
															>
																{network.name}
															</Link>
														</div>
													</td>
													<td className="px-4 py-2 whitespace-nowrap">
														<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">
															{network.driver}
														</span>
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-center">
														<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200">
															{network.scope}
														</span>
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-center text-sm text-secondary-900 dark:text-white">
														{network.container_count > 0 ? (
															<span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
																{network.container_count}
															</span>
														) : (
															<span className="text-secondary-400 dark:text-secondary-500">
																0
															</span>
														)}
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-center">
														<div className="flex items-center justify-center gap-1">
															{network.internal && (
																<span
																	className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200"
																	title="Internal"
																>
																	I
																</span>
															)}
															{network.ipv6_enabled && (
																<span
																	className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200"
																	title="IPv6 Enabled"
																>
																	6
																</span>
															)}
															{network.ingress && (
																<span
																	className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200"
																	title="Swarm Ingress"
																>
																	S
																</span>
															)}
															{!network.internal &&
																!network.ipv6_enabled &&
																!network.ingress && (
																	<span className="text-secondary-400 dark:text-secondary-500 text-xs">
																		-
																	</span>
																)}
														</div>
													</td>
													<td className="px-4 py-2 whitespace-nowrap">
														<Link
															to={`/hosts/${network.host_id}`}
															className="text-sm text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300"
														>
															{network.hosts?.friendly_name ||
																network.hosts?.hostname ||
																"Unknown"}
														</Link>
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-center">
														<div className="flex items-center justify-center gap-2">
															<Link
																to={`/docker/networks/${network.id}`}
																className="text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 inline-flex items-center gap-1"
																title="View details"
															>
																<ExternalLink className="h-4 w-4" />
															</Link>
															<button
																type="button"
																onClick={() => setDeleteNetworkModal(network)}
																className="text-red-600 hover:text-red-900 dark:text-red-400 dark:hover:text-red-300"
																title="Delete from inventory"
															>
																<Trash2 className="h-4 w-4" />
															</button>
														</div>
													</td>
												</tr>
											))}
										</tbody>
									</table>
								</div>
							</>
						))}
				</div>
			</div>

			{/* Delete Container Modal */}

			{/* Delete Container Modal */}
			{deleteContainerModal && (
				<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
					<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 max-w-md w-full mx-4">
						<div className="flex items-start mb-4">
							<div className="flex-shrink-0">
								<AlertTriangle className="h-6 w-6 text-red-600" />
							</div>
							<div className="ml-3 flex-1">
								<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
									Delete Container
								</h3>
								<div className="mt-2 text-sm text-secondary-600 dark:text-secondary-300">
									<p className="mb-2">
										Are you sure you want to delete this container from the
										inventory?
									</p>
									<div className="bg-secondary-100 dark:bg-secondary-700 p-3 rounded-md">
										<p className="font-medium text-secondary-900 dark:text-white">
											{deleteContainerModal.name}
										</p>
										<p className="text-xs text-secondary-600 dark:text-secondary-400 mt-1">
											Image: {deleteContainerModal.image_name}:
											{deleteContainerModal.image_tag}
										</p>
										<p className="text-xs text-secondary-600 dark:text-secondary-400">
											Host:{" "}
											{deleteContainerModal.host?.friendly_name || "Unknown"}
										</p>
									</div>
									<p className="mt-3 text-red-600 dark:text-red-400 font-medium">
										⚠️ This only removes the container from PatchMon's inventory.
										It does NOT stop or delete the actual Docker container on
										the host.
									</p>
								</div>
							</div>
						</div>
						<div className="mt-5 sm:mt-4 sm:flex sm:flex-row-reverse gap-3">
							<button
								type="button"
								onClick={() =>
									deleteContainerMutation.mutate(deleteContainerModal.id)
								}
								disabled={deleteContainerMutation.isPending}
								className="w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-red-600 text-base font-medium text-white hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 sm:w-auto sm:text-sm disabled:opacity-50 disabled:cursor-not-allowed"
							>
								{deleteContainerMutation.isPending
									? "Deleting..."
									: "Delete from Inventory"}
							</button>
							<button
								type="button"
								onClick={() => setDeleteContainerModal(null)}
								disabled={deleteContainerMutation.isPending}
								className="mt-3 w-full inline-flex justify-center rounded-md border border-secondary-300 dark:border-secondary-600 shadow-sm px-4 py-2 bg-white dark:bg-secondary-700 text-base font-medium text-secondary-700 dark:text-secondary-200 hover:bg-secondary-50 dark:hover:bg-secondary-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:mt-0 sm:w-auto sm:text-sm disabled:opacity-50 disabled:cursor-not-allowed"
							>
								Cancel
							</button>
						</div>
					</div>
				</div>
			)}

			{/* Delete Image Modal */}
			{deleteImageModal && (
				<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
					<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 max-w-md w-full mx-4">
						<div className="flex items-start mb-4">
							<div className="flex-shrink-0">
								<AlertTriangle className="h-6 w-6 text-red-600" />
							</div>
							<div className="ml-3 flex-1">
								<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
									Delete Image
								</h3>
								<div className="mt-2 text-sm text-secondary-600 dark:text-secondary-300">
									<p className="mb-2">
										Are you sure you want to delete this image from the
										inventory?
									</p>
									<div className="bg-secondary-100 dark:bg-secondary-700 p-3 rounded-md">
										<p className="font-medium text-secondary-900 dark:text-white">
											{deleteImageModal.repository}:{deleteImageModal.tag}
										</p>
										<p className="text-xs text-secondary-600 dark:text-secondary-400 mt-1">
											Source: {deleteImageModal.source}
										</p>
										<p className="text-xs text-secondary-600 dark:text-secondary-400">
											Containers using this:{" "}
											{deleteImageModal._count?.docker_containers || 0}
										</p>
									</div>
									{deleteImageModal._count?.docker_containers > 0 ? (
										<p className="mt-3 text-red-600 dark:text-red-400 font-medium">
											⚠️ Cannot delete: This image is in use by{" "}
											{deleteImageModal._count.docker_containers} container(s).
											Delete the containers first.
										</p>
									) : (
										<p className="mt-3 text-red-600 dark:text-red-400 font-medium">
											⚠️ This only removes the image from PatchMon's inventory.
											It does NOT delete the actual Docker image from hosts.
										</p>
									)}
								</div>
							</div>
						</div>
						<div className="mt-5 sm:mt-4 sm:flex sm:flex-row-reverse gap-3">
							<button
								type="button"
								onClick={() => deleteImageMutation.mutate(deleteImageModal.id)}
								disabled={
									deleteImageMutation.isPending ||
									deleteImageModal._count?.docker_containers > 0
								}
								className="w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-red-600 text-base font-medium text-white hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 sm:w-auto sm:text-sm disabled:opacity-50 disabled:cursor-not-allowed"
							>
								{deleteImageMutation.isPending
									? "Deleting..."
									: "Delete from Inventory"}
							</button>
							<button
								type="button"
								onClick={() => setDeleteImageModal(null)}
								disabled={deleteImageMutation.isPending}
								className="mt-3 w-full inline-flex justify-center rounded-md border border-secondary-300 dark:border-secondary-600 shadow-sm px-4 py-2 bg-white dark:bg-secondary-700 text-base font-medium text-secondary-700 dark:text-secondary-200 hover:bg-secondary-50 dark:hover:bg-secondary-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:mt-0 sm:w-auto sm:text-sm disabled:opacity-50 disabled:cursor-not-allowed"
							>
								Cancel
							</button>
						</div>
					</div>
				</div>
			)}

			{/* Delete Volume Modal */}
			{deleteVolumeModal && (
				<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
					<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 max-w-md w-full mx-4">
						<div className="flex items-start mb-4">
							<div className="flex-shrink-0">
								<AlertTriangle className="h-6 w-6 text-red-600" />
							</div>
							<div className="ml-3 flex-1">
								<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
									Delete Volume
								</h3>
								<div className="mt-2 text-sm text-secondary-600 dark:text-secondary-300">
									<p className="mb-2">
										Are you sure you want to delete this volume from the
										inventory?
									</p>
									<div className="bg-secondary-100 dark:bg-secondary-700 p-3 rounded-md">
										<p className="font-medium text-secondary-900 dark:text-white">
											{deleteVolumeModal.name}
										</p>
										<p className="text-xs text-secondary-600 dark:text-secondary-400 mt-1">
											Driver: {deleteVolumeModal.driver}
										</p>
										<p className="text-xs text-secondary-600 dark:text-secondary-400">
											Host:{" "}
											{deleteVolumeModal.hosts?.friendly_name ||
												deleteVolumeModal.hosts?.hostname ||
												"Unknown"}
										</p>
										{deleteVolumeModal.ref_count > 0 && (
											<p className="text-xs text-secondary-600 dark:text-secondary-400">
												In use by: {deleteVolumeModal.ref_count} container
												{deleteVolumeModal.ref_count !== 1 ? "s" : ""}
											</p>
										)}
									</div>
									<p className="mt-3 text-red-600 dark:text-red-400 font-medium">
										⚠️ This only removes the volume from PatchMon's inventory. It
										does NOT delete the actual Docker volume from the host.
									</p>
								</div>
							</div>
						</div>
						<div className="mt-5 sm:mt-4 sm:flex sm:flex-row-reverse gap-3">
							<button
								type="button"
								onClick={() =>
									deleteVolumeMutation.mutate(deleteVolumeModal.id)
								}
								disabled={deleteVolumeMutation.isPending}
								className="w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-red-600 text-base font-medium text-white hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 sm:w-auto sm:text-sm disabled:opacity-50 disabled:cursor-not-allowed"
							>
								{deleteVolumeMutation.isPending
									? "Deleting..."
									: "Delete from Inventory"}
							</button>
							<button
								type="button"
								onClick={() => setDeleteVolumeModal(null)}
								disabled={deleteVolumeMutation.isPending}
								className="mt-3 w-full inline-flex justify-center rounded-md border border-secondary-300 dark:border-secondary-600 shadow-sm px-4 py-2 bg-white dark:bg-secondary-700 text-base font-medium text-secondary-700 dark:text-secondary-200 hover:bg-secondary-50 dark:hover:bg-secondary-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:mt-0 sm:w-auto sm:text-sm disabled:opacity-50 disabled:cursor-not-allowed"
							>
								Cancel
							</button>
						</div>
					</div>
				</div>
			)}

			{/* Delete Network Modal */}
			{deleteNetworkModal && (
				<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
					<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 max-w-md w-full mx-4">
						<div className="flex items-start mb-4">
							<div className="flex-shrink-0">
								<AlertTriangle className="h-6 w-6 text-red-600" />
							</div>
							<div className="ml-3 flex-1">
								<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
									Delete Network
								</h3>
								<div className="mt-2 text-sm text-secondary-600 dark:text-secondary-300">
									<p className="mb-2">
										Are you sure you want to delete this network from the
										inventory?
									</p>
									<div className="bg-secondary-100 dark:bg-secondary-700 p-3 rounded-md">
										<p className="font-medium text-secondary-900 dark:text-white">
											{deleteNetworkModal.name}
										</p>
										<p className="text-xs text-secondary-600 dark:text-secondary-400 mt-1">
											Driver: {deleteNetworkModal.driver}
										</p>
										<p className="text-xs text-secondary-600 dark:text-secondary-400">
											Scope: {deleteNetworkModal.scope}
										</p>
										<p className="text-xs text-secondary-600 dark:text-secondary-400">
											Host:{" "}
											{deleteNetworkModal.hosts?.friendly_name ||
												deleteNetworkModal.hosts?.hostname ||
												"Unknown"}
										</p>
										{deleteNetworkModal.container_count > 0 && (
											<p className="text-xs text-secondary-600 dark:text-secondary-400">
												Connected containers:{" "}
												{deleteNetworkModal.container_count}
											</p>
										)}
									</div>
									<p className="mt-3 text-red-600 dark:text-red-400 font-medium">
										⚠️ This only removes the network from PatchMon's inventory.
										It does NOT delete the actual Docker network from the host.
									</p>
								</div>
							</div>
						</div>
						<div className="mt-5 sm:mt-4 sm:flex sm:flex-row-reverse gap-3">
							<button
								type="button"
								onClick={() =>
									deleteNetworkMutation.mutate(deleteNetworkModal.id)
								}
								disabled={deleteNetworkMutation.isPending}
								className="w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-red-600 text-base font-medium text-white hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 sm:w-auto sm:text-sm disabled:opacity-50 disabled:cursor-not-allowed"
							>
								{deleteNetworkMutation.isPending
									? "Deleting..."
									: "Delete from Inventory"}
							</button>
							<button
								type="button"
								onClick={() => setDeleteNetworkModal(null)}
								disabled={deleteNetworkMutation.isPending}
								className="mt-3 w-full inline-flex justify-center rounded-md border border-secondary-300 dark:border-secondary-600 shadow-sm px-4 py-2 bg-white dark:bg-secondary-700 text-base font-medium text-secondary-700 dark:text-secondary-200 hover:bg-secondary-50 dark:hover:bg-secondary-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:mt-0 sm:w-auto sm:text-sm disabled:opacity-50 disabled:cursor-not-allowed"
							>
								Cancel
							</button>
						</div>
					</div>
				</div>
			)}
		</div>
	);
};

export default Docker;
