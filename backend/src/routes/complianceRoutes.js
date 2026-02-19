const express = require("express");
const logger = require("../utils/logger");
const router = express.Router();
const rateLimit = require("express-rate-limit");
const { getPrismaClient } = require("../config/prisma");
const { authenticateToken } = require("../middleware/auth");
const { v4: uuidv4, validate: uuidValidate } = require("uuid");
const { verifyApiKey } = require("../utils/apiKeyUtils");
const agentWs = require("../services/agentWs");
const { queueManager, QUEUE_NAMES } = require("../services/automation");
const { redis } = require("../services/automation/shared/redis");

const prisma = getPrismaClient();

const COMPLIANCE_INSTALL_JOB_PREFIX = "compliance_install_job:";
const COMPLIANCE_INSTALL_CANCEL_PREFIX = "compliance_install_cancel:";
const COMPLIANCE_INSTALL_JOB_TTL = 3600;
const COMPLIANCE_SCAN_JOB_ID_PREFIX = "compliance-scan-";
const COMPLIANCE_SCAN_QUEUE_RETRY_DELAY_MS = 60 * 1000; // 1 min when agent offline

// Short-lived cache for compliance dashboard (reduces DB load under repeated requests)
const DASHBOARD_CACHE_TTL_MS = 45 * 1000; // 45 seconds
let dashboard_cache = { data: null, expires: 0 };

// Rate limiter for scan submissions (per agent)
const scanSubmitLimiter = rateLimit({
	windowMs: 60 * 1000, // 1 minute
	max: 10, // 10 scans per minute per agent
	keyGenerator: (req) => req.headers["x-api-id"] || req.ip,
	message: { error: "Too many scan submissions, please try again later" },
	standardHeaders: true,
	legacyHeaders: false,
	validate: { trustProxy: false },
});

// ==========================================
// Input Validation Helpers
// ==========================================

const VALID_RESULT_STATUSES = [
	"pass",
	"fail",
	"warn",
	"skip",
	"notapplicable",
	"skipped", // frontend uses "skipped" for skip+notapplicable
	"error",
];
const VALID_SEVERITIES = ["low", "medium", "high", "critical", "unknown"];
const VALID_PROFILE_TYPES = ["openscap", "docker-bench", "oscap-docker", "all"];

function isValidUUID(id) {
	return id && uuidValidate(id);
}

function sanitizeInt(value, defaultVal, min = 1, max = 1000) {
	const parsed = parseInt(value, 10);
	if (Number.isNaN(parsed)) return defaultVal;
	return Math.min(Math.max(parsed, min), max);
}

// Normalize agent result status to canonical DB values (fail, pass, warn, skip, notapplicable, error)
function normalizeResultStatus(status) {
	if (!status || typeof status !== "string") return status;
	const s = status.toLowerCase().trim();
	const map = {
		fail: "fail",
		failed: "fail",
		failure: "fail",
		pass: "pass",
		passed: "pass",
		warn: "warn",
		warning: "warn",
		warned: "warn",
		skip: "skip",
		skipped: "skip",
		notapplicable: "notapplicable",
		not_applicable: "notapplicable",
		na: "notapplicable",
		error: "error",
	};
	return map[s] || status;
}

// Status values that match a given filter (for querying existing data with possible variants)
function statusFilterToDbValues(statusFilter) {
	switch (statusFilter) {
		case "fail":
			return ["fail", "failed", "failure"];
		case "pass":
			return ["pass", "passed"];
		case "warn":
			return ["warn", "warning", "warned"];
		case "skipped":
			return ["skip", "notapplicable", "skipped"];
		default:
			return [statusFilter];
	}
}

// Order for host compliance rule results: Failed first, then Warning, then Passed, then N/A/skip/error
function status_rank_for_sort(status) {
	const s = (status || "").toLowerCase();
	if (["fail", "failed", "failure"].includes(s)) return 1;
	if (["warn", "warning", "warned"].includes(s)) return 2;
	if (["pass", "passed"].includes(s)) return 3;
	return 4; // skip, notapplicable, error, etc.
}

/**
 * Get latest completed scan per (host_id, profile_id), or per host_id when profileId is set.
 * Uses PostgreSQL DISTINCT ON in a single query to avoid loading all scans into memory.
 */
async function getLatestCompletedScans(prismaClient, options = {}) {
	const { profile_id: profileId = null } = options;

	// Build parameterized raw query: one row per (host_id, profile_id) or per host_id when profileId set
	const distinctColumns = profileId ? "host_id" : "host_id, profile_id";
	const orderByColumns = profileId
		? "host_id, completed_at DESC"
		: "host_id, profile_id, completed_at DESC";

	const rows = await prismaClient.$queryRawUnsafe(
		`
		SELECT cs.id, cs.host_id, cs.profile_id, cs.completed_at, cs.passed, cs.failed,
			cs.warnings, cs.skipped, cs.not_applicable, cs.score, cs.total_rules,
			cp.name AS profile_name, cp.type AS profile_type
		FROM (
			SELECT DISTINCT ON (${distinctColumns})
				id, host_id, profile_id, completed_at, passed, failed, warnings,
				skipped, not_applicable, score, total_rules
			FROM compliance_scans
			WHERE status = 'completed'
			${profileId ? "AND profile_id = $1" : ""}
			ORDER BY ${orderByColumns}
		) cs
		LEFT JOIN compliance_profiles cp ON cp.id = cs.profile_id
		`,
		...(profileId ? [profileId] : []),
	);

	// Map to shape expected by callers (compliance_profiles: { name, type })
	return rows.map((r) => ({
		id: r.id,
		host_id: r.host_id,
		profile_id: r.profile_id,
		completed_at: r.completed_at,
		passed: r.passed,
		failed: r.failed,
		warnings: r.warnings,
		skipped: r.skipped,
		not_applicable: r.not_applicable,
		score: r.score,
		total_rules: r.total_rules,
		compliance_profiles: {
			name: r.profile_name,
			type: r.profile_type,
		},
	}));
}

// ==========================================
// Public endpoints (API key auth for agents)
// ==========================================

/**
 * POST /api/v1/compliance/scans
 * Submit scan results from agent
 * Auth: X-API-ID and X-API-KEY headers
 * Rate limited: 10 submissions per minute per agent
 */
