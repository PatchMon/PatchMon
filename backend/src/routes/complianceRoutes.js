const express = require("express");
const router = express.Router();
const { getPrismaClient } = require("../config/prisma");
const { authenticateToken } = require("../middleware/auth");
const { v4: uuidv4 } = require("uuid");
const bcrypt = require("bcryptjs");
const crypto = require("node:crypto");

const prisma = getPrismaClient();

/**
 * Verify API key against stored hash or plaintext (legacy support)
 */
async function verifyApiKey(providedKey, storedKey) {
  if (!providedKey || !storedKey) return false;
  if (storedKey.match(/^\$2[aby]\$/)) {
    return bcrypt.compare(providedKey, storedKey);
  }
  try {
    const providedBuffer = Buffer.from(providedKey, "utf8");
    const storedBuffer = Buffer.from(storedKey, "utf8");
    if (providedBuffer.length !== storedBuffer.length) return false;
    return crypto.timingSafeEqual(providedBuffer, storedBuffer);
  } catch {
    return false;
  }
}

// ==========================================
// Public endpoints (API key auth for agents)
// ==========================================

/**
 * POST /api/v1/compliance/scans
 * Submit scan results from agent
 * Auth: X-API-ID and X-API-KEY headers
 */
router.post("/scans", async (req, res) => {
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

    const { profile_name, profile_type, results, started_at, completed_at, raw_output } = req.body;

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

    // Calculate stats
    const stats = {
      total_rules: results?.length || 0,
      passed: results?.filter((r) => r.status === "pass").length || 0,
      failed: results?.filter((r) => r.status === "fail").length || 0,
      warnings: results?.filter((r) => r.status === "warn").length || 0,
      skipped: results?.filter((r) => r.status === "skip").length || 0,
      not_applicable: results?.filter((r) => r.status === "notapplicable").length || 0,
    };

    // Calculate score (exclude not_applicable and skipped from denominator)
    const applicableRules = stats.total_rules - stats.not_applicable - stats.skipped;
    const score = applicableRules > 0
      ? ((stats.passed / applicableRules) * 100).toFixed(2)
      : null;

    // Create scan record
    const scan = await prisma.compliance_scans.create({
      data: {
        id: uuidv4(),
        host_id: host.id,
        profile_id: profile.id,
        started_at: started_at ? new Date(started_at) : new Date(),
        completed_at: completed_at ? new Date(completed_at) : new Date(),
        status: "completed",
        total_rules: stats.total_rules,
        passed: stats.passed,
        failed: stats.failed,
        warnings: stats.warnings,
        skipped: stats.skipped,
        score: score ? parseFloat(score) : null,
        raw_output: raw_output ? JSON.stringify(raw_output) : null,
      },
    });

    // Create rule and result records
    if (results && Array.isArray(results)) {
      for (const result of results) {
        // Upsert rule
        let rule = await prisma.compliance_rules.findFirst({
          where: {
            profile_id: profile.id,
            rule_ref: result.rule_ref || result.id,
          },
        });

        if (!rule) {
          rule = await prisma.compliance_rules.create({
            data: {
              id: uuidv4(),
              profile_id: profile.id,
              rule_ref: result.rule_ref || result.id,
              title: result.title || result.rule_ref || "Unknown",
              description: result.description || null,
              severity: result.severity || null,
              section: result.section || null,
              remediation: result.remediation || null,
            },
          });
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

    console.log(`[Compliance] Scan completed for host ${host.friendly_name || host.hostname}: ${stats.passed}/${stats.total_rules} passed (${score}%)`);

    res.json({
      message: "Scan results saved successfully",
      scan_id: scan.id,
      score: scan.score,
      stats,
    });
  } catch (error) {
    console.error("[Compliance] Error saving scan results:", error);
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
    console.error("[Compliance] Error fetching profiles:", error);
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

    res.json({
      summary: {
        total_hosts: totalHosts,
        average_score: Math.round(avgScore * 100) / 100,
        compliant,
        warning,
        critical,
        unscanned,
      },
      recent_scans: recentScans,
      worst_hosts: worstHosts,
    });
  } catch (error) {
    console.error("[Compliance] Error fetching dashboard:", error);
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
    const { limit = 20, offset = 0 } = req.query;

    const scans = await prisma.compliance_scans.findMany({
      where: { host_id: hostId },
      orderBy: { completed_at: "desc" },
      take: parseInt(limit),
      skip: parseInt(offset),
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
        limit: parseInt(limit),
        offset: parseInt(offset),
      },
    });
  } catch (error) {
    console.error("[Compliance] Error fetching scans:", error);
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
    console.error("[Compliance] Error fetching latest scan:", error);
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
    console.error("[Compliance] Error fetching results:", error);
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
    const { profile_type = "all" } = req.body; // "openscap", "docker-bench", or "all"

    const host = await prisma.hosts.findUnique({
      where: { id: hostId },
    });

    if (!host) {
      return res.status(404).json({ error: "Host not found" });
    }

    // Import agentWs to send WebSocket message
    const agentWs = require("../services/agentWs");

    if (!agentWs.isConnected(host.api_id)) {
      return res.status(400).json({ error: "Host is not connected" });
    }

    // Send compliance scan trigger via WebSocket
    const ws = agentWs.getConnectionByApiId(host.api_id);
    if (ws && ws.readyState === 1) { // WebSocket.OPEN
      ws.send(JSON.stringify({
        type: "compliance_scan",
        profile_type,
      }));

      res.json({
        message: "Compliance scan triggered",
        host_id: hostId,
        profile_type,
      });
    } else {
      res.status(400).json({ error: "Failed to send scan trigger" });
    }
  } catch (error) {
    console.error("[Compliance] Error triggering scan:", error);
    res.status(500).json({ error: "Failed to trigger scan" });
  }
});

/**
 * GET /api/v1/compliance/trends/:hostId
 * Get compliance score trends over time
 */
router.get("/trends/:hostId", async (req, res) => {
  try {
    const { hostId } = req.params;
    const { days = 30 } = req.query;

    const since = new Date();
    since.setDate(since.getDate() - parseInt(days));

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
    console.error("[Compliance] Error fetching trends:", error);
    res.status(500).json({ error: "Failed to fetch trends" });
  }
});

module.exports = router;
