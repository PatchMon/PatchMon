# Security Compliance REST API Specification

## Overview

This document defines the REST API endpoints for the Security Compliance feature. The API follows existing PatchMon conventions for authentication, error handling, and response formats.

## Base URL

```
/api/v1/compliance
```

## Authentication

### Dashboard Endpoints (User Session)
- Uses existing JWT-based `authenticateToken` middleware
- Requires valid user session via httpOnly cookie or Authorization header

### Agent Endpoints (API Key)
- Uses existing `validateApiCredentials` middleware
- Requires `X-API-ID` and `X-API-KEY` headers

---

## Agent Endpoints

### POST /api/v1/compliance/scans

Submit compliance scan results from agent.

**Authentication:** API Key (X-API-ID, X-API-KEY headers)

**Request Body:**

```json
{
  "profile": {
    "name": "CIS Ubuntu 22.04 L1",
    "type": "openscap",
    "version": "1.0.0"
  },
  "started_at": "2024-01-15T10:30:00Z",
  "completed_at": "2024-01-15T10:35:42Z",
  "status": "completed",
  "summary": {
    "total_rules": 245,
    "passed": 198,
    "failed": 32,
    "warnings": 8,
    "skipped": 7,
    "score": 80.82
  },
  "results": [
    {
      "rule_ref": "xccdf_org.cisecurity.benchmarks_rule_1.1.1",
      "title": "Ensure /tmp is a separate partition",
      "severity": "medium",
      "section": "1.1.1",
      "status": "fail",
      "finding": "/tmp is not a separate partition",
      "actual": "none",
      "expected": "separate partition",
      "remediation": "Create a separate /tmp partition during installation or using LVM"
    },
    {
      "rule_ref": "xccdf_org.cisecurity.benchmarks_rule_1.1.2",
      "title": "Ensure nodev option set on /tmp partition",
      "severity": "low",
      "section": "1.1.2",
      "status": "pass",
      "finding": null,
      "actual": "nodev",
      "expected": "nodev",
      "remediation": null
    }
  ],
  "raw_output": "... full scanner output (optional, for debugging) ..."
}
```

**Response (201 Created):**

```json
{
  "success": true,
  "message": "Compliance scan submitted successfully",
  "data": {
    "scan_id": "550e8400-e29b-41d4-a716-446655440000",
    "host_id": "660e8400-e29b-41d4-a716-446655440001",
    "profile_id": "770e8400-e29b-41d4-a716-446655440002",
    "score": 80.82,
    "status": "completed"
  }
}
```

**Error Responses:**

| Status | Error | Description |
|--------|-------|-------------|
| 400 | Invalid request body | Missing required fields or invalid data |
| 401 | Invalid API credentials | API ID or key is incorrect |
| 404 | Profile not found | Specified profile doesn't exist |
| 500 | Failed to process scan | Server error during processing |

---

## Dashboard Endpoints

### GET /api/v1/compliance/scans/:hostId

Get scan history for a specific host.

**Authentication:** User Session (JWT)

**Path Parameters:**
- `hostId` (string, required): Host UUID

**Query Parameters:**
- `page` (number, optional): Page number (default: 1)
- `limit` (number, optional): Items per page (default: 20, max: 100)
- `profile` (string, optional): Filter by profile name

**Response (200 OK):**

```json
{
  "scans": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "profile": {
        "id": "770e8400-e29b-41d4-a716-446655440002",
        "name": "CIS Ubuntu 22.04 L1",
        "type": "openscap"
      },
      "started_at": "2024-01-15T10:30:00Z",
      "completed_at": "2024-01-15T10:35:42Z",
      "status": "completed",
      "total_rules": 245,
      "passed": 198,
      "failed": 32,
      "warnings": 8,
      "skipped": 7,
      "score": 80.82
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 45,
    "totalPages": 3
  }
}
```

---

### GET /api/v1/compliance/scans/:hostId/latest

Get the most recent scan for a host.

**Authentication:** User Session (JWT)

**Path Parameters:**
- `hostId` (string, required): Host UUID

**Query Parameters:**
- `profile` (string, optional): Filter by profile name

**Response (200 OK):**

```json
{
  "scan": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "profile": {
      "id": "770e8400-e29b-41d4-a716-446655440002",
      "name": "CIS Ubuntu 22.04 L1",
      "type": "openscap"
    },
    "started_at": "2024-01-15T10:30:00Z",
    "completed_at": "2024-01-15T10:35:42Z",
    "status": "completed",
    "total_rules": 245,
    "passed": 198,
    "failed": 32,
    "warnings": 8,
    "skipped": 7,
    "score": 80.82,
    "summary": {
      "by_severity": {
        "critical": { "passed": 15, "failed": 2 },
        "high": { "passed": 45, "failed": 8 },
        "medium": { "passed": 98, "failed": 18 },
        "low": { "passed": 40, "failed": 4 }
      },
      "by_section": [
        { "section": "1", "title": "Initial Setup", "passed": 25, "failed": 5 },
        { "section": "2", "title": "Services", "passed": 35, "failed": 8 },
        { "section": "3", "title": "Network Configuration", "passed": 42, "failed": 6 }
      ]
    }
  },
  "host": {
    "id": "660e8400-e29b-41d4-a716-446655440001",
    "friendly_name": "web-server-01",
    "hostname": "web-server-01.example.com",
    "os_type": "Ubuntu",
    "os_version": "22.04"
  }
}
```