router.post("/scans", scanSubmitLimiter, async (req, res) => {
	try {
		const apiId = req.headers["x-api-id"];
		const apiKey = req.headers["x-api-key"];

		if (!apiId || !apiKey) {
			return res.status(401).json({ error: "API credentials required" });
		}

		// Validate host credentials and load scanner toggles for filtering submitted scans
		const host = await prisma.hosts.findFirst({
			where: { api_id: apiId },
			select: {
				id: true,
				api_id: true,
				api_key: true,
				hostname: true,
				friendly_name: true,
				compliance_openscap_enabled: true,
				compliance_docker_bench_enabled: true,
			},
		});

		if (!host) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		const isValidKey = await verifyApiKey(apiKey, host.api_key);
		if (!isValidKey) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		// Handle both nested payload (from agent) and flat payload (legacy)
		// Agent sends: { scans: [...], os_info: {...}, hostname: "...", ... }
		// Legacy/flat: { profile_name: "...", results: [...], ... }
		let scansToProcess = [];

		if (req.body.scans && Array.isArray(req.body.scans)) {
			// New nested format from agent
			scansToProcess = req.body.scans;
			logger.info(
				`[Compliance] Received ${scansToProcess.length} scans from agent payload`,
			);
		} else if (req.body.profile_name) {
			// Legacy flat format - wrap in array
			scansToProcess = [
				{
					profile_name: req.body.profile_name,
					profile_type: req.body.profile_type,
					results: req.body.results,
					started_at: req.body.started_at,
					completed_at: req.body.completed_at,
					status: req.body.status,
					score: req.body.score,
					total_rules: req.body.total_rules,
					passed: req.body.passed,
					failed: req.body.failed,
					warnings: req.body.warnings,
					skipped: req.body.skipped,
					not_applicable: req.body.not_applicable,
					error: req.body.error,
				},
			];
		} else {
			return res.status(400).json({
				error: "Invalid payload: expected 'scans' array or 'profile_name'",
			});
		}

		const processedScans = [];

		for (const scanData of scansToProcess) {
			const {
				profile_name,
				profile_type,
				results,
				started_at,
				completed_at,
				status: scanStatus,
				error: scanError,
			} = scanData;

			if (!profile_name) {
				logger.warn("[Compliance] Skipping scan with no profile_name");
				continue;
			}

			// Find or create profile
			let profile = await prisma.compliance_profiles.findFirst({
				where: { name: profile_name },
			});

			if (!profile) {
				profile = await prisma.compliance_profiles.create({
					data: {
						id: uuidv4(),
						name: profile_name,
						type: profile_type || "openscap",
					},
				});
			}

			// Only persist scans for scanner types that are enabled for this host.
			// Avoids storing OpenSCAP (or Docker Bench) results when the user has that scanner disabled.
			const profileType = profile.type || profile_type || "openscap";
			const openscapEnabled = host.compliance_openscap_enabled ?? true;
			const dockerBenchEnabled = host.compliance_docker_bench_enabled ?? false;
			if (
				(profileType === "openscap" && !openscapEnabled) ||
				(profileType === "docker-bench" && !dockerBenchEnabled)
			) {
				logger.info(
					`[Compliance] Skipping scan result for profile_type=${profileType} (disabled for host ${host.id})`,
				);
				continue;
			}

			// Use stats from agent if provided, otherwise calculate
			const stats = {
				total_rules: scanData.total_rules ?? results?.length ?? 0,
				passed:
					scanData.passed ??
					results?.filter((r) => r.status === "pass").length ??
					0,
				failed:
					scanData.failed ??
					results?.filter((r) => r.status === "fail").length ??
					0,
				warnings:
					scanData.warnings ??
					results?.filter((r) => r.status === "warn").length ??
					0,
				skipped:
					scanData.skipped ??
					results?.filter((r) => r.status === "skip").length ??
					0,
				not_applicable:
					scanData.not_applicable ??
					results?.filter((r) => r.status === "notapplicable").length ??
					0,
			};

			// Use score from agent if provided, otherwise calculate
			let score = scanData.score;
			if (score === undefined || score === null) {
				const applicableRules =
					stats.total_rules - stats.not_applicable - stats.skipped;
				score =
					applicableRules > 0
						? ((stats.passed / applicableRules) * 100).toFixed(2)
						: null;
			}

			// Delete any "running" placeholder scans for this host
			// We delete all running scans for this host regardless of profile, because:
			// 1. Bulk trigger creates running scans with first profile of each type
			// 2. Actual scan results may use a different profile name/id
			// 3. When real results arrive, the "running" placeholder is no longer needed
			await prisma.compliance_scans.deleteMany({
				where: {
					host_id: host.id,
					status: "running",
				},
			});

			// Create scan record
			const scan = await prisma.compliance_scans.create({
				data: {
					id: uuidv4(),
					host_id: host.id,
					profile_id: profile.id,
					started_at: started_at ? new Date(started_at) : new Date(),
					completed_at: completed_at ? new Date(completed_at) : new Date(),
					status: scanStatus === "failed" ? "failed" : "completed",
					total_rules: stats.total_rules,
					passed: stats.passed,
					failed: stats.failed,
					warnings: stats.warnings,
					skipped: stats.skipped,
					not_applicable: stats.not_applicable,
					score: score ? parseFloat(score) : null,
					error_message: scanError || null,
					raw_output: results ? JSON.stringify(results) : null,
				},
			});

			// Create rule and result records
			let results_stored = 0;
			if (!results || !Array.isArray(results)) {
				const hasResults = results != null;
				logger.warn(
					`[Compliance] Scan saved without per-rule results (profile=${profile_name}, host=${host.id}). ` +
						`Payload has 'results': ${hasResults}, isArray: ${Array.isArray(results)}. ` +
						`Agent must send a 'results' array on each scan object.`,
				);
			} else if (results && Array.isArray(results)) {
				// Debug: Count status values received from agent
				const receivedStatusCounts = {};
				for (const r of results) {
					receivedStatusCounts[r.status] =
						(receivedStatusCounts[r.status] || 0) + 1;
				}

				// Deduplicate results by rule_ref, prioritizing important statuses
				// Priority: fail > warn > pass > skip > notapplicable > error
				const statusPriority = {
					fail: 6,
					warn: 5,
					pass: 4,
					skip: 3,
					notapplicable: 2,
					error: 1,
				};
				const deduplicatedResults = new Map();

				for (const result of results) {
					const ruleRef = result.rule_ref || result.rule_id || result.id;
					if (!ruleRef) continue;

					const existingResult = deduplicatedResults.get(ruleRef);
					if (!existingResult) {
						deduplicatedResults.set(ruleRef, result);
					} else {
						// Keep the result with higher priority status
						const existingPriority = statusPriority[existingResult.status] || 0;
						const newPriority = statusPriority[result.status] || 0;
						if (newPriority > existingPriority) {
							deduplicatedResults.set(ruleRef, result);
						}
					}
				}

				const uniqueResults = Array.from(deduplicatedResults.values());

				if (uniqueResults.length === 0 && results.length > 0) {
					const sample = results[0];
					const keys =
						sample && typeof sample === "object" ? Object.keys(sample) : [];
					logger.warn(
						`[Compliance] All ${results.length} result(s) skipped: each item must have rule_ref, rule_id, or id. ` +
							`Sample keys: ${keys.join(", ") || "none"}.`,
					);
				}

				// Batch fetch all existing rules for this profile
				const ruleRefs = uniqueResults
					.map((r) => r.rule_ref || r.rule_id || r.id)
					.filter(Boolean);

				const existingRules = await prisma.compliance_rules.findMany({
					where: {
						profile_id: profile.id,
						rule_ref: { in: ruleRefs },
					},
				});

				const existingRulesMap = new Map(
					existingRules.map((r) => [r.rule_ref, r]),
				);

				// Separate rules into create and update batches
				const rulesToCreate = [];
				const rulesToUpdate = [];
				const ruleMap = new Map(); // Maps rule_ref to rule (existing or new)

				for (const result of uniqueResults) {
					const ruleRef = result.rule_ref || result.rule_id || result.id;
					if (!ruleRef) continue;

					const existingRule = existingRulesMap.get(ruleRef);

					if (!existingRule) {
						// Create new rule (compliance_rules has no created_at/updated_at)
						const newRule = {
							id: uuidv4(),
							profile_id: profile.id,
							rule_ref: ruleRef,
							title: result.title || ruleRef || "Unknown",
							description: result.description || null,
							severity: result.severity || null,
							section: result.section || null,
							remediation: result.remediation || null,
						};
						rulesToCreate.push(newRule);
						ruleMap.set(ruleRef, newRule);
					} else {
						// Check if we need to update existing rule
						const updateData = {};

						// Update title if agent provides one and current is missing/generic
						if (
							result.title &&
							result.title !== ruleRef &&
							(!existingRule.title ||
								existingRule.title === ruleRef ||
								existingRule.title === "Unknown" ||
								existingRule.title.toLowerCase().replace(/[_\s]+/g, " ") ===
									ruleRef
										.replace(/xccdf_org\.ssgproject\.content_rule_/i, "")
										.replace(/_/g, " "))
						) {
							updateData.title = result.title;
						}

						// Update description if agent provides one and current is missing
						if (result.description && !existingRule.description) {
							updateData.description = result.description;
						}

						// Update severity if agent provides one and current is missing
						if (result.severity && !existingRule.severity) {
							updateData.severity = result.severity;
						}

						// Update section if agent provides one and current is missing
						if (result.section && !existingRule.section) {
							updateData.section = result.section;
						}

						// Update remediation if agent provides one and current is missing
						if (result.remediation && !existingRule.remediation) {
							updateData.remediation = result.remediation;
						}

						if (Object.keys(updateData).length > 0) {
							rulesToUpdate.push({
								id: existingRule.id,
								...updateData,
							});
						}

						ruleMap.set(ruleRef, existingRule);
					}
				}

				// Batch create new rules
				if (rulesToCreate.length > 0) {
					await prisma.compliance_rules.createMany({
						data: rulesToCreate,
						skipDuplicates: true,
					});
				}

				// Batch update existing rules
				for (const update of rulesToUpdate) {
					const { id, ...updateData } = update;
					await prisma.compliance_rules.update({
						where: { id },
						data: updateData,
					});
				}

				// Batch create compliance results
				// Delete existing results for this scan first to avoid conflicts
				await prisma.compliance_results.deleteMany({
					where: { scan_id: scan.id },
				});

				const resultsToCreate = uniqueResults
					.map((result) => {
						const ruleRef = result.rule_ref || result.rule_id || result.id;
						if (!ruleRef) return null;

						const rule = ruleMap.get(ruleRef);
						if (!rule) return null;

						return {
							id: uuidv4(),
							scan_id: scan.id,
							rule_id: rule.id,
							status: normalizeResultStatus(result.status) || result.status,
							finding: result.finding || result.message || null,
							actual: result.actual || null,
							expected: result.expected || null,
							remediation: result.remediation || null,
						};
					})
					.filter(Boolean);

				if (resultsToCreate.length > 0) {
					await prisma.compliance_results.createMany({
						data: resultsToCreate,
						skipDuplicates: true,
					});
					results_stored = resultsToCreate.length;
				} else if (results.length > 0) {
					logger.warn(
						`[Compliance] No compliance_results created for scan ${scan.id} (profile=${profile_name}): ${results.length} results received but none could be mapped to rules.`,
					);
				}
			}
			logger.info(
				`[Compliance] Scan saved for host ${host.friendly_name || host.hostname} (${profile_name}): ${stats.passed}/${stats.total_rules} passed (${score}%), results_stored=${results_stored}`,
			);
			processedScans.push({
				scan_id: scan.id,
				profile_name,
				score: scan.score,
				stats,
				results_stored,
			});
		}

		res.json({
			message: "Scan results saved successfully",
			scans_received: processedScans.length,
			scans: processedScans,
		});
	} catch (error) {
		logger.error("[Compliance] Error saving scan results:", error);
		res.status(500).json({ error: "Failed to save scan results" });
	}
});

// ==========================================
// Protected endpoints (JWT auth for dashboard)
// ==========================================

// Apply JWT auth to all routes below
router.use(authenticateToken);

/**
 * GET /api/v1/compliance/profiles
 * List all compliance profiles
 */
router.get("/profiles", async (_req, res) => {
	try {
		const profiles = await prisma.compliance_profiles.findMany({
			orderBy: { name: "asc" },
			include: {
				_count: {
					select: { compliance_rules: true, compliance_scans: true },
				},
			},
		});

		res.json(profiles);
	} catch (error) {
		logger.error("[Compliance] Error fetching profiles:", error);
		res.status(500).json({ error: "Failed to fetch profiles" });
	}
});

/**
 * GET /api/v1/compliance/dashboard
 * Aggregated compliance statistics
 */
