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
          score: score ? parseFloat(score) : null,
          error_message: scanError || null,
          raw_output: results ? JSON.stringify(results) : null,
        },
      });

      // Create rule and result records
      if (results && Array.isArray(results)) {
        for (const result of results) {
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

          // Create result
          await prisma.compliance_results.create({
            data: {
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
    // Get latest scan per host using raw query for PostgreSQL
    const latestScans = await prisma.$queryRaw`
      SELECT DISTINCT ON (host_id) *
      FROM compliance_scans
      WHERE status = 'completed'
      ORDER BY host_id, completed_at DESC
    `;

    // Calculate averages
    const totalHosts = latestScans.length;
    const avgScore = totalHosts > 0
      ? latestScans.reduce((sum, s) => sum + (Number(s.score) || 0), 0) / totalHosts
      : 0;

    // Get hosts by compliance level
    const compliant = latestScans.filter((s) => Number(s.score) >= 80).length;
    const warning = latestScans.filter((s) => Number(s.score) >= 60 && Number(s.score) < 80).length;
    const critical = latestScans.filter((s) => Number(s.score) < 60).length;
    const unscanned = await prisma.hosts.count({
      where: {
        compliance_scans: { none: {} },
      },
    });

    // Recent scans
    const recentScans = await prisma.compliance_scans.findMany({
      take: 10,
      orderBy: { completed_at: "desc" },
      include: {
        hosts: { select: { id: true, hostname: true, friendly_name: true } },
        compliance_profiles: { select: { name: true, type: true } },
      },
    });

    // Worst performing hosts (latest scan per host, sorted by score)
    const worstHostsRaw = await prisma.$queryRaw`
      SELECT DISTINCT ON (host_id) cs.*, h.hostname, h.friendly_name, cp.name as profile_name
      FROM compliance_scans cs
      JOIN hosts h ON cs.host_id = h.id
      JOIN compliance_profiles cp ON cs.profile_id = cp.id
      WHERE cs.status = 'completed'
      ORDER BY cs.host_id, cs.completed_at DESC
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
      }));

    // Transform recent_scans to match frontend expectations (host instead of hosts, profile instead of compliance_profiles)
    const transformedRecentScans = recentScans.map((scan) => ({
      ...scan,
      host: scan.hosts,
      profile: scan.compliance_profiles,
      hosts: undefined,
      compliance_profiles: undefined,
    }));

    // Get aggregate rule statistics from latest scans
    const totalPassedRules = latestScans.reduce((sum, s) => sum + (Number(s.passed) || 0), 0);
    const totalFailedRules = latestScans.reduce((sum, s) => sum + (Number(s.failed) || 0), 0);
    const totalRules = latestScans.reduce((sum, s) => sum + (Number(s.total_rules) || 0), 0);

    // Get top failing rules across all hosts (most recent scan per host)
    const latestScanIds = latestScans.map(s => s.id);
    let topFailingRules = [];
    if (latestScanIds.length > 0) {
      // Use Prisma.join for proper array handling in raw SQL
      topFailingRules = await prisma.$queryRaw`
        SELECT
          cr.rule_id,
          cru.title,
          cru.severity,
          COUNT(*) as fail_count
        FROM compliance_results cr
        JOIN compliance_rules cru ON cr.rule_id = cru.id
        WHERE cr.scan_id IN (${Prisma.join(latestScanIds)})
          AND cr.status = 'fail'
        GROUP BY cr.rule_id, cru.title, cru.severity
        ORDER BY fail_count DESC
        LIMIT 10
      `;
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

    // Get severity breakdown from latest scans
    let severityBreakdown = [];
    if (latestScanIds.length > 0) {
      severityBreakdown = await prisma.$queryRaw`
        SELECT
          cru.severity,
          COUNT(*) as count
        FROM compliance_results cr
        JOIN compliance_rules cru ON cr.rule_id = cru.id
        WHERE cr.scan_id IN (${Prisma.join(latestScanIds)})
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
    }

    res.json({
      summary: {
        total_hosts: totalHosts,
        average_score: Math.round(avgScore * 100) / 100,
        compliant,
        warning,
        critical,
        unscanned,
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
        fail_count: Number(r.fail_count),
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
    });
  } catch (error) {
    logger.error("[Compliance] Error fetching dashboard:", error);
    res.status(500).json({ error: "Failed to fetch dashboard data" });
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
 */
router.get("/scans/:hostId/latest", async (req, res) => {
  try {
    const { hostId } = req.params;

    // Validate hostId
    if (!isValidUUID(hostId)) {
      return res.status(400).json({ error: "Invalid host ID format" });
    }

    const scan = await prisma.compliance_scans.findFirst({
      where: { host_id: hostId, status: "completed" },
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
