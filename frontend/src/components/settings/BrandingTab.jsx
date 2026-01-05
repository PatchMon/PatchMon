import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, Image, RotateCcw, Upload, X } from "lucide-react";
import { useState } from "react";
import { settingsAPI } from "../../utils/api";

const BrandingTab = () => {
	// Logo management state
	const [logoUploadState, setLogoUploadState] = useState({
		dark: { uploading: false, error: null },
		light: { uploading: false, error: null },
		favicon: { uploading: false, error: null },
	});
	const [showLogoUploadModal, setShowLogoUploadModal] = useState(false);
	const [selectedLogoType, setSelectedLogoType] = useState("dark");

	const queryClient = useQueryClient();

	// Fetch current settings
	const {
		data: settings,
		isLoading,
		error,
	} = useQuery({
		queryKey: ["settings"],
		queryFn: () => settingsAPI.get().then((res) => res.data),
	});

	// Helper function to encode logo path for URLs (handles spaces and special characters)
	const encodeLogoPath = (path) => {
		if (!path) return path;
		// Split path into directory and filename
		const parts = path.split("/");
		const filename = parts.pop();
		const directory = parts.join("/");
		// Encode only the filename part, keep directory structure
		return directory
			? `${directory}/${encodeURIComponent(filename)}`
			: encodeURIComponent(filename);
	};

	// Logo upload mutation
	const uploadLogoMutation = useMutation({
		mutationFn: async ({ logoType, fileContent, fileName }) => {
			const res = await fetch("/api/v1/settings/logos/upload", {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
				},
				credentials: "include",
				body: JSON.stringify({ logoType, fileContent, fileName }),
			});
			const data = await res.json();
			if (!res.ok) {
				throw new Error(data.error || data.details || "Failed to upload logo");
			}
			return data;
		},
		onSuccess: async (data, variables) => {
			// Invalidate and refetch settings to get updated timestamp
			queryClient.invalidateQueries(["settings"]);
			// Wait for refetch to complete before closing modal
			try {
				await queryClient.refetchQueries(["settings"]);
			} catch (error) {
				// Continue anyway - settings will update on next render
			}
			setLogoUploadState((prev) => ({
				...prev,
				[variables.logoType]: { uploading: false, error: null },
			}));
			setShowLogoUploadModal(false);
		},
		onError: (error, variables) => {
			console.error("Upload logo error:", error);
			setLogoUploadState((prev) => ({
				...prev,
				[variables.logoType]: {
					uploading: false,
					error: error.message || "Failed to upload logo",
				},
			}));
		},
	});

	// Logo reset mutation
	const resetLogoMutation = useMutation({
		mutationFn: (logoType) =>
			fetch("/api/v1/settings/logos/reset", {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
				},
				credentials: "include",
				body: JSON.stringify({ logoType }),
			}).then((res) => res.json()),
		onSuccess: () => {
			queryClient.invalidateQueries(["settings"]);
		},
		onError: (error) => {
			console.error("Reset logo error:", error);
		},
	});

	if (isLoading) {
		return (
			<div className="flex items-center justify-center h-64">
				<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
			</div>
		);
	}

	if (error) {
		return (
			<div className="bg-red-50 dark:bg-red-900 border border-red-200 dark:border-red-700 rounded-md p-4">
				<div className="flex">
					<AlertCircle className="h-5 w-5 text-red-400 dark:text-red-300" />
					<div className="ml-3">
						<h3 className="text-sm font-medium text-red-800 dark:text-red-200">
							Error loading settings
						</h3>
						<p className="mt-1 text-sm text-red-700 dark:text-red-300">
							{error.response?.data?.error || "Failed to load settings"}
						</p>
					</div>
				</div>
			</div>
		);
	}

	return (
		<div className="space-y-8">
			{/* Header */}
			<div>
				<div className="flex items-center mb-6">
					<Image className="h-6 w-6 text-primary-600 mr-3" />
					<h2 className="text-xl font-semibold text-secondary-900 dark:text-white">
						Logo & Branding
					</h2>
				</div>
				<p className="text-sm text-secondary-500 dark:text-secondary-300 mb-6">
					Customize your PatchMonEnhanced installation with custom logos and favicon.
					These will be displayed throughout the application.
				</p>
			</div>

			{/* Logo Section Header */}
			<div className="flex items-center mb-4">
				<Image className="h-5 w-5 text-primary-600 mr-2" />
				<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
					Logos
				</h3>
			</div>

			<div className="grid grid-cols-1 md:grid-cols-3 gap-6">
				{/* Dark Logo */}
				<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 border border-secondary-200 dark:border-secondary-600">
					<h4 className="text-sm font-medium text-secondary-900 dark:text-white mb-4">
						Dark Logo
					</h4>
					<div className="flex items-center justify-center p-4 bg-secondary-50 dark:bg-secondary-700 rounded-lg mb-4">
						<img
							key={`dark-${settings?.logo_dark}-${settings?.updated_at}`}
							src={`${encodeLogoPath(settings?.logo_dark || "/assets/logo_dark.png")}?v=${
								settings?.updated_at
									? new Date(settings.updated_at).getTime()
									: Date.now()
							}`}
							alt="Dark Logo"
							className="max-h-16 max-w-full object-contain"
							onError={(e) => {
								e.target.src = "/assets/logo_dark.png";
							}}
						/>
					</div>
					<p className="text-xs text-secondary-600 dark:text-secondary-400 mb-4 truncate">
						{settings?.logo_dark
							? settings.logo_dark.split("/").pop()
							: "logo_dark.png (Default)"}
					</p>
					<div className="space-y-2">
						<button
							type="button"
							onClick={() => {
								setSelectedLogoType("dark");
								setShowLogoUploadModal(true);
							}}
							disabled={logoUploadState.dark.uploading}
							className="w-full btn-outline flex items-center justify-center gap-2"
						>
							{logoUploadState.dark.uploading ? (
								<>
									<div className="animate-spin rounded-full h-4 w-4 border-b-2 border-current"></div>
									Uploading...
								</>
							) : (
								<>
									<Upload className="h-4 w-4" />
									Upload Dark Logo
								</>
							)}
						</button>
						{settings?.logo_dark && (
							<button
								type="button"
								onClick={() => resetLogoMutation.mutate("dark")}
								disabled={resetLogoMutation.isPending}
								className="w-full btn-outline flex items-center justify-center gap-2 text-orange-600 hover:text-orange-700 border-orange-300 hover:border-orange-400"
							>
								<RotateCcw className="h-4 w-4" />
								Reset to Default
							</button>
						)}
					</div>
					{logoUploadState.dark.error && (
						<p className="text-xs text-red-600 dark:text-red-400 mt-2">
							{logoUploadState.dark.error}
						</p>
					)}
				</div>

				{/* Light Logo */}
				<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 border border-secondary-200 dark:border-secondary-600">
					<h4 className="text-sm font-medium text-secondary-900 dark:text-white mb-4">
						Light Logo
					</h4>
					<div className="flex items-center justify-center p-4 bg-secondary-50 dark:bg-secondary-700 rounded-lg mb-4">
						<img
							key={`light-${settings?.logo_light}-${settings?.updated_at}`}
							src={`${encodeLogoPath(settings?.logo_light || "/assets/logo_light.png")}?v=${
								settings?.updated_at
									? new Date(settings.updated_at).getTime()
									: Date.now()
							}`}
							alt="Light Logo"
							className="max-h-16 max-w-full object-contain"
							onError={(e) => {
								e.target.src = "/assets/logo_light.png";
							}}
						/>
					</div>
					<p className="text-xs text-secondary-600 dark:text-secondary-400 mb-4 truncate">
						{settings?.logo_light
							? settings.logo_light.split("/").pop()
							: "logo_light.png (Default)"}
					</p>
					<div className="space-y-2">
						<button
							type="button"
							onClick={() => {
								setSelectedLogoType("light");
								setShowLogoUploadModal(true);
							}}
							disabled={logoUploadState.light.uploading}
							className="w-full btn-outline flex items-center justify-center gap-2"
						>
							{logoUploadState.light.uploading ? (
								<>
									<div className="animate-spin rounded-full h-4 w-4 border-b-2 border-current"></div>
									Uploading...
								</>
							) : (
								<>
									<Upload className="h-4 w-4" />
									Upload Light Logo
								</>
							)}
						</button>
						{settings?.logo_light && (
							<button
								type="button"
								onClick={() => resetLogoMutation.mutate("light")}
								disabled={resetLogoMutation.isPending}
								className="w-full btn-outline flex items-center justify-center gap-2 text-orange-600 hover:text-orange-700 border-orange-300 hover:border-orange-400"
							>
								<RotateCcw className="h-4 w-4" />
								Reset to Default
							</button>
						)}
					</div>
					{logoUploadState.light.error && (
						<p className="text-xs text-red-600 dark:text-red-400 mt-2">
							{logoUploadState.light.error}
						</p>
					)}
				</div>

				{/* Favicon */}
				<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 border border-secondary-200 dark:border-secondary-600">
					<h4 className="text-sm font-medium text-secondary-900 dark:text-white mb-4">
						Favicon
					</h4>
					<div className="flex items-center justify-center p-4 bg-secondary-50 dark:bg-secondary-700 rounded-lg mb-4">
						<img
							key={`favicon-${settings?.favicon}-${settings?.updated_at}`}
							src={`${encodeLogoPath(settings?.favicon || "/assets/favicon.svg")}?v=${
								settings?.updated_at
									? new Date(settings.updated_at).getTime()
									: Date.now()
							}`}
							alt="Favicon"
							className="h-8 w-8 object-contain"
							onError={(e) => {
								e.target.src = "/assets/favicon.svg";
							}}
						/>
					</div>
					<p className="text-xs text-secondary-600 dark:text-secondary-400 mb-4 truncate">
						{settings?.favicon
							? settings.favicon.split("/").pop()
							: "favicon.svg (Default)"}
					</p>
					<div className="space-y-2">
						<button
							type="button"
							onClick={() => {
								setSelectedLogoType("favicon");
								setShowLogoUploadModal(true);
							}}
							disabled={logoUploadState.favicon.uploading}
							className="w-full btn-outline flex items-center justify-center gap-2"
						>
							{logoUploadState.favicon.uploading ? (
								<>
									<div className="animate-spin rounded-full h-4 w-4 border-b-2 border-current"></div>
									Uploading...
								</>
							) : (
								<>
									<Upload className="h-4 w-4" />
									Upload Favicon
								</>
							)}
						</button>
						{settings?.favicon && (
							<button
								type="button"
								onClick={() => resetLogoMutation.mutate("favicon")}
								disabled={resetLogoMutation.isPending}
								className="w-full btn-outline flex items-center justify-center gap-2 text-orange-600 hover:text-orange-700 border-orange-300 hover:border-orange-400"
							>
								<RotateCcw className="h-4 w-4" />
								Reset to Default
							</button>
						)}
					</div>
					{logoUploadState.favicon.error && (
						<p className="text-xs text-red-600 dark:text-red-400 mt-2">
							{logoUploadState.favicon.error}
						</p>
					)}
				</div>
			</div>

			{/* Usage Instructions */}
			<div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-700 rounded-md p-4 mt-6">
				<div className="flex">
					<Image className="h-5 w-5 text-blue-400 dark:text-blue-300" />
					<div className="ml-3">
						<h3 className="text-sm font-medium text-blue-800 dark:text-blue-200">
							Logo Usage
						</h3>
						<div className="mt-2 text-sm text-blue-700 dark:text-blue-300">
							<p className="mb-2">
								These logos are used throughout the application:
							</p>
							<ul className="list-disc list-inside space-y-1">
								<li>
									<strong>Dark Logo:</strong> Used in dark mode and on light
									backgrounds
								</li>
								<li>
									<strong>Light Logo:</strong> Used in light mode and on dark
									backgrounds
								</li>
								<li>
									<strong>Favicon:</strong> Used as the browser tab icon (SVG
									recommended)
								</li>
							</ul>
							<p className="mt-3 text-xs">
								<strong>Supported formats:</strong> PNG, JPG, SVG |{" "}
								<strong>Max size:</strong> 5MB |{" "}
								<strong>Recommended sizes:</strong> 200x60px for logos, 32x32px
								for favicon.
							</p>
						</div>
					</div>
				</div>
			</div>

			{/* Logo Upload Modal */}
			{showLogoUploadModal && (
				<LogoUploadModal
					isOpen={showLogoUploadModal}
					onClose={() => setShowLogoUploadModal(false)}
					onSubmit={uploadLogoMutation.mutate}
					isLoading={uploadLogoMutation.isPending}
					error={uploadLogoMutation.error}
					logoType={selectedLogoType}
				/>
			)}
		</div>
	);
};