router.get("/dashboard", async (_req, res) => {
	try {
		const now = Date.now();
		if (dashboard_cache.data && dashboard_cache.expires > now) {
			return res.json(dashboard_cache.data);
		}

		// Run initial queries in parallel (include profile name for "last activity" title)
		const [latestScansRows, profileTypeQuery, unscanned, allHostsRows] =
			await Promise.all([
				getLatestCompletedScans(prisma),
				prisma.compliance_profiles.findMany({
					select: { id: true, type: true },
				}),
				prisma.hosts.count({
					where: {
						compliance_scans: { none: {} },
					},
				}),
				prisma.hosts.findMany({
					select: {
						id: true,
						hostname: true,
						friendly_name: true,
						compliance_enabled: true,
						compliance_on_demand_only: true,
						docker_enabled: true,
					},
				}),
			]);

		const latestScans = latestScansRows.map((s) => ({
			...s,
			profile_name: s.compliance_profiles?.name ?? null,
		}));

		// Build one row per host with their single most recent scan (for dashboard hosts table)
		const latest_per_host = new Map();
		for (const s of latestScans) {
			const existing = latest_per_host.get(s.host_id);
			if (
				!existing ||
				new Date(s.completed_at) > new Date(existing.completed_at)
			) {
				latest_per_host.set(s.host_id, s);
			}
		}
		const hosts_with_latest_scan = allHostsRows
			.map((h) => {
				const scan = latest_per_host.get(h.id);
				return {
					host_id: h.id,
					hostname: h.hostname,
					friendly_name: h.friendly_name,
					last_scan_date: scan?.completed_at ?? null,
					last_activity_title: scan?.profile_name ?? null,
					passed: scan != null ? Number(scan.passed) || 0 : null,
					failed: scan != null ? Number(scan.failed) || 0 : null,
					skipped:
						scan != null
							? (Number(scan.skipped) || 0) + (Number(scan.not_applicable) || 0)
							: null,
					score: scan != null ? Number(scan.score) : null,
					scanner_status:
						scan != null
							? "Scanned"
							: h.compliance_enabled
								? "Enabled"
								: "Never scanned",
					compliance_mode: h.compliance_enabled
						? h.compliance_on_demand_only
							? "on-demand"
							: "enabled"
						: "disabled",
					compliance_enabled: h.compliance_enabled,
					docker_enabled: h.docker_enabled,
				};
			})
			.sort((a, b) => {
				if (!a.last_scan_date) return 1;
				if (!b.last_scan_date) return -1;
				return new Date(b.last_scan_date) - new Date(a.last_scan_date);
			});

		// Calculate averages - use unique hosts, not scan count
		const uniqueHostIds = [...new Set(latestScans.map((s) => s.host_id))];
		const totalHosts = uniqueHostIds.length;
		const avgScore =
			latestScans.length > 0
				? latestScans.reduce((sum, s) => sum + (Number(s.score) || 0), 0) /
					latestScans.length
				: 0;

		// Get SCAN counts by compliance level (these are scan-level, not host-level)
		const scans_compliant = latestScans.filter(
			(s) => Number(s.score) >= 80,
		).length;
		const scans_warning = latestScans.filter(
			(s) => Number(s.score) >= 60 && Number(s.score) < 80,
		).length;
		const scans_critical = latestScans.filter(
			(s) => Number(s.score) < 60,
		).length;
		// Profile types map (from parallel query above)
		const profileTypes = {};
		for (const p of profileTypeQuery) {
			profileTypes[p.id] = p.type;
		}

		// Get HOST-LEVEL compliance status (using worst score per host)
		// A host is compliant only if ALL its scans are >=80%
		// A host is critical if ANY of its scans are <60%
		// A host is warning if its worst score is 60-80%

		// Track worst score and which scan type caused it per host
		const hostWorstScores = new Map(); // host_id -> { score, scanType }
		for (const scan of latestScans) {
			const scanScore = Number(scan.score) || 0;
			const scanType = profileTypes[scan.profile_id] || "unknown";
			const current = hostWorstScores.get(scan.host_id);
			if (!current || scanScore < current.score) {
				hostWorstScores.set(scan.host_id, { score: scanScore, scanType });
			}
		}

		// Count hosts by status
		const hosts_compliant = [...hostWorstScores.values()].filter(
			(h) => h.score >= 80,
		).length;
		const hosts_warning = [...hostWorstScores.values()].filter(
			(h) => h.score >= 60 && h.score < 80,
		).length;
		const hosts_critical = [...hostWorstScores.values()].filter(
			(h) => h.score < 60,
		).length;

		// Count hosts by status AND scan type that caused it
		const hostStatusByScanType = {
			compliant: { openscap: 0, "docker-bench": 0 },
			warning: { openscap: 0, "docker-bench": 0 },
			critical: { openscap: 0, "docker-bench": 0 },
		};
		for (const h of hostWorstScores.values()) {
			let status = "compliant";
			if (h.score < 60) status = "critical";
			else if (h.score < 80) status = "warning";

			if (hostStatusByScanType[status][h.scanType] !== undefined) {
				hostStatusByScanType[status][h.scanType]++;
			}
		}

		// Count total hosts with compliance enabled
		const hosts_with_compliance_enabled = allHostsRows.filter(
			(h) => h.compliance_enabled === true,
		).length;

		// For backwards compatibility, keep the old field names as scan counts
		const compliant = scans_compliant;
		const warning = scans_warning;
		const critical = scans_critical;

		// Fetch recent scans; derive worst hosts, profile distribution and type stats from latestScans
		const recentScans = await prisma.compliance_scans.findMany({
			take: 10,
			orderBy: { completed_at: "desc" },
			include: {
				hosts: {
					select: { id: true, hostname: true, friendly_name: true },
				},
				compliance_profiles: { select: { name: true, type: true } },
			},
		});

		const hostById = new Map(allHostsRows.map((h) => [h.id, h]));
		const worstHosts = latestScans
			.map((s) => {
				const host = hostById.get(s.host_id);
				return {
					id: s.id,
					host_id: s.host_id,
					score: s.score,
					completed_at: s.completed_at,
					host: host
						? {
								id: host.id,
								hostname: host.hostname,
								friendly_name: host.friendly_name,
							}
						: { id: s.host_id, hostname: null, friendly_name: null },
					profile: { name: s.profile_name ?? s.compliance_profiles?.name },
					compliance_profiles: {
						type: s.compliance_profiles?.type,
						name: s.profile_name ?? s.compliance_profiles?.name,
					},
				};
			})
			.sort((a, b) => (Number(a.score) || 0) - (Number(b.score) || 0))
			.slice(0, 5);

		const profileDistribution = Object.values(
			latestScans.reduce((acc, s) => {
				const id = s.profile_id;
				if (!acc[id])
					acc[id] = {
						profile_name: s.compliance_profiles?.name ?? s.profile_name,
						profile_type: s.compliance_profiles?.type,
						host_count: 0,
					};
				acc[id].host_count += 1;
				return acc;
			}, {}),
		)
			.map((p) => ({ ...p, host_count: Number(p.host_count) }))
			.sort((a, b) => (b.host_count || 0) - (a.host_count || 0));

		const profileTypeStats = Object.values(
			latestScans.reduce((acc, s) => {
				const t = s.compliance_profiles?.type ?? "unknown";
				if (!acc[t])
					acc[t] = {
						profile_type: t,
						hosts_scanned: 0,
						total_score: 0,
						total_passed: 0,
						total_failed: 0,
						total_warnings: 0,
						total_rules: 0,
					};
				acc[t].hosts_scanned += 1;
				acc[t].total_score += Number(s.score) || 0;
				acc[t].total_passed += Number(s.passed) || 0;
				acc[t].total_failed += Number(s.failed) || 0;
				acc[t].total_warnings += Number(s.warnings) || 0;
				acc[t].total_rules += Number(s.total_rules) || 0;
				return acc;
			}, {}),
		).map((p) => ({
			profile_type: p.profile_type,
			hosts_scanned: p.hosts_scanned,
			average_score: p.hosts_scanned ? p.total_score / p.hosts_scanned : null,
			total_passed: p.total_passed,
			total_failed: p.total_failed,
			total_warnings: p.total_warnings,
			total_rules: p.total_rules,
		}));

		// Transform recent_scans to match frontend expectations
		// Keep compliance_profiles for filtering, also add host/profile aliases for display
		const transformedRecentScans = recentScans.map((scan) => ({
			...scan,
			host: scan.hosts,
			profile: scan.compliance_profiles,
			// Keep compliance_profiles for filtering (don't set to undefined)
		}));

		// Get aggregate rule statistics from latest scans
		const totalPassedRules = latestScans.reduce(
			(sum, s) => sum + (Number(s.passed) || 0),
			0,
		);
		const totalFailedRules = latestScans.reduce(
			(sum, s) => sum + (Number(s.failed) || 0),
			0,
		);
		const totalRules = latestScans.reduce(
			(sum, s) => sum + (Number(s.total_rules) || 0),
			0,
		);

		// Parallel fetch: rules and severity (all depend on latestScanIds)
		const latestScanIds = latestScans.map((s) => s.id);
		let topFailingRules = [];
		let topWarningRules = [];
		let severityBreakdown = [];
		let severityByProfileType = [];
		let dockerBenchBySection = [];

		if (latestScanIds.length > 0) {
			const [failResults, warnResults, failWarnResults, dockerWarnResults] =
				await Promise.all([
					prisma.compliance_results.findMany({
						where: {
							scan_id: { in: latestScanIds },
							status: "fail",
						},
						select: {
							rule_id: true,
							compliance_rules: { select: { title: true, severity: true } },
							compliance_scans: {
								select: { compliance_profiles: { select: { type: true } } },
							},
						},
					}),
					prisma.compliance_results.findMany({
						where: {
							scan_id: { in: latestScanIds },
							status: "warn",
						},
						select: {
							rule_id: true,
							compliance_rules: {
								select: { title: true, severity: true, section: true },
							},
							compliance_scans: {
								select: { compliance_profiles: { select: { type: true } } },
							},
						},
					}),
					prisma.compliance_results.findMany({
						where: {
							scan_id: { in: latestScanIds },
							status: { in: ["fail", "warn"] },
						},
						select: {
							compliance_rules: { select: { severity: true } },
							compliance_scans: {
								select: { compliance_profiles: { select: { type: true } } },
							},
						},
					}),
					prisma.compliance_results.findMany({
						where: {
							scan_id: { in: latestScanIds },
							status: "warn",
						},
						select: {
							compliance_rules: { select: { section: true } },
							compliance_scans: {
								select: { compliance_profiles: { select: { type: true } } },
							},
						},
					}),
				]);

			const severityOrder = (a, b) => {
				const order = { critical: 1, high: 2, medium: 3, low: 4 };
				return (order[a] ?? 5) - (order[b] ?? 5);
			};

			const failByKey = failResults.reduce((acc, r) => {
				const key = `${r.rule_id}:${r.compliance_scans?.compliance_profiles?.type ?? ""}`;
				if (!acc[key])
					acc[key] = {
						rule_id: r.rule_id,
						title: r.compliance_rules?.title,
						severity: r.compliance_rules?.severity,
						profile_type: r.compliance_scans?.compliance_profiles?.type,
						fail_count: 0,
					};
				acc[key].fail_count += 1;
				return acc;
			}, {});
			topFailingRules = Object.values(failByKey)
				.sort((a, b) => (b.fail_count || 0) - (a.fail_count || 0))
				.slice(0, 10);

			const warnByKey = warnResults.reduce((acc, r) => {
				const key = `${r.rule_id}:${r.compliance_scans?.compliance_profiles?.type ?? ""}`;
				if (!acc[key])
					acc[key] = {
						rule_id: r.rule_id,
						title: r.compliance_rules?.title,
						severity: r.compliance_rules?.severity,
						profile_type: r.compliance_scans?.compliance_profiles?.type,
						warn_count: 0,
					};
				acc[key].warn_count += 1;
				return acc;
			}, {});
			topWarningRules = Object.values(warnByKey)
				.sort((a, b) => (b.warn_count || 0) - (a.warn_count || 0))
				.slice(0, 10);

			const severityCounts = failWarnResults.reduce((acc, r) => {
				const s = r.compliance_rules?.severity ?? "unknown";
				acc[s] = (acc[s] || 0) + 1;
				return acc;
			}, {});
			severityBreakdown = Object.entries(severityCounts)
				.map(([severity, count]) => ({ severity, count }))
				.sort((a, b) => severityOrder(a.severity, b.severity));

			const severityByTypeCounts = failWarnResults.reduce((acc, r) => {
				const sev = r.compliance_rules?.severity ?? "unknown";
				const typ = r.compliance_scans?.compliance_profiles?.type ?? "unknown";
				const key = `${sev}:${typ}`;
				if (!acc[key])
					acc[key] = { severity: sev, profile_type: typ, count: 0 };
				acc[key].count += 1;
				return acc;
			}, {});
			severityByProfileType = Object.values(severityByTypeCounts).sort((a, b) =>
				severityOrder(a.severity, b.severity),
			);

			const dockerWarnBySection = dockerWarnResults
				.filter(
					(r) =>
						r.compliance_scans?.compliance_profiles?.type === "docker-bench",
				)
				.reduce((acc, r) => {
					const sec = r.compliance_rules?.section ?? "Unknown";
					acc[sec] = (acc[sec] || 0) + 1;
					return acc;
				}, {});
			dockerBenchBySection = Object.entries(dockerWarnBySection)
				.map(([section, count]) => ({ section, count }))
				.sort((a, b) => (a.section || "").localeCompare(b.section || ""));
		}

		// Calculate scan age distribution (how fresh is the compliance data)
		// Track by profile type (OpenSCAP vs Docker Bench)
		const scanNow = new Date();
		const oneDayAgo = new Date(scanNow - 24 * 60 * 60 * 1000);
		const oneWeekAgo = new Date(scanNow - 7 * 24 * 60 * 60 * 1000);
		const oneMonthAgo = new Date(scanNow - 30 * 24 * 60 * 60 * 1000);

		const scanAgeDistribution = {
			today: { openscap: 0, "docker-bench": 0 },
			this_week: { openscap: 0, "docker-bench": 0 },
			this_month: { openscap: 0, "docker-bench": 0 },
			older: { openscap: 0, "docker-bench": 0 },
		};

		// Get the most recent scan per host per profile type
		const hostLastScansByType = new Map(); // key: `${host_id}:${profile_type}`
		for (const scan of latestScans) {
			const profileType = profileTypes[scan.profile_id] || "unknown";
			const key = `${scan.host_id}:${profileType}`;
			const existing = hostLastScansByType.get(key);
			if (
				!existing ||
				new Date(scan.completed_at) > new Date(existing.completed_at)
			) {
				hostLastScansByType.set(key, { ...scan, profileType });
			}
		}

		for (const scan of hostLastScansByType.values()) {
			const scanDate = new Date(scan.completed_at);
			const type = scan.profileType;
			if (type !== "openscap" && type !== "docker-bench") continue;

			if (scanDate >= oneDayAgo) {
				scanAgeDistribution.today[type]++;
			} else if (scanDate >= oneWeekAgo) {
				scanAgeDistribution.this_week[type]++;
			} else if (scanDate >= oneMonthAgo) {
				scanAgeDistribution.this_month[type]++;
			} else {
				scanAgeDistribution.older[type]++;
			}
		}

		const payload = {
			summary: {
				total_hosts: totalHosts,
				average_score: Math.round(avgScore * 100) / 100,
				// Host-level status (based on worst score per host)
				hosts_compliant,
				hosts_warning,
				hosts_critical,
				unscanned,
				hosts_with_compliance_enabled,
				// Host status breakdown by scan type (which scan type caused the status)
				host_status_by_scan_type: hostStatusByScanType,
				// Scan-level counts (for backwards compatibility)
				compliant,
				warning,
				critical,
				// Total scans
				total_scans: latestScans.length,
				// Rule totals
				total_passed_rules: totalPassedRules,
				total_failed_rules: totalFailedRules,
				total_rules: totalRules,
			},
			recent_scans: transformedRecentScans,
			hosts_with_latest_scan: hosts_with_latest_scan,
			worst_hosts: worstHosts,
			top_failing_rules: topFailingRules.map((r) => ({
				rule_id: r.rule_id,
				title: r.title,
				severity: r.severity,
				profile_type: r.profile_type,
				fail_count: Number(r.fail_count),
			})),
			top_warning_rules: topWarningRules.map((r) => ({
				rule_id: r.rule_id,
				title: r.title,
				severity: r.severity,
				profile_type: r.profile_type,
				warn_count: Number(r.warn_count),
			})),
			profile_distribution: profileDistribution.map((p) => ({
				name: p.profile_name,
				type: p.profile_type,
				host_count: Number(p.host_count),
			})),
			severity_breakdown: severityBreakdown.map((s) => ({
				severity: s.severity || "unknown",
				count: Number(s.count),
			})),
			severity_by_profile_type: severityByProfileType.map((s) => ({
				severity: s.severity || "unknown",
				profile_type: s.profile_type,
				count: Number(s.count),
			})),
			docker_bench_by_section: dockerBenchBySection.map((s) => ({
				section: s.section || "Unknown",
				count: Number(s.count),
			})),
			scan_age_distribution: scanAgeDistribution,
			profile_type_stats: profileTypeStats.map((p) => ({
				type: p.profile_type,
				hosts_scanned: Number(p.hosts_scanned),
				average_score: p.average_score
					? Math.round(Number(p.average_score) * 100) / 100
					: null,
				total_passed: Number(p.total_passed) || 0,
				total_failed: Number(p.total_failed) || 0,
				total_warnings: Number(p.total_warnings) || 0,
				total_rules: Number(p.total_rules) || 0,
			})),
		};

		dashboard_cache = {
			data: payload,
			expires: Date.now() + DASHBOARD_CACHE_TTL_MS,
		};
		res.json(payload);
	} catch (error) {
		logger.error("[Compliance] Error fetching dashboard:", error);
		res.status(500).json({ error: "Failed to fetch dashboard data" });
	}
});

