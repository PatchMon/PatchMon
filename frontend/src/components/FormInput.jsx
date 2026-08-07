/**
 * Reusable form input with OIDC-style styling.
 * Used across FirstTimeWizard, Profile, ProtocolUrlTab for consistent appearance.
 */
const FORM_INPUT_CLASS =
	"w-full px-3 py-2 bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-primary-500 placeholder-secondary-400";

const FormInput = ({
	id,
	label,
	type = "text",
	name,
	value,
	onChange,
	onBlur,
	placeholder,
	disabled = false,
	required = false,
	error,
	className = "",
	...rest
}) => {
	const inputClass = error
		? `${FORM_INPUT_CLASS} border-danger-500 dark:border-danger-400 focus:ring-danger-500 focus:border-danger-500 ${className}`
		: `${FORM_INPUT_CLASS} ${className}`;

	return (
		<div>
			{label && (
				<label
					htmlFor={id}
					className="block text-sm font-medium text-secondary-700 dark:text-white mb-1"
				>
					{label}
					{required && <span className="text-danger-500 ml-0.5">*</span>}
				</label>
			)}
			<input
				id={id}
				type={type}
				name={name}
				value={value}
				onChange={onChange}
				onBlur={onBlur}
				placeholder={placeholder}
				disabled={disabled}
				required={required}
				className={inputClass}
				{...rest}
			/>
			{error && (
				<p className="mt-1 text-sm text-danger-600 dark:text-danger-400">
					{error}
				</p>
			)}
		</div>
	);
};

export default FormInput;
export { FORM_INPUT_CLASS };
