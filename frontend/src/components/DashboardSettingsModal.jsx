import {
	closestCenter,
	DndContext,
	KeyboardSensor,
	PointerSensor,
	useSensor,
	useSensors,
} from "@dnd-kit/core";
import {
	arrayMove,
	SortableContext,
	sortableKeyboardCoordinates,
	useSortable,
	verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	Eye,
	EyeOff,
	GripVertical,
	RotateCcw,
	Save,
	Settings as SettingsIcon,
	X,
} from "lucide-react";
import { useEffect, useState } from "react";
import { dashboardPreferencesAPI } from "../utils/api";

// Sortable Card Item Component
const SortableCardItem = ({ card, onToggle }) => {
	const {
		attributes,
		listeners,
		setNodeRef,
		transform,
		transition,
		isDragging,
	} = useSortable({
		id: card.cardId,
	});

	const style = {
		transform: CSS.Transform.toString(transform),
		transition,
		opacity: isDragging ? 0.5 : 1,
	};

	return (
		<div
			ref={setNodeRef}
			style={style}
			className={`flex items-center justify-between p-3 bg-white dark:bg-secondary-800 border border-secondary-200 dark:border-secondary-600 rounded-lg ${
				isDragging ? "shadow-lg" : "shadow-sm"
			}`}
		>
			<div className="flex items-center gap-3">
				<button
					{...attributes}
					{...listeners}
					className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300 cursor-grab active:cursor-grabbing"
				>
					<GripVertical className="h-4 w-4" />
				</button>
				<div className="flex items-center gap-2">
					<div className="text-sm font-medium text-secondary-900 dark:text-white">
						{card.title}
						{card.typeLabel ? (
							<span className="ml-2 text-xs font-normal text-secondary-500 dark:text-secondary-400">
								({card.typeLabel})
							</span>
						) : null}
					</div>
				</div>
			</div>

			<button
				type="button"
				onClick={() => onToggle(card.cardId)}
				className={`flex items-center gap-1 px-2 py-1 rounded text-xs font-medium transition-colors ${
					card.enabled
						? "bg-green-100 dark:bg-green-900 text-green-800 dark:text-green-200 hover:bg-green-200 dark:hover:bg-green-800"
						: "bg-secondary-100 dark:bg-secondary-700 text-secondary-600 dark:text-secondary-300 hover:bg-secondary-200 dark:hover:bg-secondary-600"
				}`}
			>
				{card.enabled ? (
					<>
						<Eye className="h-3 w-3" />
						Visible
					</>
				) : (
					<>
						<EyeOff className="h-3 w-3" />
						Hidden
					</>
				)}
			</button>
		</div>
	);
};

