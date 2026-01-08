# Security Compliance Feature Architecture

## Overview

The Security Compliance feature integrates industry-standard security scanning tools into PatchMon to provide automated CIS benchmark compliance checking for both hosts (via OpenSCAP) and Docker environments (via Docker Bench for Security).

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              PatchMon Server                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────────────┐  │
│  │   REST API      │  │   WebSocket     │  │      Background Jobs        │  │
│  │   Endpoints     │  │   Handler       │  │      (BullMQ)               │  │
│  │                 │  │                 │  │                             │  │
│  │ - Submit scans  │  │ - Trigger scan  │  │ - Process scan results     │  │
│  │ - Get results   │  │ - Push status   │  │ - Generate reports         │  │
│  │ - Dashboard     │  │ - Real-time     │  │ - Cleanup old data         │  │
│  └────────┬────────┘  └────────┬────────┘  └──────────────┬──────────────┘  │
│           │                    │                          │                  │
│           └────────────────────┼──────────────────────────┘                  │
│                                │                                             │
│                    ┌───────────▼───────────┐                                 │
│                    │    Compliance         │                                 │
│                    │    Service Layer      │                                 │
│                    │                       │                                 │
│                    │ - Parse scan output   │                                 │
│                    │ - Store results       │                                 │
│                    │ - Calculate scores    │                                 │
│                    └───────────┬───────────┘                                 │
│                                │                                             │
│                    ┌───────────▼───────────┐                                 │
│                    │    PostgreSQL         │                                 │
│                    │    (Prisma ORM)       │                                 │
│                    │                       │                                 │
│                    │ - compliance_profiles │                                 │
│                    │ - compliance_scans    │                                 │
│                    │ - compliance_results  │                                 │
│                    │ - compliance_rules    │                                 │
│                    └───────────────────────┘                                 │
└─────────────────────────────────────────────────────────────────────────────┘
                                    ▲
                                    │ HTTPS / WebSocket
                                    │
┌───────────────────────────────────┼─────────────────────────────────────────┐
│                           PatchMon Agent                                     │
├───────────────────────────────────┼─────────────────────────────────────────┤
│                                   │                                          │
│  ┌────────────────────────────────▼────────────────────────────────────┐    │
│  │                     Compliance Module                                │    │
│  │                                                                      │    │
│  │  ┌──────────────────────┐    ┌──────────────────────┐              │    │
│  │  │   OpenSCAP Runner    │    │  Docker Bench Runner │              │    │
│  │  │                      │    │                      │              │    │
│  │  │ - Detect SCAP tools  │    │ - Run docker-bench   │              │    │
│  │  │ - Find CIS content   │    │ - Parse JSON output  │              │    │
│  │  │ - Execute scans      │    │ - Map to CIS rules   │              │    │
│  │  │ - Parse XCCDF/ARF    │    │                      │              │    │
│  │  └──────────────────────┘    └──────────────────────┘              │    │
│  │                                                                      │    │
│  └──────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                     Scheduler                                         │   │
│  │                                                                       │   │
│  │  - Cron-based scheduled scans                                        │   │
│  │  - On-demand via WebSocket command                                   │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────────────────────┘
```

## Data Flow

### 1. Scheduled Compliance Scan

```
┌─────────┐     ┌─────────────────┐     ┌──────────────┐     ┌────────────┐
│  Cron   │────▶│  Agent runs     │────▶│  Agent POSTs │────▶│  Server    │
│  Timer  │     │  OpenSCAP/      │     │  results to  │     │  stores    │
│         │     │  Docker Bench   │     │  /api/v1/    │     │  in DB     │
│         │     │                 │     │  compliance/ │     │            │
│         │     │                 │     │  scans       │     │            │
└─────────┘     └─────────────────┘     └──────────────┘     └────────────┘
```

### 2. On-Demand Compliance Scan

```
┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│ Frontend │────▶│  Server  │────▶│  Agent   │────▶│  Agent   │────▶│  Server  │
│  clicks  │     │  sends   │     │  receives│     │  runs    │     │  stores  │
│  "Scan   │     │  WebSocket│    │  trigger │     │  scan &  │     │  results │
│   Now"   │     │  command │     │  command │     │  POSTs   │     │          │
└──────────┘     └──────────┘     └──────────┘     └──────────┘     └──────────┘
```

### 3. Dashboard Data Flow

```
┌──────────┐     ┌──────────────────┐     ┌──────────────────┐
│ Frontend │────▶│  GET /api/v1/    │────▶│  Aggregated      │
│  loads   │     │  compliance/     │     │  stats from      │
│  page    │     │  dashboard       │     │  compliance_     │
│          │     │                  │     │  scans table     │
└──────────┘     └──────────────────┘     └──────────────────┘
                           │
                           ▼
              ┌──────────────────────────┐
              │  - Total hosts scanned   │
              │  - Average score         │
              │  - Hosts by score range  │
              │  - Recent scan activity  │
              │  - Top failing rules     │
              └──────────────────────────┘
