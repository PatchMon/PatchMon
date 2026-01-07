const express = require("express");
const logger = require("../utils/logger");
const router = express.Router();
const rateLimit = require("express-rate-limit");
const { getPrismaClient } = require("../config/prisma");
const { Prisma } = require("@prisma/client");
const { authenticateToken } = require("../middleware/auth");
const { v4: uuidv4, validate: uuidValidate } = require("uuid");
const { verifyApiKey } = require("../utils/apiKeyUtils");

const prisma = getPrismaClient();

// Rate limiter for scan submissions (per agent)
const scanSubmitLimiter = rateLimit({
  windowMs: 60 * 1000, // 1 minute
  max: 10, // 10 scans per minute per agent
  keyGenerator: (req) => req.headers["x-api-id"] || req.ip,
  message: { error: "Too many scan submissions, please try again later" },
  standardHeaders: true,
  legacyHeaders: false,
});

// ==========================================
// Input Validation Helpers
// ==========================================

const VALID_RESULT_STATUSES = ["pass", "fail", "warn", "skip", "notapplicable", "error"];
const VALID_SEVERITIES = ["low", "medium", "high", "critical"];
const VALID_PROFILE_TYPES = ["openscap", "docker-bench", "oscap-docker", "all"];

function isValidUUID(id) {
  return id && uuidValidate(id);
}