/**
 * GET /api/v1/compliance/scans/active
 * Get all currently running compliance scans
 */
router.get("/scans/active", async (_req, res) => {
	try {
		// Get all scans that are still running (completed_at is null or status is "running")
		const activeScans = await prisma.compliance_scans.findMany({
			where: {
				OR: [
					{ status: "running" },
					{ completed_at: null, status: { not: "failed" } },
				],
			},
			orderBy: { started_at: "desc" },
			include: {
				hosts: {
					select: {
						id: true,
						hostname: true,
						friendly_name: true,
						api_id: true,
					},
				},
				compliance_profiles: {
					select: {
						name: true,
						type: true,
					},
				},
			},
		});

		// Add connection status for each host
		const scansWithStatus = activeScans.map((scan) => {
			const connected = agentWs.isConnected(scan.hosts?.api_id);
			return {
				id: scan.id,
				hostId: scan.host_id,
				hostName: scan.hosts?.friendly_name || scan.hosts?.hostname,
				apiId: scan.hosts?.api_id,
				profileName: scan.compliance_profiles?.name,
				profileType: scan.compliance_profiles?.type,
				startedAt: scan.started_at,
				status: scan.status,
				connected,
			};
		});

		res.json({
			activeScans: scansWithStatus,
			count: scansWithStatus.length,
		});
	} catch (error) {
		logger.error("[Compliance] Error fetching active scans:", error);
		res.status(500).json({ error: "Failed to fetch active scans" });
	}
});

/**
 * GET /api/v1/compliance/scans/history
 * Global scan history across all hosts (paginated, filterable)
 * Query params:
 *   - limit (default 25, max 100)
 *   - offset (default 0)
 *   - status: "completed", "failed", "running"
 *   - profile_type: "openscap", "docker-bench"
 *   - host_id: filter to a single host
 */
router.get("/scans/history", async (req, res) => {
	try {
		const limit = sanitizeInt(req.query.limit, 25, 1, 100);
		const offset = sanitizeInt(req.query.offset, 0, 0, 100000);
		const { status, profile_type, host_id } = req.query;

		const where = {};
		if (status) where.status = status;
		if (host_id && uuidValidate(host_id)) where.host_id = host_id;
		if (profile_type) {
			where.compliance_profiles = { type: profile_type };
		}

		const [scans, total] = await Promise.all([
			prisma.compliance_scans.findMany({
				where,
				orderBy: { started_at: "desc" },
				take: limit,
				skip: offset,
				select: {
					id: true,
					host_id: true,
					status: true,
					started_at: true,
					completed_at: true,
					total_rules: true,
					passed: true,
					failed: true,
					warnings: true,
					skipped: true,
					not_applicable: true,
					score: true,
					error_message: true,
					raw_output: false,
					hosts: {
						select: {
							id: true,
							hostname: true,
							friendly_name: true,
						},
					},
					compliance_profiles: {
						select: { name: true, type: true },
					},
					_count: { select: { compliance_results: true } },
				},
			}),
			prisma.compliance_scans.count({ where }),
		]);

		const rows = scans.map((s) => {
			const duration_ms =
				s.completed_at && s.started_at
					? new Date(s.completed_at) - new Date(s.started_at)
					: null;
			return {
				id: s.id,
				host_id: s.host_id,
				host_name: s.hosts?.friendly_name || s.hosts?.hostname || "Unknown",
				profile_name: s.compliance_profiles?.name || "Unknown",
				profile_type: s.compliance_profiles?.type || "unknown",
				status: s.status,
				started_at: s.started_at,
				completed_at: s.completed_at,
				duration_ms,
				total_rules: s.total_rules,
				passed: s.passed,
				failed: s.failed,
				warnings: s.warnings,
				skipped: s.skipped,
				not_applicable: s.not_applicable,
				score: s.score,
				results_stored: s._count.compliance_results,
				error_message: s.error_message,
			};
		});

		res.json({
			scans: rows,
			pagination: {
				total,
				limit,
				offset,
				total_pages: Math.ceil(total / limit),
			},
		});
	} catch (error) {
		logger.error("[Compliance] Error fetching scan history:", error);
		res.status(500).json({ error: "Failed to fetch scan history" });
	}
});