const DashboardSettingsModal = ({ isOpen, onClose }) => {
	const [cards, setCards] = useState([]);
	const [hasChanges, setHasChanges] = useState(false);
	const queryClient = useQueryClient();

	const sensors = useSensors(
		useSensor(PointerSensor),
		useSensor(KeyboardSensor, {
			coordinateGetter: sortableKeyboardCoordinates,
		}),
	);

	// Fetch user's dashboard preferences
	const { data: preferences, isLoading } = useQuery({
		queryKey: ["dashboardPreferences"],
		queryFn: () => dashboardPreferencesAPI.get().then((res) => res.data),
		enabled: isOpen,
	});

	// Fetch default card configuration
	const { data: defaultCards } = useQuery({
		queryKey: ["dashboardDefaultCards"],
		queryFn: () =>
			dashboardPreferencesAPI.getDefaults().then((res) => res.data),
		enabled: isOpen,
	});

	// Update preferences mutation
	const updatePreferencesMutation = useMutation({
		mutationFn: (preferences) => dashboardPreferencesAPI.update(preferences),
		onSuccess: (response) => {
			// Optimistically update the query cache with the correct data structure
			queryClient.setQueryData(
				["dashboardPreferences"],
				response.data.preferences,
			);
			// Also invalidate to ensure fresh data
			queryClient.invalidateQueries(["dashboardPreferences"]);
			setHasChanges(false);
			onClose();
		},
		onError: (error) => {
			console.error("Failed to update dashboard preferences:", error);
		},
	});

	// Initialize cards when preferences or defaults are loaded
	useEffect(() => {
		if (preferences && defaultCards) {
			// Normalize server preferences (snake_case -> camelCase)
			const normalizedPreferences = preferences.map((p) => ({
				cardId: p.cardId ?? p.card_id,
				enabled: p.enabled,
				order: p.order,
			}));

			const typeLabelFor = (cardId) => {
				if (
					[
						"totalHosts",
						"hostsNeedingUpdates",
						"totalOutdatedPackages",
						"securityUpdates",
						"upToDateHosts",
						"totalHostGroups",
						"totalUsers",
						"totalRepos",
					].includes(cardId)
				)
					return "Top card";
				if (cardId === "osDistribution") return "Pie chart";
				if (cardId === "osDistributionBar") return "Bar chart";
				if (cardId === "osDistributionDoughnut") return "Doughnut chart";
				if (cardId === "updateStatus") return "Pie chart";
				if (cardId === "packagePriority") return "Pie chart";
				if (cardId === "recentUsers") return "Table";
				if (cardId === "recentCollection") return "Table";
				if (cardId === "quickStats") return "Wide card";
				if (cardId === "complianceHostStatus") return "Bar chart";
				if (cardId === "complianceOpenSCAPDistribution")
					return "Doughnut chart";
				if (cardId === "complianceFailuresBySeverity") return "Doughnut chart";
				if (cardId === "complianceProfilesInUse") return "Bar chart";
				if (cardId === "complianceLastScanAge") return "Doughnut chart";
				if (cardId === "complianceTrendLine") return "Line chart";
				return undefined;
			};

			// Merge user preferences with default cards
			const mergedCards = defaultCards
				.map((defaultCard) => {
					const userPreference = normalizedPreferences.find(
						(p) => p.cardId === defaultCard.cardId,
					);
					return {
						...defaultCard,
						enabled: userPreference
							? userPreference.enabled
							: defaultCard.enabled,
						order: userPreference ? userPreference.order : defaultCard.order,
						typeLabel: typeLabelFor(defaultCard.cardId),
					};
				})
				.sort((a, b) => a.order - b.order);

			setCards(mergedCards);
		}
	}, [preferences, defaultCards]);

	const handleDragEnd = (event) => {
		const { active, over } = event;

		if (active.id !== over.id) {
			setCards((items) => {
				const oldIndex = items.findIndex((item) => item.cardId === active.id);
				const newIndex = items.findIndex((item) => item.cardId === over.id);

				const newItems = arrayMove(items, oldIndex, newIndex);

				// Update order values
				return newItems.map((item, index) => ({
					...item,
					order: index,
				}));
			});
			setHasChanges(true);
		}
	};

	const handleToggle = (cardId) => {
		setCards((prevCards) =>
			prevCards.map((card) =>
				card.cardId === cardId ? { ...card, enabled: !card.enabled } : card,
			),
		);
		setHasChanges(true);
	};

	const handleSave = () => {
		const preferences = cards.map((card) => ({
			cardId: card.cardId,
			enabled: card.enabled,
			order: card.order,
		}));

		updatePreferencesMutation.mutate(preferences);
	};

	const handleReset = () => {
		if (defaultCards) {
			const resetCards = defaultCards.map((card) => ({
				...card,
				enabled: true,
				order: card.order,
			}));
			setCards(resetCards);
			setHasChanges(true);
		}
	};

	if (!isOpen) return null;

	return (
		<div className="fixed inset-0 z-50 overflow-y-auto">
			<div className="flex items-center justify-center min-h-screen pt-4 px-4 pb-20 text-center sm:block sm:p-0">
				<button
					type="button"
					className="fixed inset-0 bg-secondary-500 bg-opacity-75 transition-opacity cursor-default"
					onClick={onClose}
					aria-label="Close modal"
				/>

				<div className="inline-block align-bottom bg-white dark:bg-secondary-800 rounded-lg text-left overflow-hidden shadow-xl transform transition-all sm:my-8 sm:align-middle sm:max-w-lg sm:w-full">
					<div className="bg-white dark:bg-secondary-800 px-4 pt-5 pb-4 sm:p-6 sm:pb-4">
						<div className="flex items-center justify-between mb-4">
							<div className="flex items-center gap-2">
								<SettingsIcon className="h-5 w-5 text-primary-600" />
								<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
									Dashboard Settings
								</h3>
							</div>
							<button
								type="button"
								onClick={onClose}
								className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300"
							>
								<X className="h-5 w-5" />
							</button>
						</div>

						<p className="text-sm text-secondary-600 dark:text-secondary-400 mb-6">
							Customize your dashboard by reordering cards and toggling their
							visibility. Drag cards to reorder them, and click the visibility
							toggle to show/hide cards.
						</p>

						{isLoading ? (
							<div className="flex items-center justify-center py-8">
								<div className="animate-spin rounded-full h-6 w-6 border-b-2 border-primary-600"></div>
							</div>
						) : (
							<DndContext
								sensors={sensors}
								collisionDetection={closestCenter}
								onDragEnd={handleDragEnd}
							>
								<SortableContext
									items={cards.map((card) => card.cardId)}
									strategy={verticalListSortingStrategy}
								>
									<div className="space-y-2 max-h-96 overflow-y-auto">
										{cards.map((card) => (
											<SortableCardItem
												key={card.cardId}
												card={card}
												onToggle={handleToggle}
											/>
										))}
									</div>
								</SortableContext>
							</DndContext>
						)}
					</div>

					<div className="bg-secondary-50 dark:bg-secondary-700 px-4 py-3 sm:px-6 sm:flex sm:flex-row-reverse">
						<button
							type="button"
							onClick={handleSave}
							disabled={!hasChanges || updatePreferencesMutation.isPending}
							className={`w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 text-base font-medium text-white sm:ml-3 sm:w-auto sm:text-sm ${
								!hasChanges || updatePreferencesMutation.isPending
									? "bg-secondary-400 cursor-not-allowed"
									: "bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
							}`}
						>
							{updatePreferencesMutation.isPending ? (
								<>
									<div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
									Saving...
								</>
							) : (
								<>
									<Save className="h-4 w-4 mr-2" />
									Save Changes
								</>
							)}
						</button>

						<button
							type="button"
							onClick={handleReset}
							className="mt-3 w-full inline-flex justify-center rounded-md border border-secondary-300 dark:border-secondary-600 shadow-sm px-4 py-2 bg-white dark:bg-secondary-800 text-base font-medium text-secondary-700 dark:text-secondary-200 hover:bg-secondary-50 dark:hover:bg-secondary-700 sm:mt-0 sm:ml-3 sm:w-auto sm:text-sm"
						>
							<RotateCcw className="h-4 w-4 mr-2" />
							Reset to Defaults
						</button>

						<button
							type="button"
							onClick={onClose}
							className="mt-3 w-full inline-flex justify-center rounded-md border border-secondary-300 dark:border-secondary-600 shadow-sm px-4 py-2 bg-white dark:bg-secondary-800 text-base font-medium text-secondary-700 dark:text-secondary-200 hover:bg-secondary-50 dark:hover:bg-secondary-700 sm:mt-0 sm:ml-3 sm:w-auto sm:text-sm"
						>
							Cancel
						</button>
					</div>
				</div>
			</div>
		</div>
	);
};

export default DashboardSettingsModal;