function sanitizeInt(value, defaultVal, min = 1, max = 1000) {
  const parsed = parseInt(value, 10);
  if (isNaN(parsed)) return defaultVal;
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
      logger.info(`[Compliance] Received ${scansToProcess.length} scans from agent payload`);
    } else if (req.body.profile_name) {
      // Legacy flat format - wrap in array
      scansToProcess = [{
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
      }];
    } else {
      return res.status(400).json({ error: "Invalid payload: expected 'scans' array or 'profile_name'" });
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
        error: scanError
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
        passed: scanData.passed ?? results?.filter((r) => r.status === "pass").length ?? 0,
        failed: scanData.failed ?? results?.filter((r) => r.status === "fail").length ?? 0,
        warnings: scanData.warnings ?? results?.filter((r) => r.status === "warn").length ?? 0,
        skipped: scanData.skipped ?? results?.filter((r) => r.status === "skip").length ?? 0,
        not_applicable: scanData.not_applicable ?? results?.filter((r) => r.status === "notapplicable").length ?? 0,
      };

      // Use score from agent if provided, otherwise calculate
      let score = scanData.score;
      if (score === undefined || score === null) {
        const applicableRules = stats.total_rules - stats.not_applicable - stats.skipped;
        score = applicableRules > 0
          ? ((stats.passed / applicableRules) * 100).toFixed(2)
          : null;
      }

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
          receivedStatusCounts[r.status] = (receivedStatusCounts[r.status] || 0) + 1;
        }
        console.log(`=== DEBUG: Received ${results.length} results for ${profile_type} ===`);
        console.log(`=== DEBUG: Status counts received: ${JSON.stringify(receivedStatusCounts)} ===`);

        // Deduplicate results by rule_ref, prioritizing important statuses
        // Priority: fail > warn > pass > skip > notapplicable > error
        const statusPriority = { fail: 6, warn: 5, pass: 4, skip: 3, notapplicable: 2, error: 1 };
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
        console.log(`=== DEBUG: Deduplicated ${results.length} results to ${uniqueResults.length} unique results ===`);

        // Count deduplicated statuses
        const deduplicatedStatusCounts = {};
        for (const r of uniqueResults) {
          deduplicatedStatusCounts[r.status] = (deduplicatedStatusCounts[r.status] || 0) + 1;
        }
        console.log(`=== DEBUG: Deduplicated status counts: ${JSON.stringify(deduplicatedStatusCounts)} ===`);

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
            if (result.title && result.title !== ruleRef &&
                (!rule.title || rule.title === ruleRef || rule.title === "Unknown" ||
                 rule.title.toLowerCase().replace(/[_\s]+/g, ' ') === ruleRef.replace(/xccdf_org\.ssgproject\.content_rule_/i, '').replace(/_/g, ' '))) {
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
          // Debug: log warn status results being stored
          if (result.status === 'warn') {
            console.log(`=== DEBUG: Storing WARN result: scan_id=${scan.id}, rule_ref=${ruleRef}, status=${result.status} ===`);
          }
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

      // Debug: verify what was stored in the scan
      const storedCounts = await prisma.$queryRaw`
        SELECT status, COUNT(*)::int as count
        FROM compliance_results
        WHERE scan_id = ${scan.id}
        GROUP BY status
      `;
      console.log(`=== DEBUG: Stored results for scan ${scan.id}: ${JSON.stringify(storedCounts)} ===`);

      logger.info(`[Compliance] Scan saved for host ${host.friendly_name || host.hostname} (${profile_name}): ${stats.passed}/${stats.total_rules} passed (${score}%)`);
      processedScans.push({ scan_id: scan.id, profile_name, score: scan.score, stats });
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
router.get("/profiles", async (req, res) => {
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
router.get("/dashboard", async (req, res) => {
  try {
    console.log("=== DASHBOARD ENDPOINT HIT ===");
    // Get latest scan per host per profile using raw query for PostgreSQL
    // This ensures we get both OpenSCAP and Docker Bench scans for each host
    const latestScans = await prisma.$queryRaw`
      SELECT DISTINCT ON (host_id, profile_id) *
      FROM compliance_scans
      WHERE status = 'completed'
      ORDER BY host_id, profile_id, completed_at DESC
    `;

    // Calculate averages - use unique hosts, not scan count
    const uniqueHostIds = [...new Set(latestScans.map(s => s.host_id))];
    const totalHosts = uniqueHostIds.length;
    const avgScore = latestScans.length > 0
      ? latestScans.reduce((sum, s) => sum + (Number(s.score) || 0), 0) / latestScans.length
      : 0;

    // Get SCAN counts by compliance level (these are scan-level, not host-level)
    const scans_compliant = latestScans.filter((s) => Number(s.score) >= 80).length;
    const scans_warning = latestScans.filter((s) => Number(s.score) >= 60 && Number(s.score) < 80).length;
    const scans_critical = latestScans.filter((s) => Number(s.score) < 60).length;
    const unscanned = await prisma.hosts.count({
      where: {
        compliance_scans: { none: {} },
      },
    });

    // Get HOST-LEVEL compliance status (using worst score per host)
    // A host is compliant only if ALL its scans are >=80%
    // A host is critical if ANY of its scans are <60%
    // A host is warning if its worst score is 60-80%
    // Also track which scan type caused the worst score

    // First, get profile types for each scan
    const profileTypes = {};
    const profileTypeQuery = await prisma.compliance_profiles.findMany({
      select: { id: true, type: true }
    });
    for (const p of profileTypeQuery) {
      profileTypes[p.id] = p.type;
    }

    // Track worst score and which scan type caused it per host
    const hostWorstScores = new Map(); // host_id -> { score, scanType }
    for (const scan of latestScans) {
      const scanScore = Number(scan.score) || 0;
      const scanType = profileTypes[scan.profile_id] || 'unknown';
      const current = hostWorstScores.get(scan.host_id);
      if (!current || scanScore < current.score) {
        hostWorstScores.set(scan.host_id, { score: scanScore, scanType });
      }
    }

    // Count hosts by status
    const hosts_compliant = [...hostWorstScores.values()].filter(h => h.score >= 80).length;
    const hosts_warning = [...hostWorstScores.values()].filter(h => h.score >= 60 && h.score < 80).length;
    const hosts_critical = [...hostWorstScores.values()].filter(h => h.score < 60).length;

    // Count hosts by status AND scan type that caused it
    const hostStatusByScanType = {
      compliant: { openscap: 0, "docker-bench": 0 },
      warning: { openscap: 0, "docker-bench": 0 },
      critical: { openscap: 0, "docker-bench": 0 },
    };
    for (const h of hostWorstScores.values()) {
      let status = 'compliant';
      if (h.score < 60) status = 'critical';
      else if (h.score < 80) status = 'warning';

      if (hostStatusByScanType[status][h.scanType] !== undefined) {
        hostStatusByScanType[status][h.scanType]++;
      }
    }

    // For backwards compatibility, keep the old field names as scan counts
    const compliant = scans_compliant;
    const warning = scans_warning;
    const critical = scans_critical;

    // Recent scans
    const recentScans = await prisma.compliance_scans.findMany({
      take: 10,
      orderBy: { completed_at: "desc" },
      include: {
        hosts: { select: { id: true, hostname: true, friendly_name: true } },
        compliance_profiles: { select: { name: true, type: true } },
      },
    });

    // Worst performing hosts (latest scan per host per profile, sorted by score)
    // Include both OpenSCAP and Docker Bench scans for each host
    const worstHostsRaw = await prisma.$queryRaw`
      SELECT DISTINCT ON (host_id, profile_id) cs.*, h.hostname, h.friendly_name, cp.name as profile_name, cp.type as profile_type
      FROM compliance_scans cs
      JOIN hosts h ON cs.host_id = h.id
      JOIN compliance_profiles cp ON cs.profile_id = cp.id
      WHERE cs.status = 'completed'
      ORDER BY cs.host_id, cs.profile_id, cs.completed_at DESC
    `;

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
    const totalPassedRules = latestScans.reduce((sum, s) => sum + (Number(s.passed) || 0), 0);
    const totalFailedRules = latestScans.reduce((sum, s) => sum + (Number(s.failed) || 0), 0);
    const totalRules = latestScans.reduce((sum, s) => sum + (Number(s.total_rules) || 0), 0);

    // Get top failing rules across all hosts (most recent scan per host) - for OpenSCAP
    const latestScanIds = latestScans.map(s => s.id);
    console.log(`=== latestScans: ${latestScans.length}, latestScanIds: ${latestScanIds.length} ===`);

    // Debug: Show which docker-bench scans are included
    const dockerBenchScans = latestScans.filter(s => {
      // Check profile_id to determine type - we need to look this up
      return true; // Will show all for now
    });
    console.log(`=== latestScanIds for aggregation: ${JSON.stringify(latestScanIds.slice(0, 5))}... ===`);
    let topFailingRules = [];
    if (latestScanIds.length > 0) {
      // Use Prisma.join for proper array handling in raw SQL
      // Include profile_type for filtering
      topFailingRules = await prisma.$queryRaw`
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
      `;
    }

    // Get top warning rules across all hosts - for Docker Bench (uses 'warn' status)
    let topWarningRules = [];
    if (latestScanIds.length > 0) {
      // First check what statuses exist for these scans
      const statusCheck = await prisma.$queryRaw`
        SELECT DISTINCT cr.status, cp.type as profile_type, COUNT(*)::int as count
        FROM compliance_results cr
        JOIN compliance_scans cs ON cr.scan_id = cs.id
        JOIN compliance_profiles cp ON cs.profile_id = cp.id
        WHERE cr.scan_id IN (${Prisma.join(latestScanIds)})
        GROUP BY cr.status, cp.type
        ORDER BY cp.type, cr.status
      `;
      console.log(`=== Status values: ${JSON.stringify(statusCheck)} ===`);

      topWarningRules = await prisma.$queryRaw`
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
      `;
      console.log(`=== Top warning rules: ${topWarningRules.length} ===`);
    }

    // Get profile distribution (how many hosts use each profile)
    const profileDistribution = await prisma.$queryRaw`
      SELECT
        cp.name as profile_name,
        cp.type as profile_type,
        COUNT(DISTINCT cs.host_id) as host_count
      FROM compliance_scans cs
      JOIN compliance_profiles cp ON cs.profile_id = cp.id
      WHERE cs.status = 'completed'
      GROUP BY cp.id, cp.name, cp.type
      ORDER BY host_count DESC
    `;

    // Get severity breakdown from latest scans (with profile type for breakdown)
    // Include both 'fail' (OpenSCAP) and 'warn' (Docker Bench) statuses
    let severityBreakdown = [];
    let severityByProfileType = [];
    if (latestScanIds.length > 0) {
      // Overall severity breakdown - include fail AND warn
      severityBreakdown = await prisma.$queryRaw`
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
      `;

      // Severity breakdown by profile type (for stacked chart)
      // Include fail AND warn to capture both OpenSCAP failures and Docker Bench warnings
      severityByProfileType = await prisma.$queryRaw`
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
      `;
    }

    // Docker Bench issues by section (since Docker Bench doesn't have severity)
    let dockerBenchBySection = [];
    if (latestScanIds.length > 0) {
      dockerBenchBySection = await prisma.$queryRaw`
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
      `;
    }

    // Get aggregate stats by profile type (openscap vs docker-bench)
    const profileTypeStats = await prisma.$queryRaw`
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
    `;

    // Calculate scan age distribution (how fresh is the compliance data)
    // Track by profile type (OpenSCAP vs Docker Bench)
    const now = new Date();
    const oneDayAgo = new Date(now - 24 * 60 * 60 * 1000);
    const oneWeekAgo = new Date(now - 7 * 24 * 60 * 60 * 1000);
    const oneMonthAgo = new Date(now - 30 * 24 * 60 * 60 * 1000);

    const scanAgeDistribution = {
      today: { openscap: 0, "docker-bench": 0 },
      this_week: { openscap: 0, "docker-bench": 0 },
      this_month: { openscap: 0, "docker-bench": 0 },
      older: { openscap: 0, "docker-bench": 0 },
    };

    // Get the most recent scan per host per profile type
    const hostLastScansByType = new Map(); // key: `${host_id}:${profile_type}`
    for (const scan of latestScans) {
      const profileType = profileTypes[scan.profile_id] || 'unknown';
      const key = `${scan.host_id}:${profileType}`;
      const existing = hostLastScansByType.get(key);
      if (!existing || new Date(scan.completed_at) > new Date(existing.completed_at)) {
        hostLastScansByType.set(key, { ...scan, profileType });
      }
    }

    for (const scan of hostLastScansByType.values()) {
      const scanDate = new Date(scan.completed_at);
      const type = scan.profileType;
      if (type !== 'openscap' && type !== 'docker-bench') continue;

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

    res.json({
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
      worst_hosts: worstHosts,
      top_failing_rules: topFailingRules.map(r => ({
        rule_id: r.rule_id,
        title: r.title,
        severity: r.severity,
        profile_type: r.profile_type,
        fail_count: Number(r.fail_count),
      })),
      top_warning_rules: topWarningRules.map(r => ({
        rule_id: r.rule_id,
        title: r.title,
        severity: r.severity,
        profile_type: r.profile_type,
        warn_count: Number(r.warn_count),
      })),
      profile_distribution: profileDistribution.map(p => ({
        name: p.profile_name,
        type: p.profile_type,
        host_count: Number(p.host_count),
      })),
      severity_breakdown: severityBreakdown.map(s => ({
        severity: s.severity || 'unknown',
        count: Number(s.count),
      })),
      severity_by_profile_type: severityByProfileType.map(s => ({
        severity: s.severity || 'unknown',
        profile_type: s.profile_type,
        count: Number(s.count),
      })),
      docker_bench_by_section: dockerBenchBySection.map(s => ({
        section: s.section || 'Unknown',
        count: Number(s.count),
      })),
      scan_age_distribution: scanAgeDistribution,
      profile_type_stats: profileTypeStats.map(p => ({
        type: p.profile_type,
        hosts_scanned: Number(p.hosts_scanned),
        average_score: p.average_score ? Math.round(Number(p.average_score) * 100) / 100 : null,
        total_passed: Number(p.total_passed) || 0,
        total_failed: Number(p.total_failed) || 0,
        total_warnings: Number(p.total_warnings) || 0,
        total_rules: Number(p.total_rules) || 0,
      })),
    });
  } catch (error) {
    logger.error("[Compliance] Error fetching dashboard:", error);
    res.status(500).json({ error: "Failed to fetch dashboard data" });
  }
});

/**
 * GET /api/v1/compliance/scans/active
 * Get all currently running compliance scans
 */
router.get("/scans/active", async (req, res) => {
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
        results.openscap.severity_breakdown = severityBreakdown.map(s => ({
          severity: s.severity || 'unknown',
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
        results["docker-bench"].section_breakdown = sectionBreakdown.map(s => ({
          section: s.section || 'Unknown',
          count: Number(s.count),
        }));
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
 * Get detailed results for a specific scan
 */
router.get("/results/:scanId", async (req, res) => {
  try {
    const { scanId } = req.params;
    const { status, severity } = req.query;

    // Validate scanId
    if (!isValidUUID(scanId)) {
      return res.status(400).json({ error: "Invalid scan ID format" });
    }

    // Validate status filter if provided
    if (status && !VALID_RESULT_STATUSES.includes(status)) {
      return res.status(400).json({ error: `Invalid status. Must be one of: ${VALID_RESULT_STATUSES.join(", ")}` });
    }

    // Validate severity filter if provided
    if (severity && !VALID_SEVERITIES.includes(severity)) {
      return res.status(400).json({ error: `Invalid severity. Must be one of: ${VALID_SEVERITIES.join(", ")}` });
    }

    const where = { scan_id: scanId };
    if (status) where.status = status;

    const results = await prisma.compliance_results.findMany({
      where,
      include: {
        compliance_rules: true,
      },
      orderBy: [
        { status: "asc" },
      ],
    });

    // Filter by severity if specified
    const filteredResults = severity
      ? results.filter((r) => r.compliance_rules.severity === severity)
      : results;

    res.json(filteredResults);
  } catch (error) {
    logger.error("[Compliance] Error fetching results:", error);
    res.status(500).json({ error: "Failed to fetch results" });
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
      return res.status(400).json({ error: `Invalid profile_type. Must be one of: ${VALID_PROFILE_TYPES.join(", ")}` });
    }

    const host = await prisma.hosts.findUnique({
      where: { id: hostId },
    });

    if (!host) {
      return res.status(404).json({ error: "Host not found" });
    }

    // Use agentWs service to send compliance scan trigger
    const agentWs = require("../services/agentWs");

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
      const success = agentWs.pushDockerImageScan(host.api_id, dockerScanOptions);
      if (success) {
        return res.json({
          message: "Docker image CVE scan triggered",
          host_id: hostId,
          profile_type,
          scan_all_images: Boolean(scan_all_images),
          image_name: image_name || null,
        });
      } else {
        return res.status(400).json({ error: "Failed to send Docker image scan trigger" });
      }
    }

    // Use the dedicated pushComplianceScan function with options
    const success = agentWs.pushComplianceScan(host.api_id, profile_type, scanOptions);

    if (success) {
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
 * POST /api/v1/compliance/trigger/bulk
 * Trigger compliance scans on multiple hosts at once
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

    // Validate hostIds array
    if (!Array.isArray(hostIds) || hostIds.length === 0) {
      return res.status(400).json({ error: "hostIds must be a non-empty array" });
    }

    if (hostIds.length > 100) {
      return res.status(400).json({ error: "Maximum 100 hosts per bulk operation" });
    }

    // Validate all UUIDs
    const invalidIds = hostIds.filter((id) => !isValidUUID(id));
    if (invalidIds.length > 0) {
      return res.status(400).json({ error: `Invalid host IDs: ${invalidIds.join(", ")}` });
    }

    // Validate profile_type
    if (!VALID_PROFILE_TYPES.includes(profile_type)) {
      return res.status(400).json({ error: `Invalid profile_type. Must be one of: ${VALID_PROFILE_TYPES.join(", ")}` });
    }

    // Get all hosts
    const hosts = await prisma.hosts.findMany({
      where: { id: { in: hostIds } },
      select: { id: true, api_id: true, hostname: true, friendly_name: true },
    });

    const hostMap = new Map(hosts.map((h) => [h.id, h]));
    const agentWs = require("../services/agentWs");

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

      const success = agentWs.pushComplianceScan(host.api_id, profile_type, scanOptions);

      if (success) {
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
    const agentWs = require("../services/agentWs");

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
    const agentWs = require("../services/agentWs");

    if (!agentWs.isConnected(host.api_id)) {
      return res.status(400).json({ error: "Host is not connected" });
    }

    const success = agentWs.pushRemediateRule(host.api_id, rule_id);

    if (success) {
      logger.info(`[Compliance] Single rule remediation triggered for ${host.api_id}: ${rule_id}`);
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

module.exports = router;