/**
 * GET /api/v1/compliance/scans/:hostId
 * Get scan history for a specific host
 */
router.get("/scans/:hostId", async (req, res) => {
	try {
		const { hostId } = req.params;

		// Validate hostId
		if (!isValidUUID(hostId)) {
			return res.status(400).json({ error: "Invalid host ID format" });
		}

		// Sanitize pagination params
		const limit = sanitizeInt(req.query.limit, 20, 1, 100);
		const offset = sanitizeInt(req.query.offset, 0, 0, 10000);

		const scans = await prisma.compliance_scans.findMany({
			where: { host_id: hostId },
			orderBy: { completed_at: "desc" },
			take: limit,
			skip: offset,
			include: {
				compliance_profiles: { select: { name: true, type: true } },
				_count: { select: { compliance_results: true } },
			},
		});

		const total = await prisma.compliance_scans.count({
			where: { host_id: hostId },
		});

		res.json({
			scans,
			pagination: {
				total,
				limit,
				offset,
			},
		});
	} catch (error) {
		logger.error("[Compliance] Error fetching scans:", error);
		res.status(500).json({ error: "Failed to fetch scans" });
	}
});

/**
 * GET /api/v1/compliance/scans/:hostId/latest
 * Get the latest scan for a host
 * Query params:
 *   - profile_type: Filter by profile type (openscap, docker-bench)
 */
router.get("/scans/:hostId/latest", async (req, res) => {
	try {
		const { hostId } = req.params;
		const { profile_type } = req.query;

		// Validate hostId
		if (!isValidUUID(hostId)) {
			return res.status(400).json({ error: "Invalid host ID format" });
		}

		// Build where clause
		const where = { host_id: hostId, status: "completed" };

		// Filter by profile type if specified
		if (profile_type) {
			where.compliance_profiles = { type: profile_type };
		}

		const scan = await prisma.compliance_scans.findFirst({
			where,
			orderBy: { completed_at: "desc" },
			include: {
				compliance_profiles: true,
				compliance_results: {
					include: {
						compliance_rules: true,
					},
					orderBy: [
						{ status: "asc" }, // fail first
					],
				},
			},
		});

		if (!scan) {
			return res.status(404).json({ error: "No scans found for this host" });
		}

		res.json(scan);
	} catch (error) {
		logger.error("[Compliance] Error fetching latest scan:", error);
		res.status(500).json({ error: "Failed to fetch latest scan" });
	}
});

/**
 * GET /api/v1/compliance/scans/:hostId/latest-by-type
 * Get the latest scan for each profile type (openscap, docker-bench)
 * Returns summary info for tab display
 */
router.get("/scans/:hostId/latest-by-type", async (req, res) => {
	try {
		const { hostId } = req.params;

		// Validate hostId
		if (!isValidUUID(hostId)) {
			return res.status(400).json({ error: "Invalid host ID format" });
		}

		// Get all completed scans for this host with their profile types
		const scans = await prisma.compliance_scans.findMany({
			where: {
				host_id: hostId,
				status: "completed",
			},
			orderBy: { completed_at: "desc" },
			include: {
				compliance_profiles: {
					select: { type: true, name: true },
				},
			},
		});

		// Group by profile type and get the latest for each
		const results = {};
		const scanIds = [];

		for (const scan of scans) {
			const type = scan.compliance_profiles?.type;
			if (!type) continue;

			// Only keep the first (latest) scan for each type
			if (!results[type]) {
				results[type] = {
					id: scan.id,
					profile_name: scan.compliance_profiles?.name,
					profile_type: type,
					score: scan.score,
					total_rules: scan.total_rules,
					passed: scan.passed,
					failed: scan.failed,
					warnings: scan.warnings,
					skipped: scan.skipped,
					completed_at: scan.completed_at,
				};
				scanIds.push(scan.id);
			}
		}

		// Get severity breakdown for OpenSCAP scans and section breakdown for Docker Bench
		if (scanIds.length > 0) {
			const severityOrder = (a, b) => {
				const order = { critical: 1, high: 2, medium: 3, low: 4 };
				return (order[a] ?? 5) - (order[b] ?? 5);
			};
			// Severity breakdown for OpenSCAP failures
			if (results.openscap) {
				const rows = await prisma.compliance_results.findMany({
					where: {
						scan_id: results.openscap.id,
						status: "fail",
					},
					select: { compliance_rules: { select: { severity: true } } },
				});
				const bySeverity = rows.reduce((acc, r) => {
					const s = r.compliance_rules?.severity ?? "unknown";
					acc[s] = (acc[s] || 0) + 1;
					return acc;
				}, {});
				results.openscap.severity_breakdown = Object.entries(bySeverity)
					.map(([severity, count]) => ({ severity, count: Number(count) }))
					.sort((a, b) => severityOrder(a.severity, b.severity));
			}

			// Section breakdown for Docker Bench warnings
			if (results["docker-bench"]) {
				const rows = await prisma.compliance_results.findMany({
					where: {
						scan_id: results["docker-bench"].id,
						status: "warn",
					},
					select: { compliance_rules: { select: { section: true } } },
				});
				const bySection = rows.reduce((acc, r) => {
					const s = r.compliance_rules?.section ?? "Unknown";
					acc[s] = (acc[s] || 0) + 1;
					return acc;
				}, {});
				results["docker-bench"].section_breakdown = Object.entries(bySection)
					.map(([section, count]) => ({ section, count: Number(count) }))
					.sort((a, b) => (a.section || "").localeCompare(b.section || ""));
			}
		}

		res.json(results);
	} catch (error) {
		logger.error("[Compliance] Error fetching latest scans by type:", error);
		res.status(500).json({ error: "Failed to fetch latest scans by type" });
	}
});

/**
 * GET /api/v1/compliance/results/:scanId
 * Get detailed results for a specific scan (paginated).
 * Query params: status, severity, limit (default 50, max 100), offset (default 0)
 */
router.get("/results/:scanId", async (req, res) => {
	try {
		const { scanId } = req.params;
		const {
			status,
			severity,
			limit: limitParam,
			offset: offsetParam,
		} = req.query;

		// Validate scanId
		if (!isValidUUID(scanId)) {
			return res.status(400).json({ error: "Invalid scan ID format" });
		}

		// Validate status filter if provided
		if (status && !VALID_RESULT_STATUSES.includes(status)) {
			return res.status(400).json({
				error: `Invalid status. Must be one of: ${VALID_RESULT_STATUSES.join(", ")}`,
			});
		}

		// Validate severity filter if provided
		if (severity && !VALID_SEVERITIES.includes(severity)) {
			return res.status(400).json({
				error: `Invalid severity. Must be one of: ${VALID_SEVERITIES.join(", ")}`,
			});
		}

		const limit = sanitizeInt(limitParam, 50, 1, 100);
		const offset = Math.max(0, parseInt(offsetParam, 10) || 0);

		const where = { scan_id: scanId };
		if (status) {
			const db_values = statusFilterToDbValues(status);
			where.status = db_values.length === 1 ? db_values[0] : { in: db_values };
		}

		// Severity filter: apply via relation (required for count and findMany)
		if (severity) {
			where.compliance_rules = { severity };
		}

		const baseWhere = { scan_id: scanId };

		const [all_results, statusCounts, severityRows] = await Promise.all([
			prisma.compliance_results.findMany({
				where,
				include: {
					compliance_rules: true,
				},
			}),
			offset === 0
				? prisma.compliance_results.groupBy({
						by: ["status"],
						where: baseWhere,
						_count: true,
					})
				: Promise.resolve([]),
			offset === 0
				? (async () => {
						const rows = await prisma.compliance_results.findMany({
							where: {
								scan_id: scanId,
								status: { in: ["fail", "failed", "failure"] },
							},
							select: { compliance_rules: { select: { severity: true } } },
						});
						const bySeverity = rows.reduce((acc, r) => {
							const s = r.compliance_rules?.severity ?? "unknown";
							acc[s] = (acc[s] || 0) + 1;
							return acc;
						}, {});
						return Object.entries(bySeverity).map(([severity, count]) => ({
							severity,
							count,
						}));
					})()
				: Promise.resolve([]),
		]);

		// Order: Failed first, then Warning, then Passed, then N/A/skip/error
		all_results.sort(
			(a, b) => status_rank_for_sort(a.status) - status_rank_for_sort(b.status),
		);
		const total = all_results.length;
		const results = all_results.slice(offset, offset + limit);

		const payload = {
			results,
			pagination: {
				total,
				limit,
				offset,
			},
		};
		if (offset === 0 && (statusCounts.length > 0 || severityRows.length > 0)) {
			payload.severity_breakdown = {
				by_status: Object.fromEntries(
					statusCounts.map((s) => [s.status, s._count]),
				),
				by_severity: Object.fromEntries(
					severityRows.map((r) => [r.severity || "unknown", r.count]),
				),
			};
		}
		res.json(payload);
	} catch (error) {
		logger.error("[Compliance] Error fetching results:", error);
		res.status(500).json({ error: "Failed to fetch results" });
	}
});

/**
 * POST /api/v1/compliance/trigger/bulk
 * Trigger compliance scans on multiple hosts at once
 * NOTE: This route MUST be defined before /trigger/:hostId to prevent "bulk" being matched as a hostId
 */
