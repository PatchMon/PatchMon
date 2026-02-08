// Common utilities for automation jobs
const logger = require("../../../utils/logger");
const dns = require("node:dns").promises;

/**
 * Compare two semantic versions
 * @param {string} version1 - First version
 * @param {string} version2 - Second version
 * @returns {number} - 1 if version1 > version2, -1 if version1 < version2, 0 if equal
 */
function compareVersions(version1, version2) {
	const v1parts = version1.split(".").map(Number);
	const v2parts = version2.split(".").map(Number);

	const maxLength = Math.max(v1parts.length, v2parts.length);

	for (let i = 0; i < maxLength; i++) {
		const v1part = v1parts[i] || 0;
		const v2part = v2parts[i] || 0;

		if (v1part > v2part) return 1;
		if (v1part < v2part) return -1;
	}

	return 0;
}

/**
 * Check version from DNS TXT record
 * @param {string} domain - DNS domain to query (e.g., "server.vcheck.patchmon.net")
 * @returns {Promise<string>} - Latest version string (e.g., "1.4.0")
 */
async function checkVersionFromDNS(domain) {
	try {
		const records = await dns.resolveTxt(domain);
		if (!records || records.length === 0) {
			throw new Error(`No TXT records found for ${domain}`);
		}
		// TXT records are arrays of strings, get first record's first string
		const version = records[0][0].trim().replace(/^["']|["']$/g, "");
		// Validate version format (semantic versioning)
		if (!/^\d+\.\d+\.\d+/.test(version)) {
			throw new Error(`Invalid version format: ${version}`);
		}
		return version;
	} catch (error) {
		logger.error(`DNS lookup failed for ${domain}:`, error.message);
		throw error;
	}
}

module.exports = {
	compareVersions,
	checkVersionFromDNS,
};
