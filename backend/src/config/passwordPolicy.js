/**
 * Shared password policy (env-driven). Used by auth validation and by
 * public login-settings so the frontend can validate with the same rules.
 */

const PASSWORD_MIN_LENGTH = Math.max(
	1,
	parseInt(process.env.PASSWORD_MIN_LENGTH, 10) || 8,
);
const PASSWORD_REQUIRE_UPPERCASE =
	process.env.PASSWORD_REQUIRE_UPPERCASE !== "false";
const PASSWORD_REQUIRE_LOWERCASE =
	process.env.PASSWORD_REQUIRE_LOWERCASE !== "false";
const PASSWORD_REQUIRE_NUMBER = process.env.PASSWORD_REQUIRE_NUMBER !== "false";
const PASSWORD_REQUIRE_SPECIAL =
	process.env.PASSWORD_REQUIRE_SPECIAL !== "false";

/** Non-alphanumeric (any character that is not A-Z, a-z, 0-9) so all symbols count */
const SPECIAL_REGEX = /[^A-Za-z0-9]/;

/**
 * Public policy for frontend (no logic, just requirements)
 */
function get_password_policy() {
	return {
		min_length: PASSWORD_MIN_LENGTH,
		require_uppercase: PASSWORD_REQUIRE_UPPERCASE,
		require_lowercase: PASSWORD_REQUIRE_LOWERCASE,
		require_number: PASSWORD_REQUIRE_NUMBER,
		require_special: PASSWORD_REQUIRE_SPECIAL,
	};
}

/**
 * Validate password complexity
 * @param {string} password - The password to validate
 * @returns {{valid: boolean, errors: string[]}}
 */
function validate_password_complexity(password) {
	const errors = [];

	if (!password || password.length < PASSWORD_MIN_LENGTH) {
		errors.push(`Password must be at least ${PASSWORD_MIN_LENGTH} characters`);
	}

	if (PASSWORD_REQUIRE_UPPERCASE && !/[A-Z]/.test(password)) {
		errors.push("Password must contain at least one uppercase letter");
	}

	if (PASSWORD_REQUIRE_LOWERCASE && !/[a-z]/.test(password)) {
		errors.push("Password must contain at least one lowercase letter");
	}

	if (PASSWORD_REQUIRE_NUMBER && !/[0-9]/.test(password)) {
		errors.push("Password must contain at least one number");
	}

	if (PASSWORD_REQUIRE_SPECIAL && !SPECIAL_REGEX.test(password)) {
		errors.push("Password must contain at least one special character");
	}

	return { valid: errors.length === 0, errors };
}

/**
 * Express-validator custom validator for password complexity
 */
function password_complexity_validator(value) {
	const result = validate_password_complexity(value);
	if (!result.valid) {
		throw new Error(result.errors.join(". "));
	}
	return true;
}

module.exports = {
	get_password_policy,
	validate_password_complexity,
	password_complexity_validator,
	PASSWORD_MIN_LENGTH,
};