router.post("/trigger/bulk", async (req, res) => {
	try {
		const {
			hostIds = [],
			profile_type = "all",
			profile_id = null,
			enable_remediation = false,
			fetch_remote_resources = false,
		} = req.body;

		logger.info(
			`[Compliance] Bulk trigger received: ${hostIds.length} hosts, profile_type=${profile_type}`,
		);

		// Validate hostIds array
		if (!Array.isArray(hostIds) || hostIds.length === 0) {
			return res
				.status(400)
				.json({ error: "hostIds must be a non-empty array" });
		}

		if (hostIds.length > 100) {
			return res
				.status(400)
				.json({ error: "Maximum 100 hosts per bulk operation" });
		}

		// Validate all UUIDs
		const invalidIds = hostIds.filter((id) => !isValidUUID(id));
		if (invalidIds.length > 0) {
			return res
				.status(400)
				.json({ error: `Invalid host IDs: ${invalidIds.join(", ")}` });
		}

		// Validate profile_type
		if (!VALID_PROFILE_TYPES.includes(profile_type)) {
			return res.status(400).json({
				error: `Invalid profile_type. Must be one of: ${VALID_PROFILE_TYPES.join(", ")}`,
			});
		}

		// Get all hosts
		const hosts = await prisma.hosts.findMany({
			where: { id: { in: hostIds } },
			select: {
				id: true,
				api_id: true,
				hostname: true,
				friendly_name: true,
				compliance_openscap_enabled: true,
				compliance_docker_bench_enabled: true,
			},
		});

		const hostMap = new Map(hosts.map((h) => [h.id, h]));

		const results = {
			triggered: [],
			failed: [],
		};

		// Build scan options
		const scanOptions = {
			profileId: profile_id,
			enableRemediation: Boolean(enable_remediation),
			fetchRemoteResources: Boolean(fetch_remote_resources),
		};

		// Get or create profiles for running scan records
		const profilesToUse = [];
		if (profile_type === "all" || profile_type === "openscap") {
			let oscapProfile = await prisma.compliance_profiles.findFirst({
				where: { type: "openscap" },
				orderBy: { name: "asc" },
			});
			if (!oscapProfile) {
				// Create placeholder profile
				oscapProfile = await prisma.compliance_profiles.create({
					data: {
						id: uuidv4(),
						name: "OpenSCAP Scan",
						type: "openscap",
					},
				});
			}
			profilesToUse.push(oscapProfile);
		}
		if (profile_type === "all" || profile_type === "docker-bench") {
			let dockerProfile = await prisma.compliance_profiles.findFirst({
				where: { type: "docker-bench" },
				orderBy: { name: "asc" },
			});
			if (!dockerProfile) {
				// Create placeholder profile
				dockerProfile = await prisma.compliance_profiles.create({
					data: {
						id: uuidv4(),
						name: "Docker Bench Security",
						type: "docker-bench",
					},
				});
			}
			profilesToUse.push(dockerProfile);
		}

		logger.info(
			`[Compliance] Bulk: Found ${profilesToUse.length} profiles to use: ${profilesToUse.map((p) => p.name).join(", ")}`,
		);

		// Process each host
		for (const hostId of hostIds) {
			const host = hostMap.get(hostId);

			if (!host) {
				results.failed.push({ hostId, error: "Host not found" });
				continue;
			}

			if (!agentWs.isConnected(host.api_id)) {
				results.failed.push({
					hostId,
					hostName: host.friendly_name || host.hostname,
					error: "Host not connected",
				});
				continue;
			}

			const oscapOn = host.compliance_openscap_enabled ?? true;
			const dbenchOn = host.compliance_docker_bench_enabled ?? false;

			let hostProfileType = profile_type;
			if (profile_type === "all" || profile_type === "" || !profile_type) {
				if (oscapOn && !dbenchOn) hostProfileType = "openscap";
				else if (!oscapOn && dbenchOn) hostProfileType = "docker-bench";
				else if (!oscapOn && !dbenchOn) {
					results.failed.push({
						hostId,
						hostName: host.friendly_name || host.hostname,
						error: "Both scanners disabled",
					});
					continue;
				}
			}

			const hostScanOptions = {
				...scanOptions,
				openscapEnabled: oscapOn,
				dockerBenchEnabled: dbenchOn,
			};
			const success = agentWs.pushComplianceScan(
				host.api_id,
				hostProfileType,
				hostScanOptions,
			);

			if (success) {
				// Create "running" scan records only for the profile type we actually sent to the agent
				const profilesForHost =
					hostProfileType === "all"
						? profilesToUse
						: profilesToUse.filter((p) => p.type === hostProfileType);
				for (const profile of profilesForHost) {
					try {
						await prisma.compliance_scans.create({
							data: {
								id: uuidv4(),
								host_id: hostId,
								profile_id: profile.id,
								started_at: new Date(),
								completed_at: null,
								status: "running",
								total_rules: 0,
								passed: 0,
								failed: 0,
								warnings: 0,
								skipped: 0,
								not_applicable: 0,
								score: null,
							},
						});
					} catch (err) {
						logger.warn(
							`[Compliance] Could not create running scan record: ${err.message}`,
						);
					}
				}

				results.triggered.push({
					hostId,
					hostName: host.friendly_name || host.hostname,
					apiId: host.api_id,
				});
			} else {
				results.failed.push({
					hostId,
					hostName: host.friendly_name || host.hostname,
					error: "Failed to send trigger command",
				});
			}
		}

		const message = `Triggered ${results.triggered.length} of ${hostIds.length} scans`;
		logger.info(`[Compliance] Bulk scan: ${message}`);

		res.json({
			message,
			profile_type,
			enable_remediation,
			triggered: results.triggered,
			failed: results.failed,
			summary: {
				total: hostIds.length,
				success: results.triggered.length,
				failed: results.failed.length,
			},
		});
	} catch (error) {
		logger.error("[Compliance] Error triggering bulk scan:", error);
		res.status(500).json({ error: "Failed to trigger bulk scan" });
	}
});

/**
 * POST /api/v1/compliance/trigger/:hostId
 * Trigger an on-demand compliance scan
 */
router.post("/trigger/:hostId", async (req, res) => {
	try {
		const { hostId } = req.params;
		const {
			profile_type = "all",
			profile_id = null,
			enable_remediation = false,
			fetch_remote_resources = false,
			image_name = null,
			scan_all_images = false,
		} = req.body;

		// Validate hostId
		if (!isValidUUID(hostId)) {
			return res.status(400).json({ error: "Invalid host ID format" });
		}

		// Validate profile_type
		if (!VALID_PROFILE_TYPES.includes(profile_type)) {
			return res.status(400).json({
				error: `Invalid profile_type. Must be one of: ${VALID_PROFILE_TYPES.join(", ")}`,
			});
		}

		const host = await prisma.hosts.findUnique({
			where: { id: hostId },
		});

		if (!host) {
			return res.status(404).json({ error: "Host not found" });
		}

		// Docker image CVE scan: must be connected, no queue
		if (profile_type === "oscap-docker") {
			if (!agentWs.isConnected(host.api_id)) {
				return res.status(400).json({
					error: "Docker image CVE scan requires the agent to be connected.",
				});
			}
			const dockerScanOptions = {
				imageName: image_name || null,
				scanAllImages: Boolean(scan_all_images),
			};
			const success = agentWs.pushDockerImageScan(
				host.api_id,
				dockerScanOptions,
			);
			if (success) {
				return res.json({
					message: "Docker image CVE scan triggered",
					host_id: hostId,
					profile_type,
					scan_all_images: Boolean(scan_all_images),
					image_name: image_name || null,
				});
			}
			return res
				.status(400)
				.json({ error: "Failed to send Docker image scan trigger" });
		}

		// All other scans (all, openscap, docker-bench): always go through BullMQ so we always have a job_id
		const queue = queueManager.queues[QUEUE_NAMES.COMPLIANCE];
		if (!queue) {
			logger.error("[Compliance] Compliance queue not initialized");
			return res.status(503).json({
				error: "Scan queue is not available. Please try again later.",
			});
		}
		const job_id = `${COMPLIANCE_SCAN_JOB_ID_PREFIX}${hostId}`;
		try {
			const existing = await queue.getJob(job_id);
			if (existing) {
				const state = await existing.getState();
				if (state === "waiting" || state === "delayed") {
					return res.json({
						message: "Scan already queued for this host",
						host_id: hostId,
						queued: true,
						already_queued: true,
						job_id: existing.id,
					});
				}
				// Completed/failed: remove so we can add a fresh job (max 1 per host)
				await existing.remove().catch(() => {});
			}
			const job = await queue.add(
				"run_scan",
				{
					type: "run_scan",
					hostId,
					api_id: host.api_id,
					profile_type,
					profile_id: profile_id || null,
					enable_remediation: Boolean(enable_remediation),
					fetch_remote_resources: Boolean(fetch_remote_resources),
				},
				{
					jobId: job_id,
					attempts: 1,
					backoff: {
						type: "fixed",
						delay: COMPLIANCE_SCAN_QUEUE_RETRY_DELAY_MS,
					},
				},
			);
			const connected = agentWs.isConnected(host.api_id);
			const safe_host_id = String(hostId).replace(/\r|\n/g, "");
			logger.info(
				`[Compliance] Scan queued for host ${safe_host_id}; job_id=${job.id} agent_connected=${connected}`,
			);
			return res.json({
				message: connected
					? enable_remediation
						? "Compliance scan with remediation triggered"
						: "Compliance scan triggered"
					: "Scan queued; will run when agent is online",
				host_id: hostId,
				queued: true,
				job_id: job.id,
				profile_type,
				enable_remediation,
			});
		} catch (err) {
			logger.error("[Compliance] Error queuing scan:", err);
			return res.status(500).json({
				error: "Failed to queue scan. Please try again.",
			});
		}
	} catch (error) {
		logger.error("[Compliance] Error triggering scan:", error);
		res.status(500).json({ error: "Failed to trigger scan" });
	}
});

/**
 * POST /api/v1/compliance/cancel/:hostId
 * Request the agent to cancel the currently running compliance scan (if any)
 */
router.post("/cancel/:hostId", async (req, res) => {
	try {
		const { hostId } = req.params;

		if (!isValidUUID(hostId)) {
			return res.status(400).json({ error: "Invalid host ID format" });
		}

		const host = await prisma.hosts.findUnique({
			where: { id: hostId },
		});

		if (!host) {
			return res.status(404).json({ error: "Host not found" });
		}

		if (!agentWs.isConnected(host.api_id)) {
			return res.status(400).json({ error: "Host is not connected" });
		}

		const success = agentWs.pushComplianceScanCancel(host.api_id);

		if (success) {
			res.json({
				message: "Cancel scan request sent",
				host_id: hostId,
			});
		} else {
			res.status(400).json({ error: "Failed to send cancel request" });
		}
	} catch (error) {
		logger.error("[Compliance] Error sending cancel scan:", error);
		res.status(500).json({ error: "Failed to send cancel request" });
	}
});

/**
 * GET /api/v1/compliance/install-job/:hostId
 * Return current install job status for progress polling.
 */
