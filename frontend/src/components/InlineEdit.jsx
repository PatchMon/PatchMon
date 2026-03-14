import { Check, Edit2, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";

const InlineEdit = ({
	value,
	onSave,
	onCancel,
	placeholder = "Enter value...",
	maxLength = 100,
	className = "",
	disabled = false,
	validate = null,
	linkTo = null,
}) => {
	const [isEditing, setIsEditing] = useState(false);
	const [editValue, setEditValue] = useState(value);
	const [isLoading, setIsLoading] = useState(false);
	const [error, setError] = useState("");
	const inputRef = useRef(null);

	useEffect(() => {
		if (isEditing && inputRef.current) {
			inputRef.current.focus();
			inputRef.current.select();
		}
	}, [isEditing]);

	useEffect(() => {
		setEditValue(value);
	}, [value]);

	const handleEdit = () => {
		if (disabled) return;
		setIsEditing(true);
		setEditValue(value);
		setError("");
	};

	const handleCancel = () => {
		setIsEditing(false);
		setEditValue(value);
		setError("");
		if (onCancel) onCancel();
	};

	const handleSave = async () => {
		if (disabled || isLoading) return;

		// Validate if validator function provided
		if (validate) {
			const validationError = validate(editValue);
			if (validationError) {
				setError(validationError);
				return;
			}
		}

		// Check if value actually changed
		if (editValue.trim() === value.trim()) {
			setIsEditing(false);
			return;
		}

		setIsLoading(true);
		setError("");

		try {
			await onSave(editValue.trim());
			setIsEditing(false);
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

	if (isEditing) {
		return (
			<div className={`flex items-center gap-2 ${className}`}>
				<input
					ref={inputRef}
					type="text"
					value={editValue}
					onChange={(e) => setEditValue(e.target.value)}
					onKeyDown={handleKeyDown}
					placeholder={placeholder}
					maxLength={maxLength}
					disabled={isLoading}
					className={`flex-1 px-2 py-1 text-sm border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent ${
						error ? "border-red-500" : ""
					} ${isLoading ? "opacity-50" : ""}`}
				/>
				<button
					type="button"
					onClick={handleSave}
					disabled={isLoading || editValue.trim() === ""}
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
				{error && (
					<span className="text-xs text-red-600 dark:text-red-400">
						{error}
					</span>
				)}
			</div>
		);
	}

	const displayValue = linkTo ? (
		<Link
			to={linkTo}
			className="text-sm font-medium text-secondary-900 dark:text-white hover:text-primary-600 dark:hover:text-primary-400 cursor-pointer transition-colors"
			title="View details"
		>
			{value}
		</Link>
	) : (
		<span className="text-sm font-medium text-secondary-900 dark:text-white">
			{value}
		</span>
	);

	return (
		<div className={`flex items-center gap-2 group ${className}`}>
			{displayValue}
			{!disabled && (
				<button
					type="button"
					onClick={handleEdit}
					className="p-1 text-secondary-400 hover:text-secondary-600 dark:hover:text-secondary-300 hover:bg-secondary-100 dark:hover:bg-secondary-700 rounded transition-colors opacity-0 group-hover:opacity-100"
					title="Edit"
				>
					<Edit2 className="h-3 w-3" />
				</button>
			)}
		</div>
	);
};

export default InlineEdit;
