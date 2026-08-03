// Version comparison mirroring util.CompareVersions in the Go server
// (server-source-code/internal/util/version.go). Keep the two in step: the
// login screen and the server's update alert must agree on whether an update
// is available, or the UI contradicts the notification.
//
// Pre-releases rank below their release (2.0.3-rc.4 < 2.0.3) per semver §11.
// Edge images built from main carry a pre-release version, so this ordering is
// what lets an edge instance recognise the eventual release as an upgrade.

const stripPrefix = (v) =>
	String(v ?? "")
		.trim()
		.replace(/^v/i, "");

// "2.0.3-rc.4" -> { core: "2.0.3", pre: "rc.4" }. Build metadata ("+abc") is
// discarded: semver gives it no ordering weight.
const splitPreRelease = (version) => {
	const v = stripPrefix(version).split("+")[0];
	const dash = v.indexOf("-");
	if (dash < 0) return { core: v, pre: "" };
	return { core: v.slice(0, dash), pre: v.slice(dash + 1) };
};

const compareNumericParts = (a, b) => {
	const pa = a.split(".");
	const pb = b.split(".");
	const len = Math.max(pa.length, pb.length);
	for (let i = 0; i < len; i++) {
		// Number("") is 0 and Number("x") is NaN; both collapse to 0 so a
		// malformed part cannot outrank a real one.
		const na = Number(pa[i]) || 0;
		const nb = Number(pb[i]) || 0;
		if (na !== nb) return na > nb ? 1 : -1;
	}
	return 0;
};

const isNumeric = (s) => s !== "" && /^\d+$/.test(s);

// Semver §11.4: numeric identifiers compare numerically and rank below
// alphanumeric ones, and a longer identifier list wins when all shared fields
// are equal.
const comparePreRelease = (pre1, pre2) => {
	const a = pre1.split(".");
	const b = pre2.split(".");
	const len = Math.max(a.length, b.length);
	for (let i = 0; i < len; i++) {
		if (i >= a.length) return -1;
		if (i >= b.length) return 1;
		const na = isNumeric(a[i]);
		const nb = isNumeric(b[i]);
		if (na && nb) {
			const diff = Number(a[i]) - Number(b[i]);
			if (diff !== 0) return diff > 0 ? 1 : -1;
		} else if (na) {
			return -1;
		} else if (nb) {
			return 1;
		} else if (a[i] !== b[i]) {
			return a[i] > b[i] ? 1 : -1;
		}
	}
	return 0;
};

/**
 * Compare two version strings.
 * @returns {number} 1 if v1 > v2, -1 if v1 < v2, 0 if equal.
 */
export const compareVersions = (v1, v2) => {
	const a = splitPreRelease(v1);
	const b = splitPreRelease(v2);

	const core = compareNumericParts(a.core, b.core);
	if (core !== 0) return core;

	if (!a.pre && !b.pre) return 0;
	if (!a.pre) return 1;
	if (!b.pre) return -1;
	return comparePreRelease(a.pre, b.pre);
};

/** True when `latest` is strictly newer than `installed`. */
export const isUpdateAvailable = (installed, latest) => {
	if (!stripPrefix(installed) || !stripPrefix(latest)) return false;
	return compareVersions(latest, installed) > 0;
};