**Response (404 Not Found):**

```json
{
  "error": "No scans found for this host"
}
```

---

### GET /api/v1/compliance/results/:scanId

Get detailed results for a specific scan.

**Authentication:** User Session (JWT)

**Path Parameters:**
- `scanId` (string, required): Scan UUID

**Query Parameters:**
- `status` (string, optional): Filter by status ("pass", "fail", "warn", "skip")
- `severity` (string, optional): Filter by severity ("critical", "high", "medium", "low")
- `section` (string, optional): Filter by CIS section (e.g., "1.1")
- `search` (string, optional): Search in rule title/description
- `page` (number, optional): Page number (default: 1)
- `limit` (number, optional): Items per page (default: 50, max: 200)

**Response (200 OK):**

```json
{
  "scan": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "profile": {
      "id": "770e8400-e29b-41d4-a716-446655440002",
      "name": "CIS Ubuntu 22.04 L1"
    },
    "started_at": "2024-01-15T10:30:00Z",
    "completed_at": "2024-01-15T10:35:42Z",
    "status": "completed",
    "score": 80.82
  },
  "results": [
    {
      "id": "880e8400-e29b-41d4-a716-446655440003",
      "rule": {
        "id": "990e8400-e29b-41d4-a716-446655440004",
        "rule_ref": "xccdf_org.cisecurity.benchmarks_rule_1.1.1",
        "title": "Ensure /tmp is a separate partition",
        "description": "The /tmp directory is a world-writable directory used for temporary storage...",
        "rationale": "Placing /tmp on a separate partition enables the administrator to set...",
        "severity": "medium",
        "section": "1.1.1"
      },
      "status": "fail",
      "finding": "/tmp is not a separate partition",
      "actual": "none",
      "expected": "separate partition",
      "remediation": "Create a separate /tmp partition during installation or using LVM"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 50,
    "total": 245,
    "totalPages": 5
  }
}
```

---

### POST /api/v1/compliance/trigger/:hostId

Trigger an on-demand compliance scan via WebSocket.

**Authentication:** User Session (JWT) + requireManageHosts permission

**Path Parameters:**
- `hostId` (string, required): Host UUID

**Request Body:**

```json
{
  "profile": "CIS Ubuntu 22.04 L1"
}
```

**Response (202 Accepted):**

```json
{
  "success": true,
  "message": "Compliance scan triggered",
  "data": {
    "host_id": "660e8400-e29b-41d4-a716-446655440001",
    "profile": "CIS Ubuntu 22.04 L1",
    "triggered_at": "2024-01-15T10:30:00Z"
  }
}
```

**Error Responses:**

| Status | Error | Description |
|--------|-------|-------------|
| 400 | Invalid profile | Profile not found or not applicable to host |
| 404 | Host not found | Host doesn't exist |
| 503 | Agent not connected | Agent WebSocket not connected |

---

### GET /api/v1/compliance/profiles

List available compliance profiles.

**Authentication:** User Session (JWT)

**Query Parameters:**
- `type` (string, optional): Filter by type ("openscap", "docker-bench")
- `os_family` (string, optional): Filter by OS family

**Response (200 OK):**

```json
{
  "profiles": [
    {
      "id": "770e8400-e29b-41d4-a716-446655440002",
      "name": "CIS Ubuntu 22.04 L1",
      "type": "openscap",
      "os_family": "ubuntu",
      "version": "1.0.0",
      "description": "CIS Benchmark for Ubuntu 22.04 LTS - Level 1 Server",
      "rules_count": 245
    },
    {
      "id": "770e8400-e29b-41d4-a716-446655440003",
      "name": "CIS Docker",
      "type": "docker-bench",
      "os_family": null,
      "version": "1.5.0",
      "description": "CIS Docker Benchmark v1.5.0",
      "rules_count": 117
    }
  ]
}
```

---

### GET /api/v1/compliance/dashboard

Get aggregated compliance statistics for the dashboard.

**Authentication:** User Session (JWT)

**Query Parameters:**
- `days` (number, optional): Number of days for trend data (default: 30)

**Response (200 OK):**