router.get("/install-job/:hostId", async (req, res) => {
	try {
		const { hostId } = req.params;

		if (!isValidUUID(hostId)) {
			return res.status(400).json({ error: "Invalid host ID format" });
		}

		const host = await prisma.hosts.findUnique({
			where: { id: hostId },
			select: { id: true },
		});

		if (!host) {
			return res.status(404).json({ error: "Host not found" });
		}

		const jobKey = `${COMPLIANCE_INSTALL_JOB_PREFIX}${hostId}`;
		const jobId = await redis.get(jobKey);

		if (!jobId) {
			return res.json({
				status: "none",
				message: "No active or recent install job",
			});
		}

		const queue = queueManager.queues[QUEUE_NAMES.COMPLIANCE];
		const job = await queue.getJob(jobId);

		if (!job) {
			await redis.del(jobKey).catch(() => {});
			return res.json({
				job_id: jobId,
				status: "completed",
				message: "Job no longer in queue (completed or expired)",
			});
		}

		const state = await job.getState();
		res.json({
			job_id: job.id,
			status: state,
			progress: job.progress,
			message: job.data?.message,
			install_events: job.data?.install_events || [],
			error: job.failedReason,
		});
	} catch (error) {
		logger.error("[Compliance] Error getting install job status:", error);
		res.status(500).json({ error: "Failed to get install job status" });
	}
});

/**
 * POST /api/v1/compliance/install-scanner/:hostId/cancel
 * Signal worker to cancel the current install job for this host.
 * Defined before /install-scanner/:hostId so "cancel" is not parsed as hostId.
 */
router.post("/install-scanner/:hostId/cancel", async (req, res) => {
	try {
		const { hostId } = req.params;

		if (!isValidUUID(hostId)) {
			return res.status(400).json({ error: "Invalid host ID format" });
		}

		const jobKey = `${COMPLIANCE_INSTALL_JOB_PREFIX}${hostId}`;
		const jobId = await redis.get(jobKey);

		if (!jobId) {
			return res
				.status(404)
				.json({ error: "No active install job for this host" });
		}

		const cancelKey = `${COMPLIANCE_INSTALL_CANCEL_PREFIX}${jobId}`;
		await redis.set(cancelKey, "1", "EX", 300);

		await redis.del(jobKey).catch(() => {});

		res.json({ message: "Cancel requested" });
	} catch (error) {
		logger.error("[Compliance] Error cancelling install job:", error);
		res.status(500).json({ error: "Failed to cancel install job" });
	}
});

/**
 * POST /api/v1/compliance/install-scanner/:hostId
 * Enqueue OpenSCAP/SSG install job; worker sends install_scanner to agent and polls until ready.
 */
router.post("/install-scanner/:hostId", async (req, res) => {
	try {
		const { hostId } = req.params;

		if (!isValidUUID(hostId)) {
			return res.status(400).json({ error: "Invalid host ID format" });
		}

		const host = await prisma.hosts.findUnique({
			where: { id: hostId },
		});

		if (!host) {
			return res.status(404).json({ error: "Host not found" });
		}

		if (!agentWs.isConnected(host.api_id)) {
			return res.status(400).json({ error: "Host is not connected" });
		}

		const queue = queueManager.queues[QUEUE_NAMES.COMPLIANCE];
		const job = await queue.add(
			"install_compliance_tools",
			{ hostId, api_id: host.api_id, type: "install_compliance_tools" },
			{ attempts: 1 },
		);

		const jobKey = `${COMPLIANCE_INSTALL_JOB_PREFIX}${hostId}`;
		await redis.setex(jobKey, COMPLIANCE_INSTALL_JOB_TTL, job.id);

		res.json({
			job_id: job.id,
			host_id: hostId,
			message: "Install job queued",
		});
	} catch (error) {
		logger.error("[Compliance] Error enqueueing install scanner:", error);
		res.status(500).json({ error: "Failed to enqueue install scanner" });
	}
});

/**
 * POST /api/v1/compliance/upgrade-ssg/:hostId
 * Trigger SSG content package upgrade on the agent
 */
router.post("/upgrade-ssg/:hostId", async (req, res) => {
	try {
		const { hostId } = req.params;

		// Validate hostId
		if (!isValidUUID(hostId)) {
			return res.status(400).json({ error: "Invalid host ID format" });
		}

		const host = await prisma.hosts.findUnique({
			where: { id: hostId },
		});

		if (!host) {
			return res.status(404).json({ error: "Host not found" });
		}

		// Use agentWs service to send SSG upgrade command
		if (!agentWs.isConnected(host.api_id)) {
			return res.status(400).json({ error: "Host is not connected" });
		}

		const success = agentWs.pushUpgradeSSG(host.api_id);

		if (success) {
			res.json({
				message: "SSG upgrade command sent",
				host_id: hostId,
			});
		} else {
			res.status(400).json({ error: "Failed to send SSG upgrade command" });
		}
	} catch (error) {
		logger.error("[Compliance] Error triggering SSG upgrade:", error);
		res.status(500).json({ error: "Failed to trigger SSG upgrade" });
	}
});

/**
 * POST /api/v1/compliance/remediate/:hostId
 * Remediate a single failed rule on the agent
 */
router.post("/remediate/:hostId", async (req, res) => {
	try {
		const { hostId } = req.params;
		const { rule_id } = req.body;

		// Validate hostId
		if (!isValidUUID(hostId)) {
			return res.status(400).json({ error: "Invalid host ID format" });
		}

		// Validate rule_id
		if (!rule_id || typeof rule_id !== "string") {
			return res.status(400).json({ error: "rule_id is required" });
		}

		const host = await prisma.hosts.findUnique({
			where: { id: hostId },
		});

		if (!host) {
			return res.status(404).json({ error: "Host not found" });
		}

		// Use agentWs service to send remediation command
		if (!agentWs.isConnected(host.api_id)) {
			return res.status(400).json({ error: "Host is not connected" });
		}

		const success = agentWs.pushRemediateRule(host.api_id, rule_id);

		if (success) {
			logger.info(
				`[Compliance] Single rule remediation triggered for ${host.api_id}: ${rule_id}`,
			);
			res.json({
				message: "Remediation command sent",
				host_id: hostId,
				rule_id: rule_id,
			});
		} else {
			res.status(400).json({ error: "Failed to send remediation command" });
		}
	} catch (error) {
		logger.error("[Compliance] Error triggering remediation:", error);
		res.status(500).json({ error: "Failed to trigger remediation" });
	}
});

/**
 * GET /api/v1/compliance/trends/:hostId
 * Get compliance score trends over time
 */
router.get("/trends/:hostId", async (req, res) => {
	try {
		const { hostId } = req.params;

		// Validate hostId
		if (!isValidUUID(hostId)) {
			return res.status(400).json({ error: "Invalid host ID format" });
		}

		// Sanitize days param (1-365)
		const days = sanitizeInt(req.query.days, 30, 1, 365);

		const since = new Date();
		since.setDate(since.getDate() - days);

		const scans = await prisma.compliance_scans.findMany({
			where: {
				host_id: hostId,
				status: "completed",
				completed_at: { gte: since },
			},
			orderBy: { completed_at: "asc" },
			select: {
				completed_at: true,
				score: true,
				compliance_profiles: { select: { name: true, type: true } },
			},
		});

		res.json(scans);
	} catch (error) {
		logger.error("[Compliance] Error fetching trends:", error);
		res.status(500).json({ error: "Failed to fetch trends" });
	}
});

// ==========================================
// Rule-centric endpoints (cross-host view)
// ==========================================

/**
 * GET /api/v1/compliance/rules
 * List all compliance rules with aggregated pass/fail/warn counts across hosts.
 * Uses only the latest completed scan per host per profile to avoid counting old results.
 * Query params: severity, status, search, limit, offset, profile_type
 */