```

## Component Interactions

### Integration with Existing PatchMon Features

| Existing Component | Integration Point | Purpose |
|-------------------|-------------------|---------|
| `hosts` table | `compliance_scans.host_id` FK | Associate scans with hosts |
| WebSocket service (`agentWs.js`) | New `compliance_trigger` message type | Trigger on-demand scans |
| Authentication middleware | Reuse `authenticateToken` | Protect dashboard endpoints |
| API key validation | Reuse `validateApiCredentials` | Authenticate agent submissions |
| BullMQ queues | New `compliance-processing` queue | Async result processing |
| Host detail page | New "Compliance" tab | Show host compliance status |

### New Components

1. **Backend Services**
   - `src/services/compliance/` - Compliance business logic
   - `src/routes/complianceRoutes.js` - REST API endpoints

2. **Frontend Pages/Components**
   - `src/pages/Compliance.jsx` - Main compliance dashboard
   - `src/components/ComplianceScore.jsx` - Score display widget
   - `src/components/ComplianceTrends.jsx` - Trends chart
   - `src/components/RuleResults.jsx` - Detailed rule results table

3. **Agent Modules**
   - `compliance/openscap.go` - OpenSCAP execution and parsing
   - `compliance/dockerbench.go` - Docker Bench execution
   - `compliance/scheduler.go` - Compliance scan scheduling

## Supported Compliance Profiles

### OpenSCAP (Host Compliance)

| Profile Name | OS Family | Description |
|-------------|-----------|-------------|
| CIS Ubuntu 22.04 L1 | ubuntu | Level 1 server profile |
| CIS Ubuntu 22.04 L2 | ubuntu | Level 2 server profile |
| CIS Ubuntu 20.04 L1 | ubuntu | Level 1 server profile |
| CIS RHEL 8 L1 | rhel | Level 1 server profile |
| CIS RHEL 8 L2 | rhel | Level 2 server profile |
| CIS Debian 11 L1 | debian | Level 1 server profile |

### Docker Bench for Security

| Profile Name | Type | Description |
|-------------|------|-------------|
| CIS Docker | docker-bench | CIS Docker Benchmark v1.5.0 |

## Security Considerations

1. **Agent Authentication**
   - All scan submissions use existing API key authentication
   - API keys are hashed (bcrypt) in database
   - Legacy plaintext keys supported for migration

2. **Data Sanitization**
   - Raw scan output stored but sanitized before display
   - SQL injection prevented via Prisma parameterized queries
   - XSS prevention on frontend rendering

3. **Access Control**
   - Dashboard requires authenticated user session
   - Per-host compliance data respects host visibility rules
   - Admin-only access to trigger forced scans

4. **Scan Execution Safety**
   - OpenSCAP runs read-only profile evaluation
   - Docker Bench runs with read-only Docker socket access
   - No remediation actions executed automatically

## Scalability Considerations

1. **Large Result Sets**
   - Paginated API responses (default 50 items)
   - Indexed database queries on scan_id, host_id
   - Old scan results purged after configurable retention period

2. **Concurrent Scans**
   - BullMQ queue prevents database overwhelm
   - Rate limiting on trigger endpoints
   - Stale scan detection and cleanup

3. **Dashboard Performance**
   - Pre-computed aggregate statistics
   - Cached compliance scores
   - Lazy loading for detailed drill-downs