// Logo Upload Modal Component
const LogoUploadModal = ({
	isOpen,
	onClose,
	onSubmit,
	isLoading,
	error,
	logoType,
}) => {
	const [selectedFile, setSelectedFile] = useState(null);
	const [previewUrl, setPreviewUrl] = useState(null);
	const [uploadError, setUploadError] = useState("");

	const handleFileSelect = (e) => {
		const file = e.target.files[0];
		if (file) {
			// Validate file type
			const allowedTypes = [
				"image/png",
				"image/jpeg",
				"image/jpg",
				"image/svg+xml",
			];
			if (!allowedTypes.includes(file.type)) {
				setUploadError("Please select a PNG, JPG, or SVG file");
				return;
			}

			// Validate file size (5MB limit)
			if (file.size > 5 * 1024 * 1024) {
				setUploadError("File size must be less than 5MB");
				return;
			}

			setSelectedFile(file);
			setUploadError("");

			// Create preview URL
			const url = URL.createObjectURL(file);
			setPreviewUrl(url);
		}
	};

	const handleSubmit = (e) => {
		e.preventDefault();
		setUploadError("");

		if (!selectedFile) {
			setUploadError("Please select a file");
			return;
		}

		// Convert file to base64
		const reader = new FileReader();
		reader.onload = (event) => {
			const base64 = event.target.result;
			onSubmit({
				logoType,
				fileContent: base64,
				fileName: selectedFile.name,
			});
		};
		reader.readAsDataURL(selectedFile);
	};

	const handleClose = () => {
		setSelectedFile(null);
		setPreviewUrl(null);
		setUploadError("");
		onClose();
	};

	if (!isOpen) return null;

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
			<div className="bg-white dark:bg-secondary-800 rounded-lg shadow-xl max-w-2xl w-full mx-4 max-h-[90vh] overflow-y-auto">
				<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-600">
					<div className="flex items-center justify-between">
						<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
							Upload{" "}
							{logoType === "favicon"
								? "Favicon"
								: `${logoType.charAt(0).toUpperCase() + logoType.slice(1)} Logo`}
						</h3>
						<button
							type="button"
							onClick={handleClose}
							className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300"
						>
							<X className="h-5 w-5" />
						</button>
					</div>
				</div>

				<form onSubmit={handleSubmit} className="px-6 py-4">
					<div className="space-y-4">
						<div>
							<label className="block">
								<span className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2">
									Select File
								</span>
								<input
									type="file"
									accept="image/png,image/jpeg,image/jpg,image/svg+xml"
									onChange={handleFileSelect}
									className="block w-full text-sm text-secondary-500 dark:text-secondary-400 file:mr-4 file:py-2 file:px-4 file:rounded-md file:border-0 file:text-sm file:font-medium file:bg-primary-50 file:text-primary-700 hover:file:bg-primary-100 dark:file:bg-primary-900 dark:file:text-primary-200"
								/>
							</label>
							<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
								Supported formats: PNG, JPG, SVG. Max size: 5MB.
								{logoType === "favicon"
									? " Recommended: 32x32px SVG."
									: " Recommended: 200x60px."}
							</p>
						</div>

						{previewUrl && (
							<div>
								<div className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2">
									Preview
								</div>
								<div className="flex items-center justify-center p-4 bg-white dark:bg-secondary-800 rounded-lg border border-secondary-200 dark:border-secondary-600">
									<img
										src={previewUrl}
										alt="Preview"
										className={`object-contain ${
											logoType === "favicon" ? "h-8 w-8" : "max-h-16 max-w-full"
										}`}
									/>
								</div>
							</div>
						)}

						{(uploadError || error) && (
							<div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md p-3">
								<p className="text-sm text-red-800 dark:text-red-200">
									{uploadError ||
										error?.response?.data?.error ||
										error?.message}
								</p>
							</div>
						)}

						<div className="bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-md p-3">
							<div className="flex">
								<AlertCircle className="h-4 w-4 text-yellow-600 dark:text-yellow-400 mr-2 mt-0.5" />
								<div className="text-sm text-yellow-800 dark:text-yellow-200">
									<p className="font-medium">Important:</p>
									<ul className="mt-1 list-disc list-inside space-y-1">
										<li>This will replace the current {logoType} logo</li>
										<li>A backup will be created automatically</li>
										<li>The change will be applied immediately</li>
									</ul>
								</div>
							</div>
						</div>
					</div>

					<div className="flex justify-end gap-3 mt-6">
						<button type="button" onClick={handleClose} className="btn-outline">
							Cancel
						</button>
						<button
							type="submit"
							disabled={isLoading || !selectedFile}
							className="btn-primary"
						>
							{isLoading ? "Uploading..." : "Upload Logo"}
						</button>
					</div>
				</form>
			</div>
		</div>
	);
};

export default BrandingTab;