```json
{
  "summary": {
    "total_hosts": 25,
    "hosts_scanned": 22,
    "hosts_never_scanned": 3,
    "average_score": 78.5,
    "hosts_by_status": {
      "compliant": 15,
      "warning": 5,
      "critical": 2
    }
  },
  "score_distribution": [
    { "range": "90-100", "count": 8 },
    { "range": "80-89", "count": 7 },
    { "range": "70-79", "count": 4 },
    { "range": "60-69", "count": 2 },
    { "range": "0-59", "count": 1 }
  ],
  "top_failing_rules": [
    {
      "rule_ref": "xccdf_org.cisecurity.benchmarks_rule_1.1.1",
      "title": "Ensure /tmp is a separate partition",
      "severity": "medium",
      "fail_count": 18,
      "profile": "CIS Ubuntu 22.04 L1"
    },
    {
      "rule_ref": "xccdf_org.cisecurity.benchmarks_rule_2.2.1",
      "title": "Ensure xinetd is not installed",
      "severity": "high",
      "fail_count": 12,
      "profile": "CIS Ubuntu 22.04 L1"
    }
  ],
  "recent_scans": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "host": {
        "id": "660e8400-e29b-41d4-a716-446655440001",
        "friendly_name": "web-server-01"
      },
      "profile": "CIS Ubuntu 22.04 L1",
      "score": 80.82,
      "completed_at": "2024-01-15T10:35:42Z"
    }
  ],
  "trends": {
    "dates": ["2024-01-01", "2024-01-08", "2024-01-15"],
    "average_scores": [75.2, 77.8, 78.5],
    "hosts_scanned": [18, 20, 22]
  }
}
```

---

### GET /api/v1/compliance/dashboard/hosts

Get hosts sorted by compliance score.

**Authentication:** User Session (JWT)

**Query Parameters:**
- `sort` (string, optional): Sort order ("asc", "desc", default: "asc")
- `status` (string, optional): Filter by status ("compliant", "warning", "critical", "never_scanned")
- `page` (number, optional): Page number (default: 1)
- `limit` (number, optional): Items per page (default: 25, max: 100)

**Response (200 OK):**

```json
{
  "hosts": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440001",
      "friendly_name": "web-server-01",
      "hostname": "web-server-01.example.com",
      "os_type": "Ubuntu",
      "os_version": "22.04",
      "compliance": {
        "score": 80.82,
        "status": "warning",
        "last_scan": "2024-01-15T10:35:42Z",
        "profile": "CIS Ubuntu 22.04 L1",
        "failed_rules": 32,
        "critical_fails": 2,
        "high_fails": 8
      }
    },
    {
      "id": "660e8400-e29b-41d4-a716-446655440002",
      "friendly_name": "db-server-01",
      "hostname": "db-server-01.example.com",
      "os_type": "RHEL",
      "os_version": "8.7",
      "compliance": {
        "score": 92.45,
        "status": "compliant",
        "last_scan": "2024-01-15T08:22:15Z",
        "profile": "CIS RHEL 8 L1",
        "failed_rules": 8,
        "critical_fails": 0,
        "high_fails": 1
      }
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 25,
    "total": 22,
    "totalPages": 1
  }
}
```

---

## Error Response Format

All error responses follow this format:

```json
{
  "error": "Error message",
  "details": "Additional details (optional)",
  "code": "ERROR_CODE (optional)"
}
```

## Status Codes

| Code | Description |
|------|-------------|
| 200 | Success |
| 201 | Created (scan submitted) |
| 202 | Accepted (scan triggered) |
| 400 | Bad Request |
| 401 | Unauthorized |
| 403 | Forbidden |
| 404 | Not Found |
| 500 | Internal Server Error |
| 503 | Service Unavailable (agent not connected) |

## Rate Limiting

- Agent scan submissions: 1 per minute per host
- Dashboard endpoints: Standard API rate limits
- Trigger endpoint: 1 per 5 minutes per host

## Implementation Notes

### Route File Structure

```javascript
// backend/src/routes/complianceRoutes.js
const express = require("express");
const { authenticateToken } = require("../middleware/auth");
const { requireManageHosts } = require("../middleware/permissions");
const { validateApiCredentials } = require("./hostRoutes");

const router = express.Router();

// Agent endpoint (API key auth)
router.post("/scans", validateApiCredentials, submitScan);

// Dashboard endpoints (JWT auth)
router.get("/scans/:hostId", authenticateToken, getScanHistory);
router.get("/scans/:hostId/latest", authenticateToken, getLatestScan);
router.get("/results/:scanId", authenticateToken, getScanResults);
router.post("/trigger/:hostId", authenticateToken, requireManageHosts, triggerScan);
router.get("/profiles", authenticateToken, getProfiles);
router.get("/dashboard", authenticateToken, getDashboard);
router.get("/dashboard/hosts", authenticateToken, getDashboardHosts);

module.exports = router;
```

### Register Routes

```javascript
// backend/src/routes/index.js
const complianceRoutes = require("./complianceRoutes");
// ...
app.use("/api/v1/compliance", complianceRoutes);
```
