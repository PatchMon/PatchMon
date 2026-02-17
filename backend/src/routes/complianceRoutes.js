const express = require("express");
const logger = require("../utils/logger");
const router = express.Router();
const rateLimit = require("express-rate-limit");
const { getPrismaClient } = require("../config/prisma");
const { Prisma } = require("@prisma/client");
const { authenticateToken } = require("../middleware/auth");
const { v4: uuidv4, validate: uuidValidate } = require("uuid");
const { verifyApiKey } = require("../utils/apiKeyUtils");
const agentWs = require("../services/agentWs");

const prisma = getPrismaClient();

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

		// Validate host credentials
		const host = await prisma.hosts.findFirst({
			where: { api_id: apiId },
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
			if (results && Array.isArray(results)) {
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

				for (const result of uniqueResults) {
					// Get rule_ref from various possible field names
					const ruleRef = result.rule_ref || result.rule_id || result.id;
					if (!ruleRef) continue;

					// Upsert rule - always update if we have better metadata
					let rule = await prisma.compliance_rules.findFirst({
						where: {
							profile_id: profile.id,
							rule_ref: ruleRef,
						},
					});

					if (!rule) {
						rule = await prisma.compliance_rules.create({
							data: {
								id: uuidv4(),
								profile_id: profile.id,
								rule_ref: ruleRef,
								title: result.title || ruleRef || "Unknown",
								description: result.description || null,
								severity: result.severity || null,
								section: result.section || null,
								remediation: result.remediation || null,
							},
						});
					} else {
						// Update existing rule if we have new/better metadata from agent
						// Only update fields that have values and are currently missing or generic
						const updateData = {};

						// Update title if agent provides one and current is missing/generic
						if (
							result.title &&
							result.title !== ruleRef &&
							(!rule.title ||
								rule.title === ruleRef ||
								rule.title === "Unknown" ||
								rule.title.toLowerCase().replace(/[_\s]+/g, " ") ===
									ruleRef
										.replace(/xccdf_org\.ssgproject\.content_rule_/i, "")
										.replace(/_/g, " "))
						) {
							updateData.title = result.title;
						}

						// Update description if agent provides one and current is missing
						if (result.description && !rule.description) {
							updateData.description = result.description;
						}

						// Update severity if agent provides one and current is missing
						if (result.severity && !rule.severity) {
							updateData.severity = result.severity;
						}

						// Update section if agent provides one and current is missing
						if (result.section && !rule.section) {
							updateData.section = result.section;
						}

						// Update remediation if agent provides one and current is missing
						if (result.remediation && !rule.remediation) {
							updateData.remediation = result.remediation;
						}

						// Only run update if we have changes
						if (Object.keys(updateData).length > 0) {
							rule = await prisma.compliance_rules.update({
								where: { id: rule.id },
								data: updateData,
							});
						}
					}

					// Create or update result (upsert to handle duplicate rules in same scan)
					await prisma.compliance_results.upsert({
						where: {
							scan_id_rule_id: {
								scan_id: scan.id,
								rule_id: rule.id,
							},
						},
						update: {
							status: result.status,
							finding: result.finding || result.message || null,
							actual: result.actual || null,
							expected: result.expected || null,
							remediation: result.remediation || null,
						},
						create: {
							id: uuidv4(),
							scan_id: scan.id,
							rule_id: rule.id,
							status: result.status,
							finding: result.finding || result.message || null,
							actual: result.actual || null,
							expected: result.expected || null,
							remediation: result.remediation || null,
						},
					});
				}
			}

			logger.info(
				`[Compliance] Scan saved for host ${host.friendly_name || host.hostname} (${profile_name}): ${stats.passed}/${stats.total_rules} passed (${score}%)`,
			);
			processedScans.push({
				scan_id: scan.id,
				profile_name,
				score: scan.score,
				stats,
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

		// Run initial queries in parallel
		const [latestScansRows, profileTypeQuery, unscanned, allHostsRows] =
			await Promise.all([
				prisma.$queryRaw`
        SELECT DISTINCT ON (host_id, profile_id) *
        FROM compliance_scans
        WHERE status = 'completed'
        ORDER BY host_id, profile_id, completed_at DESC
      `,
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

		const latestScans = latestScansRows;

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

		// For backwards compatibility, keep the old field names as scan counts
		const compliant = scans_compliant;
		const warning = scans_warning;
		const critical = scans_critical;

		// Parallel fetch: recent scans, worst hosts, profile distribution, profile type stats
		const [recentScans, worstHostsRaw, profileDistribution, profileTypeStats] =
			await Promise.all([
				prisma.compliance_scans.findMany({
					take: 10,
					orderBy: { completed_at: "desc" },
					include: {
						hosts: {
							select: { id: true, hostname: true, friendly_name: true },
						},
						compliance_profiles: { select: { name: true, type: true } },
					},
				}),
				prisma.$queryRaw`
          SELECT DISTINCT ON (host_id, profile_id) cs.*, h.hostname, h.friendly_name, cp.name as profile_name, cp.type as profile_type
          FROM compliance_scans cs
          JOIN hosts h ON cs.host_id = h.id
          JOIN compliance_profiles cp ON cs.profile_id = cp.id
          WHERE cs.status = 'completed'
          ORDER BY cs.host_id, cs.profile_id, cs.completed_at DESC
        `,
				prisma.$queryRaw`
          SELECT
            cp.name as profile_name,
            cp.type as profile_type,
            COUNT(DISTINCT cs.host_id) as host_count
          FROM compliance_scans cs
          JOIN compliance_profiles cp ON cs.profile_id = cp.id
          WHERE cs.status = 'completed'
          GROUP BY cp.id, cp.name, cp.type
          ORDER BY host_count DESC
        `,
				prisma.$queryRaw`
          SELECT
            cp.type as profile_type,
            COUNT(DISTINCT cs.host_id) as hosts_scanned,
            AVG(cs.score) as average_score,
            SUM(cs.passed) as total_passed,
            SUM(cs.failed) as total_failed,
            SUM(cs.warnings) as total_warnings,
            SUM(cs.total_rules) as total_rules
          FROM (
            SELECT DISTINCT ON (host_id, profile_id) *
            FROM compliance_scans
            WHERE status = 'completed'
            ORDER BY host_id, profile_id, completed_at DESC
          ) cs
          JOIN compliance_profiles cp ON cs.profile_id = cp.id
          GROUP BY cp.type
        `,
			]);

		// Sort by score ascending and take top 5
		const worstHosts = worstHostsRaw
			.sort((a, b) => (Number(a.score) || 0) - (Number(b.score) || 0))
			.slice(0, 5)
			.map((h) => ({
				id: h.id,
				host_id: h.host_id,
				score: h.score,
				completed_at: h.completed_at,
				host: {
					id: h.host_id,
					hostname: h.hostname,
					friendly_name: h.friendly_name,
				},
				profile: {
					name: h.profile_name,
				},
				// Include compliance_profiles for frontend filtering
				compliance_profiles: {
					type: h.profile_type,
					name: h.profile_name,
				},
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
			const [
				topFailingRows,
				topWarningRows,
				severityRows,
				severityByTypeRows,
				dockerSectionRows,
			] = await Promise.all([
				prisma.$queryRaw`
          SELECT
            cr.rule_id,
            cru.title,
            cru.severity,
            cp.type as profile_type,
            COUNT(*) as fail_count
          FROM compliance_results cr
          JOIN compliance_rules cru ON cr.rule_id = cru.id
          JOIN compliance_scans cs ON cr.scan_id = cs.id
          JOIN compliance_profiles cp ON cs.profile_id = cp.id
          WHERE cr.scan_id IN (${Prisma.join(latestScanIds)})
            AND cr.status = 'fail'
          GROUP BY cr.rule_id, cru.title, cru.severity, cp.type
          ORDER BY fail_count DESC
          LIMIT 10
        `,
				prisma.$queryRaw`
          SELECT
            cr.rule_id,
            cru.title,
            cru.severity,
            cp.type as profile_type,
            COUNT(*) as warn_count
          FROM compliance_results cr
          JOIN compliance_rules cru ON cr.rule_id = cru.id
          JOIN compliance_scans cs ON cr.scan_id = cs.id
          JOIN compliance_profiles cp ON cs.profile_id = cp.id
          WHERE cr.scan_id IN (${Prisma.join(latestScanIds)})
            AND cr.status = 'warn'
          GROUP BY cr.rule_id, cru.title, cru.severity, cp.type
          ORDER BY warn_count DESC
          LIMIT 10
        `,
				prisma.$queryRaw`
          SELECT
            cru.severity,
            COUNT(*) as count
          FROM compliance_results cr
          JOIN compliance_rules cru ON cr.rule_id = cru.id
          WHERE cr.scan_id IN (${Prisma.join(latestScanIds)})
            AND cr.status IN ('fail', 'warn')
          GROUP BY cru.severity
          ORDER BY
            CASE cru.severity
              WHEN 'critical' THEN 1
              WHEN 'high' THEN 2
              WHEN 'medium' THEN 3
              WHEN 'low' THEN 4
              ELSE 5
            END
        `,
				prisma.$queryRaw`
          SELECT
            cru.severity,
            cp.type as profile_type,
            COUNT(*) as count
          FROM compliance_results cr
          JOIN compliance_rules cru ON cr.rule_id = cru.id
          JOIN compliance_scans cs ON cr.scan_id = cs.id
          JOIN compliance_profiles cp ON cs.profile_id = cp.id
          WHERE cr.scan_id IN (${Prisma.join(latestScanIds)})
            AND cr.status IN ('fail', 'warn')
          GROUP BY cru.severity, cp.type
          ORDER BY
            CASE cru.severity
              WHEN 'critical' THEN 1
              WHEN 'high' THEN 2
              WHEN 'medium' THEN 3
              WHEN 'low' THEN 4
              ELSE 5
            END
        `,
				prisma.$queryRaw`
          SELECT
            cru.section,
            COUNT(*) as count
          FROM compliance_results cr
            JOIN compliance_rules cru ON cr.rule_id = cru.id
            JOIN compliance_scans cs ON cr.scan_id = cs.id
            JOIN compliance_profiles cp ON cs.profile_id = cp.id
          WHERE cr.scan_id IN (${Prisma.join(latestScanIds)})
            AND cr.status = 'warn'
            AND cp.type = 'docker-bench'
          GROUP BY cru.section
          ORDER BY cru.section
        `,
			]);
			topFailingRules = topFailingRows;
			topWarningRules = topWarningRows;
			severityBreakdown = severityRows;
			severityByProfileType = severityByTypeRows;
			dockerBenchBySection = dockerSectionRows;
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
			// Severity breakdown for OpenSCAP failures
			if (results.openscap) {
				const severityBreakdown = await prisma.$queryRaw`
          SELECT cru.severity, COUNT(*) as count
          FROM compliance_results cr
          JOIN compliance_rules cru ON cr.rule_id = cru.id
          WHERE cr.scan_id = ${results.openscap.id}
            AND cr.status = 'fail'
          GROUP BY cru.severity
          ORDER BY
            CASE cru.severity
              WHEN 'critical' THEN 1
              WHEN 'high' THEN 2
              WHEN 'medium' THEN 3
              WHEN 'low' THEN 4
              ELSE 5
            END
        `;
				results.openscap.severity_breakdown = severityBreakdown.map((s) => ({
					severity: s.severity || "unknown",
					count: Number(s.count),
				}));
			}

			// Section breakdown for Docker Bench warnings
			if (results["docker-bench"]) {
				const sectionBreakdown = await prisma.$queryRaw`
          SELECT cru.section, COUNT(*) as count
          FROM compliance_results cr
          JOIN compliance_rules cru ON cr.rule_id = cru.id
          WHERE cr.scan_id = ${results["docker-bench"].id}
            AND cr.status = 'warn'
          GROUP BY cru.section
          ORDER BY cru.section
        `;
				results["docker-bench"].section_breakdown = sectionBreakdown.map(
					(s) => ({
						section: s.section || "Unknown",
						count: Number(s.count),
					}),
				);
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
			if (status === "skipped") {
				where.status = { in: ["skip", "notapplicable"] };
			} else {
				where.status = status;
			}
		}

		// Severity filter: apply via relation (required for count and findMany)
		if (severity) {
			where.compliance_rules = { severity };
		}

		const baseWhere = { scan_id: scanId };

		const [results, total, statusCounts, severityRows] = await Promise.all([
			prisma.compliance_results.findMany({
				where,
				include: {
					compliance_rules: true,
				},
				orderBy: [{ status: "asc" }],
				take: limit,
				skip: offset,
			}),
			prisma.compliance_results.count({ where }),
			offset === 0
				? prisma.compliance_results.groupBy({
						by: ["status"],
						where: baseWhere,
						_count: true,
					})
				: Promise.resolve([]),
			offset === 0
				? prisma.$queryRaw`
            SELECT cru.severity, COUNT(*)::int as count
            FROM compliance_results cr
            JOIN compliance_rules cru ON cr.rule_id = cru.id
            WHERE cr.scan_id = ${scanId} AND cr.status = 'fail'
            GROUP BY cru.severity
          `
				: Promise.resolve([]),
		]);

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
			select: { id: true, api_id: true, hostname: true, friendly_name: true },
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

			const success = agentWs.pushComplianceScan(
				host.api_id,
				profile_type,
				scanOptions,
			);

			if (success) {
				// Create "running" scan records for tracking
				for (const profile of profilesToUse) {
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

		// Use agentWs service to send compliance scan trigger
		if (!agentWs.isConnected(host.api_id)) {
			return res.status(400).json({ error: "Host is not connected" });
		}

		// Build scan options
		const scanOptions = {
			profileId: profile_id,
			enableRemediation: Boolean(enable_remediation),
			fetchRemoteResources: Boolean(fetch_remote_resources),
		};

		// Handle Docker image CVE scanning separately
		if (profile_type === "oscap-docker") {
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
			} else {
				return res
					.status(400)
					.json({ error: "Failed to send Docker image scan trigger" });
			}
		}

		// Use the dedicated pushComplianceScan function with options
		const success = agentWs.pushComplianceScan(
			host.api_id,
			profile_type,
			scanOptions,
		);

		if (success) {
			// Create "running" scan records for tracking
			const profilesToUse = [];
			if (profile_type === "all" || profile_type === "openscap") {
				let oscapProfile = await prisma.compliance_profiles.findFirst({
					where: { type: "openscap" },
					orderBy: { name: "asc" },
				});
				if (!oscapProfile) {
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
				`[Compliance] Creating ${profilesToUse.length} running scan records for host ${hostId}`,
			);

			for (const profile of profilesToUse) {
				try {
					const runningScan = await prisma.compliance_scans.create({
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
					logger.info(
						`[Compliance] Created running scan record: ${runningScan.id} for profile ${profile.name}`,
					);
				} catch (err) {
					logger.warn(
						`[Compliance] Could not create running scan record: ${err.message}`,
					);
				}
			}

			res.json({
				message: enable_remediation
					? "Compliance scan with remediation triggered"
					: "Compliance scan triggered",
				host_id: hostId,
				profile_type,
				enable_remediation,
			});
		} else {
			res.status(400).json({ error: "Failed to send scan trigger" });
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
		const latest_scans_raw = await prisma.$queryRaw`
			SELECT DISTINCT ON (host_id, profile_id) id
			FROM compliance_scans
			WHERE status = 'completed'
			ORDER BY host_id, profile_id, completed_at DESC
		`;
		const latest_scan_ids = latest_scans_raw.map((s) => s.id);

		if (latest_scan_ids.length === 0) {
			return res.json({ rules: [], pagination: { total: 0, limit, offset } });
		}

		// Restrict to rules that appear in scan results: only failing or compliant (passing) rules; exclude not discovered and warning-only
		if (status_filter) {
			// Filter by status: only rule IDs that have at least one result in that status
			const rule_ids_with_status = await prisma.$queryRaw`
				SELECT DISTINCT cr.rule_id as id
				FROM compliance_results cr
				WHERE cr.scan_id IN (${Prisma.join(latest_scan_ids)})
					AND cr.status = ${status_filter}
			`;
			const ids = rule_ids_with_status.map((r) => r.id);
			if (ids.length === 0) {
				return res.json({ rules: [], pagination: { total: 0, limit, offset } });
			}
			rule_where.id = { in: ids };
		} else {
			// No status filter: only show rules that are failing or fully compliant (discovered in scans, no warning-only)
			const [ids_with_fail, ids_with_warn, ids_with_pass] = await Promise.all([
				prisma.$queryRaw`
					SELECT DISTINCT cr.rule_id as id FROM compliance_results cr
					WHERE cr.scan_id IN (${Prisma.join(latest_scan_ids)}) AND cr.status = 'fail'
				`.then((rows) => rows.map((r) => r.id)),
				prisma.$queryRaw`
					SELECT DISTINCT cr.rule_id as id FROM compliance_results cr
					WHERE cr.scan_id IN (${Prisma.join(latest_scan_ids)}) AND cr.status = 'warn'
				`.then((rows) => rows.map((r) => r.id)),
				prisma.$queryRaw`
					SELECT DISTINCT cr.rule_id as id FROM compliance_results cr
					WHERE cr.scan_id IN (${Prisma.join(latest_scan_ids)}) AND cr.status = 'pass'
				`.then((rows) => rows.map((r) => r.id)),
			]);
			const fail_set = new Set(ids_with_fail);
			const warn_set = new Set(ids_with_warn);
			// Compliant = has pass, no fail, no warn
			const compliant_ids = ids_with_pass.filter(
				(id) => !fail_set.has(id) && !warn_set.has(id),
			);
			const ids = [...new Set([...ids_with_fail, ...compliant_ids])];
			if (ids.length === 0) {
				return res.json({ rules: [], pagination: { total: 0, limit, offset } });
			}
			rule_where.id = { in: ids };
		}

		// Count total matching rules (for pagination)
		const total = await prisma.compliance_rules.count({ where: rule_where });

		// Fetch rules with result aggregation
		const rules = await prisma.compliance_rules.findMany({
			where: rule_where,
			orderBy: [{ section: "asc" }, { title: "asc" }],
			skip: offset,
			take: limit,
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
		const rules_with_counts = rules.map((r) => {
			const results = r.compliance_results || [];
			const hosts_passed = results.filter((x) => x.status === "pass").length;
			const hosts_failed = results.filter((x) => x.status === "fail").length;
			const hosts_warned = results.filter((x) => x.status === "warn").length;
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

		res.json({
			rules: rules_with_counts,
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
		const latest_scans_raw = await prisma.$queryRaw`
			SELECT DISTINCT ON (host_id) id, host_id, completed_at
			FROM compliance_scans
			WHERE status = 'completed' AND profile_id = ${rule.profile_id}
			ORDER BY host_id, completed_at DESC
		`;

		const latest_scan_ids = latest_scans_raw.map((s) => s.id);
		const _scan_map = new Map(latest_scans_raw.map((s) => [s.id, s]));

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
