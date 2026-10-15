/**
 * Authentication phases for the centralized auth state machine
 *
 * Flow: INITIALISING -> CHECKING_SETUP -> READY
 * A signed-in user an admin reset sits in ACCOUNT_SETUP_REQUIRED until they replace a temporary
 * password and set up 2FA again.
 */
export const AUTH_PHASES = {
	INITIALISING: "INITIALISING",
	CHECKING_SETUP: "CHECKING_SETUP",
	ACCOUNT_SETUP_REQUIRED: "ACCOUNT_SETUP_REQUIRED",
	READY: "READY",
};

/**
 * Helper functions for auth phase management
 */
export const isAuthPhase = {
	initialising: (phase) => phase === AUTH_PHASES.INITIALISING,
	checkingSetup: (phase) => phase === AUTH_PHASES.CHECKING_SETUP,
	accountSetupRequired: (phase) => phase === AUTH_PHASES.ACCOUNT_SETUP_REQUIRED,
	ready: (phase) => phase === AUTH_PHASES.READY,
};

/**
 * Check if authentication is fully initialised and ready
 * @param {string} phase - Current auth phase
 * @param {boolean} isAuthenticated - Whether user is authenticated
 * @returns {boolean} - True if auth is ready for other contexts to use
 */
export const isAuthReady = (phase, isAuthenticated) => {
	return isAuthPhase.ready(phase) && isAuthenticated;
};
