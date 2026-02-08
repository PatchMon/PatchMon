import { Check, ChevronDown, Edit2, X } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

const InlineMultiGroupEdit = ({
	value = [], // Array of group IDs
	onSave,
	onCancel,
	options = [],
	className = "",
	disabled = false,
}) => {
	const [isEditing, setIsEditing] = useState(false);
	const [selectedValues, setSelectedValues] = useState(value);
	const [isLoading, setIsLoading] = useState(false);
	const [error, setError] = useState("");
	const [isOpen, setIsOpen] = useState(false);
	const [dropdownPosition, setDropdownPosition] = useState({
		top: 0,
		left: 0,
		width: 0,
	});
	const dropdownRef = useRef(null);
	const buttonRef = useRef(null);

	useEffect(() => {
		if (isEditing && dropdownRef.current) {
			dropdownRef.current.focus();
		}
	}, [isEditing]);

	useEffect(() => {
		setSelectedValues(value);
		// Force re-render when value changes
		if (!isEditing) {
			setIsOpen(false);
		}
	}, [value, isEditing]);

	// Calculate dropdown position
	const calculateDropdownPosition = useCallback(() => {
		if (buttonRef.current) {
			const rect = buttonRef.current.getBoundingClientRect();
			setDropdownPosition({
				top: rect.bottom + window.scrollY + 4,
				left: rect.left + window.scrollX,
				width: rect.width,
			});
		}
	}, []);

	// Close dropdown when clicking outside
	useEffect(() => {
		const handleClickOutside = (event) => {
			if (dropdownRef.current && !dropdownRef.current.contains(event.target)) {
				setIsOpen(false);
			}
		};

		if (isOpen) {
			calculateDropdownPosition();
			document.addEventListener("mousedown", handleClickOutside);
			window.addEventListener("resize", calculateDropdownPosition);
			window.addEventListener("scroll", calculateDropdownPosition);
			return () => {
				document.removeEventListener("mousedown", handleClickOutside);
				window.removeEventListener("resize", calculateDropdownPosition);
				window.removeEventListener("scroll", calculateDropdownPosition);
			};
		}
	}, [isOpen, calculateDropdownPosition]);

	const handleEdit = () => {
		if (disabled) return;
		setIsEditing(true);
		setSelectedValues(value);
		setError("");
		// Automatically open dropdown when editing starts
		setTimeout(() => {
			setIsOpen(true);
		}, 0);
	};

	const handleCancel = () => {
		setIsEditing(false);
		setSelectedValues(value);
		setError("");
		setIsOpen(false);
		if (onCancel) onCancel();
	};

	const handleSave = async () => {
		if (disabled || isLoading) return;

		// Check if values actually changed
		const sortedCurrent = [...value].sort();
		const sortedSelected = [...selectedValues].sort();
		if (JSON.stringify(sortedCurrent) === JSON.stringify(sortedSelected)) {
			setIsEditing(false);
			setIsOpen(false);
			return;
		}

		setIsLoading(true);
		setError("");

		try {
			await onSave(selectedValues);
			setIsEditing(false);
			setIsOpen(false);
		} catch (err) {
			setError(err.message || "Failed to save");
		} finally {
			setIsLoading(false);
		}
	};

	const handleKeyDown = (e) => {
		if (e.key === "Enter") {
			e.preventDefault();
			handleSave();
		} else if (e.key === "Escape") {
			e.preventDefault();
			handleCancel();
		}
	};

	const toggleGroup = (groupId) => {
		setSelectedValues((prev) => {
			if (prev.includes(groupId)) {
				return prev.filter((id) => id !== groupId);
			} else {
				return [...prev, groupId];
			}
		});
	};

	const displayGroups = useMemo(() => {
		if (!value || value.length === 0) {
			return [];
		}
		return value
			.map((groupId) => options.find((opt) => opt.id === groupId))
			.filter(Boolean);
	}, [value, options]);

	if (isEditing) {
		return (
			<div className={`relative ${className}`} ref={dropdownRef}>
				<div className="flex items-center gap-2">
					<div className="relative flex-1">
						<button
							ref={buttonRef}
							type="button"
							onClick={() => setIsOpen(!isOpen)}
							onKeyDown={handleKeyDown}
							disabled={isLoading}
							className={`w-full px-3 py-1 text-sm border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent flex items-center justify-between ${
								error ? "border-red-500" : ""
							} ${isLoading ? "opacity-50" : ""}`}
						>
							<span className="truncate">
								{selectedValues.length === 0
									? "Ungrouped"
									: selectedValues.length === 1
										? options.find((opt) => opt.id === selectedValues[0])
												?.name || "Unknown Group"
										: `${selectedValues.length} groups selected`}
							</span>
							<ChevronDown className="h-4 w-4 flex-shrink-0" />
						</button>

						{isOpen && (
							<div
								className="fixed z-50 bg-white dark:bg-secondary-800 border border-secondary-300 dark:border-secondary-600 rounded-md shadow-lg max-h-60 overflow-auto"
								style={{
									top: `${dropdownPosition.top}px`,
									left: `${dropdownPosition.left}px`,
									width: `${dropdownPosition.width}px`,
									minWidth: "200px",
								}}
							>
								<div className="py-1">
									{options.map((option) => (
										<label
											key={option.id}
											className="w-full px-3 py-2 text-left text-sm hover:bg-secondary-100 dark:hover:bg-secondary-700 flex items-center cursor-pointer"
										>
											<input
												type="checkbox"
												checked={selectedValues.includes(option.id)}
												onChange={() => toggleGroup(option.id)}
												className="mr-2 h-4 w-4 text-primary-600 focus:ring-primary-500 border-secondary-300 rounded"
											/>
											<span
												className="inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium text-white"
												style={{ backgroundColor: option.color }}
											>
												{option.name}
											</span>
										</label>
									))}
									{options.length === 0 && (
										<div className="px-3 py-2 text-sm text-secondary-500 dark:text-secondary-400">
											No groups available
										</div>
									)}
								</div>
							</div>
						)}
					</div>
					<button
						type="button"
						onClick={handleSave}
						disabled={isLoading}
						className="p-1 text-green-600 hover:text-green-700 hover:bg-green-50 dark:hover:bg-green-900/20 rounded transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
						title="Save"
					>
						<Check className="h-4 w-4" />
					</button>
					<button
						type="button"
						onClick={handleCancel}
						disabled={isLoading}
						className="p-1 text-red-600 hover:text-red-700 hover:bg-red-50 dark:hover:bg-red-900/20 rounded transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
						title="Cancel"
					>
						<X className="h-4 w-4" />
					</button>
				</div>
				{error && (
					<span className="text-xs text-red-600 dark:text-red-400 mt-1 block">
						{error}
					</span>
				)}
			</div>
		);
	}

	return (
		<div className={`flex items-center gap-1 group ${className}`}>
			{displayGroups.length === 0 ? (
				<span className="inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium bg-secondary-100 text-secondary-800">
					Ungrouped
				</span>
			) : (
				<div className="flex items-center gap-1 flex-wrap">
					{displayGroups.map((group) => (
						<span
							key={group.id}
							className="inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium text-white"
							style={{ backgroundColor: group.color }}
						>
							{group.name}
						</span>
					))}
				</div>
			)}
			{!disabled && (
				<button
					type="button"
					onClick={handleEdit}
					className="p-1 text-secondary-400 hover:text-secondary-600 dark:hover:text-secondary-300 hover:bg-secondary-100 dark:hover:bg-secondary-700 rounded transition-colors opacity-0 group-hover:opacity-100"
					title="Edit groups"
				>
					<Edit2 className="h-3 w-3" />
				</button>
			)}
		</div>
	);
};

export default InlineMultiGroupEdit;