router.get("/rules", async (req, res) => {
	try {
		const limit = sanitizeInt(req.query.limit, 50, 1, 200);
		const offset = sanitizeInt(req.query.offset, 0, 0, 100000);
		const severity = req.query.severity;
		const status_filter = req.query.status; // "fail", "warn", "pass", or null for all
		const search = req.query.search ? String(req.query.search).trim() : null;
		const profile_type = req.query.profile_type;
		const host_id = req.query.host_id;
		const sort_by = req.query.sort_by || "status"; // status, severity, title, hosts_failed, hosts_warned, hosts_passed, total_hosts, profile_type
		const sort_dir = req.query.sort_dir === "asc" ? "asc" : "desc";

		// Validate optional params
		if (severity && !VALID_SEVERITIES.includes(severity)) {
			return res.status(400).json({ error: "Invalid severity filter" });
		}
		if (status_filter && !["pass", "fail", "warn"].includes(status_filter)) {
			return res.status(400).json({ error: "Invalid status filter" });
		}
		if (profile_type && !VALID_PROFILE_TYPES.includes(profile_type)) {
			return res.status(400).json({ error: "Invalid profile_type filter" });
		}
		if (host_id && !isValidUUID(host_id)) {
			return res.status(400).json({ error: "Invalid host_id format" });
		}

		// Build rule where clause
		const rule_where = {};
		if (severity) {
			if (severity === "unknown") {
				// Match both null and string "unknown"
				rule_where.OR = [{ severity: null }, { severity: "unknown" }];
			} else {
				rule_where.severity = severity;
			}
		}
		if (search) {
			const search_or = [
				{ title: { contains: search, mode: "insensitive" } },
				{ rule_ref: { contains: search, mode: "insensitive" } },
				{ section: { contains: search, mode: "insensitive" } },
			];
			if (rule_where.OR) {
				// severity=unknown already set OR; combine with AND
				rule_where.AND = [{ OR: rule_where.OR }, { OR: search_or }];
				delete rule_where.OR;
			} else {
				rule_where.OR = search_or;
			}
		}
		if (profile_type && profile_type !== "all") {
			rule_where.compliance_profiles = { type: profile_type };
		}

		// Get latest completed scan ID per host per profile (avoids counting old results)
		const latest_scans_list = await getLatestCompletedScans(prisma, {
			select: { id: true, host_id: true, profile_id: true },
		});
		const filtered_scans = host_id
			? latest_scans_list.filter((s) => s.host_id === host_id)
			: latest_scans_list;
		const latest_scan_ids = filtered_scans.map((s) => s.id);

		if (latest_scan_ids.length === 0) {
			return res.json({ rules: [], pagination: { total: 0, limit, offset } });
		}

		// Restrict to rules that appear in scan results: only failing or compliant (passing) rules; exclude not discovered and warning-only
		// Use status variants (fail/failed/failure, etc.) so stored values match
		if (status_filter) {
			const status_values = statusFilterToDbValues(status_filter);
			const rule_results = await prisma.compliance_results.findMany({
				where: {
					scan_id: { in: latest_scan_ids },
					status: { in: status_values },
				},
				select: { rule_id: true },
				distinct: ["rule_id"],
			});
			const ids = rule_results.map((r) => r.rule_id).filter(Boolean);
			if (ids.length === 0) {
				return res.json({ rules: [], pagination: { total: 0, limit, offset } });
			}
			rule_where.id = { in: ids };
		} else {
			// No status filter: show all rules that have at least one result in any latest scan (pass, fail, warn, skip, notapplicable)
			const rule_results = await prisma.compliance_results.findMany({
				where: { scan_id: { in: latest_scan_ids } },
				select: { rule_id: true },
				distinct: ["rule_id"],
			});
			const ids = rule_results.map((r) => r.rule_id).filter(Boolean);
			if (ids.length === 0) {
				return res.json({ rules: [], pagination: { total: 0, limit, offset } });
			}
			rule_where.id = { in: ids };
		}

		// Fetch ALL matching rules with result aggregation (needed for server-side sort before pagination)
		const all_rules = await prisma.compliance_rules.findMany({
			where: rule_where,
			select: {
				id: true,
				rule_ref: true,
				title: true,
				severity: true,
				section: true,
				profile_id: true,
				compliance_profiles: { select: { type: true, name: true } },
				compliance_results: {
					where: { scan_id: { in: latest_scan_ids } },
					select: { status: true },
				},
			},
		});

		// Aggregate counts per rule
		const count_by_status = (results, statuses) =>
			results.filter((x) => statuses.includes(x.status)).length;
		const rules_with_counts = all_rules.map((r) => {
			const results = r.compliance_results || [];
			const hosts_passed = count_by_status(results, ["pass", "passed"]);
			const hosts_failed = count_by_status(results, [
				"fail",
				"failed",
				"failure",
			]);
			const hosts_warned = count_by_status(results, [
				"warn",
				"warning",
				"warned",
			]);
			const total_hosts = results.length;
			return {
				id: r.id,
				rule_ref: r.rule_ref,
				title: r.title,
				severity: r.severity,
				section: r.section,
				profile_id: r.profile_id,
				profile_type: r.compliance_profiles?.type,
				profile_name: r.compliance_profiles?.name,
				hosts_passed,
				hosts_failed,
				hosts_warned,
				total_hosts,
			};
		});

		// Server-side sort so pagination returns the correct slice
		// Higher rank = worse: fail=3, warn=2, pass=1, none=0 — so "desc" means worst first
		const severity_rank = {
			critical: 4,
			high: 3,
			medium: 2,
			low: 1,
			unknown: 0,
		};
		const get_status_rank = (r) => {
			if (r.hosts_failed > 0) return 3;
			if (r.hosts_warned > 0) return 2;
			if (r.hosts_passed > 0) return 1;
			return 0;
		};
		const asc = sort_dir === "asc";
		rules_with_counts.sort((a, b) => {
			let va, vb;
			switch (sort_by) {
				case "status": {
					va = get_status_rank(a);
					vb = get_status_rank(b);
					if (va !== vb) return asc ? va - vb : vb - va;
					// Secondary: severity (worst first when desc)
					va = severity_rank[a.severity] ?? 0;
					vb = severity_rank[b.severity] ?? 0;
					if (va !== vb) return asc ? va - vb : vb - va;
					// Tertiary: fail count (most fails first when desc)
					if (a.hosts_failed !== b.hosts_failed)
						return asc
							? a.hosts_failed - b.hosts_failed
							: b.hosts_failed - a.hosts_failed;
					return (a.title || "").localeCompare(b.title || "", undefined, {
						sensitivity: "base",
					});
				}
				case "severity":
					va = severity_rank[a.severity] ?? 0;
					vb = severity_rank[b.severity] ?? 0;
					if (va !== vb) return asc ? va - vb : vb - va;
					// Secondary: status (worst first)
					va = get_status_rank(a);
					vb = get_status_rank(b);
					if (va !== vb) return vb - va;
					return b.hosts_failed - a.hosts_failed;
				case "title":
					return asc
						? (a.title || "").localeCompare(b.title || "", undefined, {
								sensitivity: "base",
							})
						: (b.title || "").localeCompare(a.title || "", undefined, {
								sensitivity: "base",
							});
				case "profile_type":
					va = a.profile_type || "";
					vb = b.profile_type || "";
					if (va !== vb)
						return asc ? va.localeCompare(vb) : vb.localeCompare(va);
					return (a.title || "").localeCompare(b.title || "", undefined, {
						sensitivity: "base",
					});
				case "hosts_passed":
				case "hosts_failed":
				case "hosts_warned":
				case "total_hosts":
					va = a[sort_by] ?? 0;
					vb = b[sort_by] ?? 0;
					if (va !== vb) return asc ? va - vb : vb - va;
					return (a.title || "").localeCompare(b.title || "", undefined, {
						sensitivity: "base",
					});
				default:
					return 0;
			}
		});

		const total = rules_with_counts.length;
		const paginated = rules_with_counts.slice(offset, offset + limit);

		res.json({
			rules: paginated,
			pagination: { total, limit, offset },
		});
	} catch (error) {
		logger.error("[Compliance] Error fetching rules:", error);
		res.status(500).json({ error: "Failed to fetch rules" });
	}
});

/**
 * GET /api/v1/compliance/rules/:ruleId
 * Get detailed rule information plus affected hosts with their latest result status.
 */
router.get("/rules/:ruleId", async (req, res) => {
	try {
		const { ruleId } = req.params;
		if (!isValidUUID(ruleId)) {
			return res.status(400).json({ error: "Invalid rule ID format" });
		}

		const rule = await prisma.compliance_rules.findUnique({
			where: { id: ruleId },
			include: {
				compliance_profiles: { select: { id: true, type: true, name: true } },
			},
		});

		if (!rule) {
			return res.status(404).json({ error: "Rule not found" });
		}

		// Get latest completed scan per host for this rule's profile
		const latest_scans_list = await getLatestCompletedScans(prisma, {
			profile_id: rule.profile_id,
			select: { id: true, host_id: true, completed_at: true },
		});
		const latest_scan_ids = latest_scans_list.map((s) => s.id);
		const _scan_map = new Map(latest_scans_list.map((s) => [s.id, s]));

		// Get results for this rule from those latest scans (include remediation for fallback)
		const results = await prisma.compliance_results.findMany({
			where: {
				rule_id: ruleId,
				scan_id: { in: latest_scan_ids },
			},
			select: {
				status: true,
				finding: true,
				actual: true,
				expected: true,
				remediation: true,
				scan_id: true,
				compliance_scans: {
					select: {
						host_id: true,
						completed_at: true,
						hosts: {
							select: {
								id: true,
								hostname: true,
								friendly_name: true,
								ip: true,
							},
						},
					},
				},
			},
		});

		const affected_hosts = results.map((r) => ({
			host_id: r.compliance_scans.host_id,
			hostname: r.compliance_scans.hosts.hostname,
			friendly_name: r.compliance_scans.hosts.friendly_name,
			ip: r.compliance_scans.hosts.ip,
			status: r.status,
			finding: r.finding,
			actual: r.actual,
			expected: r.expected,
			scan_date: r.compliance_scans.completed_at,
		}));

		// Sort: failures first, then warnings, then passes
		const status_order = {
			fail: 0,
			warn: 1,
			error: 2,
			skip: 3,
			notapplicable: 4,
			pass: 5,
		};
		affected_hosts.sort(
			(a, b) => (status_order[a.status] ?? 99) - (status_order[b.status] ?? 99),
		);

		// When rule has no rationale, derive "why this failed" from first fail/warn host that has content
		// (affected_hosts already sorted: fail first, then warn, then pass)
		const fail_warn_hosts = affected_hosts.filter(
			(h) => h.status === "fail" || h.status === "warn" || h.status === "error",
		);
		let rationale_display = rule.rationale?.trim() || null;
		if (!rationale_display && fail_warn_hosts.length > 0) {
			const with_content = fail_warn_hosts.find(
				(h) => h.finding?.trim() || h.actual?.trim() || h.expected?.trim(),
			);
			const src = with_content || fail_warn_hosts[0];
			if (src.finding?.trim()) {
				rationale_display = src.finding.trim();
			} else if (src.actual?.trim() || src.expected?.trim()) {
				const parts = [];
				if (src.actual?.trim()) parts.push(`Actual: ${src.actual.trim()}`);
				if (src.expected?.trim())
					parts.push(`Expected: ${src.expected.trim()}`);
				rationale_display = parts.join("\n");
			} else {
				// No finding/actual/expected on any fail/warn result: use rule description or title
				rationale_display =
					rule.description?.trim() ||
					rule.title ||
					"This check did not meet the benchmark requirement on one or more hosts. See the table below for per-host status.";
			}
		}

		// When rule has no remediation, use first result that has remediation (same as host page)
		let remediation_display = rule.remediation?.trim() || null;
		if (!remediation_display && results.length > 0) {
			const with_remediation = results.find((r) => r.remediation?.trim());
			if (with_remediation?.remediation?.trim()) {
				remediation_display = with_remediation.remediation.trim();
			}
		}

		res.json({
			rule: {
				id: rule.id,
				rule_ref: rule.rule_ref,
				title: rule.title,
				description: rule.description,
				rationale: rationale_display ?? rule.rationale,
				severity: rule.severity,
				section: rule.section,
				remediation: remediation_display ?? rule.remediation,
				profile_id: rule.profile_id,
				profile_type: rule.compliance_profiles?.type,
				profile_name: rule.compliance_profiles?.name,
			},
			affected_hosts,
		});
	} catch (error) {
		logger.error("[Compliance] Error fetching rule detail:", error);
		res.status(500).json({ error: "Failed to fetch rule detail" });
	}
});

module.exports = router;
