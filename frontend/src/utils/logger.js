/**
 * Development-Only Logging Utility
 *
 * Provides console logging functions that only output in development mode.
 * This prevents debug information from leaking in production builds.
 *
 * Usage:
 *   import { devLog, devError, devWarn, devDebug } from '../utils/logger';
 *
 *   devLog('User logged in:', user);
 *   devError('Failed to fetch data:', error);
 *   devWarn('Deprecated API used');
 *   devDebug('Detailed debug info:', data);
 */

// Check if we're in development mode
// Supports Vite's import.meta.env.DEV and fallback for other environments
const isDev = import.meta.env?.DEV ?? process.env.NODE_ENV === "development";

/**
 * Log a message in development mode only
 * @param {...any} args - Arguments to pass to console.log
 */
export const devLog = (...args) => {
	if (isDev) {
		console.log(...args);
	}
};

/**
 * Log an error in development mode only
 * @param {...any} args - Arguments to pass to console.error
 */
export const devError = (...args) => {
	if (isDev) {
		console.error(...args);
	}
};

/**
 * Log a warning in development mode only
 * @param {...any} args - Arguments to pass to console.warn
 */
export const devWarn = (...args) => {
	if (isDev) {
		console.warn(...args);
	}
};

/**
 * Log debug info in development mode only
 * @param {...any} args - Arguments to pass to console.debug
 */
export const devDebug = (...args) => {
	if (isDev) {
		console.debug(...args);
	}
};

/**
 * Log a grouped set of messages in development mode only
 * @param {string} label - Group label
 * @param {Function} fn - Function containing log statements
 */
export const devGroup = (label, fn) => {
	if (isDev) {
		console.group(label);
		fn();
		console.groupEnd();
	}
};

/**
 * Log a table in development mode only
 * @param {any} data - Data to display in table format
 */
export const devTable = (data) => {
	if (isDev) {
		console.table(data);
	}
};

// Export isDev for components that need to conditionally render debug UI
export { isDev };

export default {
	log: devLog,
	error: devError,
	warn: devWarn,
	debug: devDebug,
	group: devGroup,
	table: devTable,
	isDev,
};
