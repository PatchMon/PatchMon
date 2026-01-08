# Security Compliance API Reference

## Authentication

### Agent Endpoints
Agent endpoints use API key authentication via headers:
- `X-API-ID`: Host API ID
- `X-API-KEY`: Host API Key

### Dashboard Endpoints
Dashboard endpoints require JWT authentication:
- `Authorization: Bearer <token>`

---

## Endpoints

### Submit Scan Results
**POST** `/api/v1/compliance/scans`

Submit compliance scan results from an agent.

**Authentication:** API Key

**Request Body:**
```json
{
  "profile_name": "CIS Docker Benchmark",
  "profile_type": "docker-bench",
  "started_at": "2024-01-15T10:00:00Z",
  "completed_at": "2024-01-15T10:05:00Z",
  "results": [
    {
      "rule_ref": "1.1.1",
      "title": "Ensure a separate partition for containers has been created",
      "status": "pass",
      "severity": "high",
      "section": "1",
      "finding": "Check passed",
      "actual": "/var/lib/docker on separate partition",
      "expected": "Separate partition for /var/lib/docker"
    }
  ],
  "raw_output": "..."
}
```

**Response:**
```json
{
  "message": "Scan results saved successfully",
  "scan_id": "uuid",
  "score": 85.5,
  "stats": {
    "total_rules": 100,
    "passed": 85,
    "failed": 10,
    "warnings": 3,
    "skipped": 2,
    "not_applicable": 0
  }
}
```

---

### Get Dashboard Statistics
**GET** `/api/v1/compliance/dashboard`

Get aggregated compliance statistics for all hosts.

**Authentication:** JWT

**Response:**
```json
{
  "summary": {
    "total_hosts": 50,
    "average_score": 78.5,
    "compliant": 35,
    "warning": 10,
    "critical": 3,
    "unscanned": 2
  },
  "recent_scans": [...],
  "worst_hosts": [...]
}
```

---

### List Profiles
**GET** `/api/v1/compliance/profiles`

List all available compliance profiles.

**Authentication:** JWT

**Response:**
```json
[
  {
    "id": "uuid",
    "name": "CIS Ubuntu 22.04 Level 1",
    "type": "openscap",
    "os_family": "ubuntu",
    "_count": {
      "rules": 250,
      "scans": 100
    }
  }
]
```

---

### Get Scan History
**GET** `/api/v1/compliance/scans/:hostId`

Get scan history for a specific host.

**Authentication:** JWT

**Query Parameters:**
- `limit` (default: 20) - Number of results
- `offset` (default: 0) - Pagination offset

**Response:**
```json
{
  "scans": [...],
  "pagination": {
    "total": 50,
    "limit": 20,
    "offset": 0
  }
}
```

---

### Get Latest Scan
**GET** `/api/v1/compliance/scans/:hostId/latest`

Get the most recent completed scan for a host.

**Authentication:** JWT

**Response:** Full scan object with results

**Error Response (404):**
```json
{
  "error": "No scans found for this host"
}
```

---

### Get Scan Results
**GET** `/api/v1/compliance/results/:scanId`

Get detailed results for a specific scan.

**Authentication:** JWT

**Query Parameters:**
- `status` - Filter by status (pass, fail, warn, skip)
- `severity` - Filter by severity (low, medium, high, critical)

**Response:**
```json
[
  {
    "id": "uuid",
    "status": "fail",
    "compliance_rules": {
      "title": "Ensure /tmp is a separate partition",
      "severity": "high",
      "rule_ref": "1.1.1"
    }
  }
]
```

---

### Trigger Scan
**POST** `/api/v1/compliance/trigger/:hostId`

Trigger an on-demand compliance scan via WebSocket.

**Authentication:** JWT

**Request Body:**
```json
{
  "profile_type": "all"
}
```

Profile type options: `"openscap"`, `"docker-bench"`, or `"all"`

**Response:**
```json
{
  "message": "Compliance scan triggered",
  "host_id": "uuid",
  "profile_type": "all"
}
```

**Error Responses:**

| Status | Error | Description |
|--------|-------|-------------|
| 400 | Host is not connected | Agent WebSocket not connected |
| 404 | Host not found | Host doesn't exist |

---

### Get Score Trends
**GET** `/api/v1/compliance/trends/:hostId`

Get compliance score trends over time.

**Authentication:** JWT

**Query Parameters:**
- `days` (default: 30) - Number of days of history

**Response:**
```json
[
  {
    "completed_at": "2024-01-15T10:00:00Z",
    "score": 85.0,
    "profile": {
      "name": "CIS Docker",
      "type": "docker-bench"
    }
  }
]
```

---

## Status Codes

| Code | Description |
|------|-------------|
| 200 | Success |
| 400 | Bad Request |
| 401 | Unauthorized |
| 404 | Not Found |
| 500 | Internal Server Error |

## Rate Limiting

- Agent scan submissions: 1 per minute per host
- Dashboard endpoints: Standard API rate limits
- Trigger endpoint: 1 per 5 minutes per host
