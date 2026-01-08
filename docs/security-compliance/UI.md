# Security Compliance UI Specification

## Overview

This document defines the frontend components and pages for the Security Compliance feature. The UI follows existing PatchMon design patterns using React, TailwindCSS, and Lucide icons.

## Navigation Integration

Add "Compliance" item to the main navigation sidebar:

```jsx
// src/components/Sidebar.jsx
{
  name: "Compliance",
  icon: Shield,
  path: "/compliance",
  requiredPermission: "can_view_dashboard"
}
```

## Pages

### 1. Compliance Dashboard (`/compliance`)

Main entry point showing fleet-wide compliance status.

#### Wireframe

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  Compliance Dashboard                                           [Refresh]    │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐         │
│  │   22 / 25   │  │   78.5%     │  │     15      │  │      5      │         │
│  │   Hosts     │  │   Average   │  │  Compliant  │  │   Warning   │         │
│  │   Scanned   │  │   Score     │  │   (>80%)    │  │   (50-80%)  │         │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘         │
│                                                                              │
│  ┌────────────────────────────────────┐  ┌────────────────────────────────┐ │
│  │  Score Distribution                │  │  Compliance Trends (30 days)   │ │
│  │                                    │  │                                │ │
│  │  90-100  ████████████ 8            │  │      ___---~~~                 │ │
│  │  80-89   ██████████ 7              │  │   __/                          │ │
│  │  70-79   ██████ 4                  │  │  /                             │ │
│  │  60-69   ██ 2                      │  │ /     Score over time          │ │
│  │  0-59    █ 1                       │  │/                               │ │
│  │                                    │  │                                │ │
│  └────────────────────────────────────┘  └────────────────────────────────┘ │
│                                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  Top Failing Rules                                                    │  │
│  ├───────────────────────────────────────────────────────────────────────┤  │
│  │  Rule                                      Severity   Hosts Failing   │  │
│  │  ──────────────────────────────────────────────────────────────────── │  │
│  │  1.1.1 - Ensure /tmp is separate partition [MEDIUM]   18 hosts        │  │
│  │  2.2.1 - Ensure xinetd is not installed    [HIGH]     12 hosts        │  │
│  │  5.2.4 - Ensure SSH MaxAuthTries is set    [HIGH]     10 hosts        │  │
│  │  1.4.1 - Ensure AIDE is installed          [MEDIUM]   9 hosts         │  │
│  │  3.2.1 - Ensure IP forwarding disabled     [MEDIUM]   8 hosts         │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  Hosts by Compliance Score                [Filter ▼] [Sort ▼]         │  │
│  ├───────────────────────────────────────────────────────────────────────┤  │
│  │  Host              OS              Score    Status    Last Scan       │  │
│  │  ──────────────────────────────────────────────────────────────────── │  │
│  │  web-server-01     Ubuntu 22.04    80.8%   Warning   2 hours ago     │  │
│  │  db-server-01      RHEL 8.7        92.5%   OK        4 hours ago     │  │
│  │  app-server-01     Ubuntu 22.04    45.2%   Critical  1 day ago       │  │
│  │  docker-host-01    Ubuntu 22.04    85.0%   OK        3 hours ago     │  │
│  │                                                                       │  │
│  │  [<] Page 1 of 3 [>]                                                  │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

#### Component: ComplianceDashboard.jsx

```jsx
// src/pages/Compliance.jsx
import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  AlertTriangle,
  CheckCircle,
  RefreshCw,
  Server,
  Shield,
  XCircle,
} from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import api from "../utils/api";
import ComplianceScoreChart from "../components/ComplianceScoreChart";
import ComplianceTrendsChart from "../components/ComplianceTrendsChart";

const Compliance = () => {
  const [hostFilter, setHostFilter] = useState("all");
  const [sortBy, setSortBy] = useState("score");
  const [sortDir, setSortDir] = useState("asc");
  const [page, setPage] = useState(1);

  // Fetch dashboard data
  const { data: dashboard, isLoading: dashboardLoading, refetch } = useQuery({
    queryKey: ["compliance", "dashboard"],
    queryFn: () => api.get("/compliance/dashboard").then(res => res.data),
    refetchInterval: 60000,
  });

  // Fetch hosts list
  const { data: hostsData, isLoading: hostsLoading } = useQuery({
    queryKey: ["compliance", "hosts", hostFilter, sortBy, sortDir, page],
    queryFn: () => api.get("/compliance/dashboard/hosts", {
      params: { status: hostFilter, sort: sortDir, page, limit: 25 }
    }).then(res => res.data),
  });

  // ... component implementation
};
```

---

### 2. Host Compliance Tab

Add a "Compliance" tab to the existing Host Detail page.

#### Wireframe

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  ← web-server-01                                               [Refresh]     │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  [Host] [Packages] [Repositories] [Docker] [Compliance] [Terminal]          │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │  Compliance Overview                              [Scan Now ▼]         │ │
│  ├────────────────────────────────────────────────────────────────────────┤ │
│  │                                                                        │ │
│  │   ┌──────────────────┐   Profile: CIS Ubuntu 22.04 L1                 │ │
│  │   │                  │   Last Scan: 2 hours ago                        │ │
│  │   │      80.8%       │   Status: Warning                               │ │
│  │   │                  │                                                 │ │
│  │   │   ████████░░     │   Total Rules: 245                              │ │
│  │   │                  │   ├── Passed:    198 (80.8%)                    │ │
│  │   └──────────────────┘   ├── Failed:     32 (13.1%)                    │ │
│  │                          ├── Warnings:    8 (3.3%)                     │ │
│  │                          └── Skipped:     7 (2.9%)                     │ │
│  │                                                                        │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │  Scan History                                                          │ │
│  ├────────────────────────────────────────────────────────────────────────┤ │
│  │  Date                   Profile              Score   Rules   Status    │ │
│  │  ──────────────────────────────────────────────────────────────────── │ │
│  │  Jan 15, 2024 10:35    CIS Ubuntu 22.04 L1  80.8%   245     Complete  │ │
│  │  Jan 8, 2024 10:32     CIS Ubuntu 22.04 L1  78.3%   245     Complete  │ │
│  │  Jan 1, 2024 10:28     CIS Ubuntu 22.04 L1  75.1%   245     Complete  │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │  Failed Rules (32)                    [Search...] [Severity ▼]         │ │
│  ├────────────────────────────────────────────────────────────────────────┤ │
│  │  [v] 1.1.1 - Ensure /tmp is separate partition              [MEDIUM]   │ │
│  │      Finding: /tmp is not a separate partition                         │ │
│  │      Remediation: Create a separate /tmp partition during install...   │ │
│  │                                                                        │ │
│  │  [>] 2.2.1 - Ensure xinetd is not installed                 [HIGH]     │ │
│  │  [>] 5.2.4 - Ensure SSH MaxAuthTries is set to 4 or less    [HIGH]     │ │
│  │  [>] 1.4.1 - Ensure AIDE is installed                       [MEDIUM]   │ │
│  │                                                                        │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

#### Component Integration

```jsx
// In src/pages/HostDetail.jsx

// Add to tabs array
const tabs = [
  { id: "host", label: "Host", icon: Server },
  { id: "packages", label: "Packages", icon: Package },
  { id: "repositories", label: "Repositories", icon: Database },
  { id: "docker", label: "Docker", icon: Container },
  { id: "compliance", label: "Compliance", icon: Shield },
  { id: "terminal", label: "Terminal", icon: Terminal },
];

// Add tab content
{activeTab === "compliance" && (
  <HostComplianceTab hostId={hostId} />
)}
```

---

### 3. Scan Results Drill-Down (`/compliance/scan/:scanId`)

Detailed view of a specific scan's results.

#### Wireframe

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  ← Scan Results - CIS Ubuntu 22.04 L1                                        │
│  Host: web-server-01 | Jan 15, 2024 10:35 AM                                 │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐         │
│  │     245     │  │     198     │  │      32     │  │    80.8%    │         │
│  │    Total    │  │   Passed    │  │   Failed    │  │    Score    │         │
│  │    Rules    │  │             │  │             │  │             │         │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘         │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │  Results by Section                                                    │ │
│  ├────────────────────────────────────────────────────────────────────────┤ │
│  │                                                                        │ │
│  │  Section                              Passed   Failed   Score          │ │
│  │  ────────────────────────────────────────────────────────────────────  │ │
│  │  1. Initial Setup                     25       5        83.3%          │ │
│  │  2. Services                          35       8        81.4%          │ │
│  │  3. Network Configuration             42       6        87.5%          │ │
│  │  4. Logging and Auditing              28       4        87.5%          │ │
│  │  5. Access, Authentication, Auth      40       7        85.1%          │ │
│  │  6. System Maintenance                28       2        93.3%          │ │
│  │                                                                        │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │  All Rules                       [Search...] [Status ▼] [Severity ▼]   │ │
│  ├────────────────────────────────────────────────────────────────────────┤ │
│  │                                                                        │ │
│  │  [✓] 1.1.1.1 - Ensure mounting of cramfs is disabled       [LOW]      │ │
│  │      Status: PASS                                                      │ │
│  │                                                                        │ │
│  │  [✗] 1.1.1.2 - Ensure /tmp is a separate partition         [MEDIUM]   │ │
│  │      Status: FAIL                                                      │ │
│  │      ┌─────────────────────────────────────────────────────────────┐   │ │
│  │      │ Finding:     /tmp is not a separate partition               │   │ │
│  │      │ Expected:    Separate partition mounted at /tmp             │   │ │
│  │      │ Actual:      None                                           │   │ │
│  │      │                                                             │   │ │
│  │      │ Rationale:                                                  │   │ │
│  │      │ Placing /tmp on a separate partition enables admin to...    │   │ │
│  │      │                                                             │   │ │
│  │      │ Remediation:                                                │   │ │
│  │      │ Configure /etc/fstab or create a separate partition...      │   │ │
│  │      └─────────────────────────────────────────────────────────────┘   │ │
│  │                                                                        │ │
│  │  [✓] 1.1.1.3 - Ensure nodev option set on /tmp             [LOW]      │ │
│  │      Status: PASS                                                      │ │
│  │                                                                        │ │
│  │  [<] Page 1 of 5 [>]                                                   │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## Components

### ComplianceScore

Visual score display component.

```jsx
// src/components/ComplianceScore.jsx

const ComplianceScore = ({ score, size = "md", showLabel = true }) => {
  const getScoreColor = (score) => {
    if (score >= 80) return "text-green-500";
    if (score >= 50) return "text-yellow-500";
    return "text-red-500";
  };

  const getScoreBgColor = (score) => {
    if (score >= 80) return "bg-green-500/20";
    if (score >= 50) return "bg-yellow-500/20";
    return "bg-red-500/20";
  };

  const sizes = {
    sm: { container: "w-16 h-16", text: "text-lg", label: "text-xs" },
    md: { container: "w-24 h-24", text: "text-2xl", label: "text-sm" },
    lg: { container: "w-32 h-32", text: "text-4xl", label: "text-base" },
  };

  const { container, text, label } = sizes[size];

  return (
    <div className={`${container} relative`}>
      {/* Circular progress */}
      <svg className="w-full h-full transform -rotate-90">
        <circle
          cx="50%"
          cy="50%"
          r="45%"
          strokeWidth="8"
          stroke="currentColor"
          className="text-surface-600"
          fill="none"
        />
        <circle
          cx="50%"
          cy="50%"
          r="45%"
          strokeWidth="8"
          stroke="currentColor"
          className={getScoreColor(score)}
          fill="none"
          strokeDasharray={`${score * 2.83} 283`}
          strokeLinecap="round"
        />
      </svg>
      {/* Score text */}
      <div className="absolute inset-0 flex flex-col items-center justify-center">
        <span className={`${text} font-bold ${getScoreColor(score)}`}>
          {score.toFixed(1)}%
        </span>
        {showLabel && (
          <span className={`${label} text-gray-400`}>Score</span>
        )}
      </div>
    </div>
  );
};
```

### ComplianceTrendsChart

Line chart showing score trends over time.

```jsx
// src/components/ComplianceTrendsChart.jsx

import { Line } from "react-chartjs-2";

const ComplianceTrendsChart = ({ data }) => {
  const chartData = {
    labels: data.dates,
    datasets: [
      {
        label: "Average Score",
        data: data.average_scores,
        borderColor: "rgb(59, 130, 246)",
        backgroundColor: "rgba(59, 130, 246, 0.1)",
        fill: true,
        tension: 0.4,
      },
    ],
  };

  const options = {
    responsive: true,
    maintainAspectRatio: false,
    scales: {
      y: {
        min: 0,
        max: 100,
        ticks: {
          callback: (value) => `${value}%`,
        },
      },
    },
    plugins: {
      tooltip: {
        callbacks: {
          label: (context) => `Score: ${context.raw.toFixed(1)}%`,
        },
      },
    },
  };

  return (
    <div className="h-64">
      <Line data={chartData} options={options} />
    </div>
  );
};
```

### RuleResultCard

Expandable card for individual rule results.

```jsx
// src/components/RuleResultCard.jsx

const RuleResultCard = ({ result, expanded, onToggle }) => {
  const getStatusIcon = (status) => {
    switch (status) {
      case "pass":
        return <CheckCircle className="w-5 h-5 text-green-500" />;
      case "fail":
        return <XCircle className="w-5 h-5 text-red-500" />;
      case "warn":
        return <AlertTriangle className="w-5 h-5 text-yellow-500" />;
      default:
        return <MinusCircle className="w-5 h-5 text-gray-500" />;
    }
  };

  const getSeverityBadge = (severity) => {
    const colors = {
      critical: "bg-red-500/20 text-red-400",
      high: "bg-orange-500/20 text-orange-400",
      medium: "bg-yellow-500/20 text-yellow-400",
      low: "bg-blue-500/20 text-blue-400",
    };
    return colors[severity] || colors.low;
  };

  return (
    <div className="border border-surface-700 rounded-lg overflow-hidden">
      {/* Header - always visible */}
      <button
        onClick={onToggle}
        className="w-full px-4 py-3 flex items-center justify-between hover:bg-surface-700/50"
      >
        <div className="flex items-center gap-3">
          {getStatusIcon(result.status)}
          <span className="font-medium">{result.rule.section}</span>
          <span className="text-gray-300">{result.rule.title}</span>
        </div>
        <div className="flex items-center gap-3">
          <span className={`px-2 py-1 rounded text-xs ${getSeverityBadge(result.rule.severity)}`}>
            {result.rule.severity.toUpperCase()}
          </span>
          <ChevronDown className={`w-5 h-5 transition-transform ${expanded ? "rotate-180" : ""}`} />
        </div>
      </button>

      {/* Expanded content */}
      {expanded && (
        <div className="px-4 py-3 bg-surface-800 border-t border-surface-700">
          {result.finding && (
            <div className="mb-3">
              <span className="text-sm text-gray-400">Finding:</span>
              <p className="text-gray-200">{result.finding}</p>
            </div>
          )}

          {result.rule.rationale && (
            <div className="mb-3">
              <span className="text-sm text-gray-400">Rationale:</span>
              <p className="text-gray-300">{result.rule.rationale}</p>
            </div>
          )}

          {result.remediation && (
            <div className="mb-3">
              <span className="text-sm text-gray-400">Remediation:</span>
              <p className="text-gray-300">{result.remediation}</p>
            </div>
          )}

          <div className="flex gap-6 text-sm">
            <div>
              <span className="text-gray-400">Expected:</span>
              <span className="ml-2 text-gray-200">{result.expected || "N/A"}</span>
            </div>
            <div>
              <span className="text-gray-400">Actual:</span>
              <span className="ml-2 text-gray-200">{result.actual || "N/A"}</span>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
```

### ScanNowButton

Dropdown button to trigger on-demand scans.

```jsx
// src/components/ScanNowButton.jsx

const ScanNowButton = ({ hostId, profiles, onScanTriggered }) => {
  const [isOpen, setIsOpen] = useState(false);
  const [triggering, setTriggering] = useState(false);

  const triggerScan = async (profile) => {
    setTriggering(true);
    try {
      await api.post(`/compliance/trigger/${hostId}`, { profile });
      onScanTriggered?.();
      setIsOpen(false);
    } catch (error) {
      console.error("Failed to trigger scan:", error);
    } finally {
      setTriggering(false);
    }
  };

  return (
    <div className="relative">
      <button
        onClick={() => setIsOpen(!isOpen)}
        disabled={triggering}
        className="flex items-center gap-2 px-4 py-2 bg-primary-600 hover:bg-primary-500 rounded-lg"
      >
        <Play className="w-4 h-4" />
        Scan Now
        <ChevronDown className="w-4 h-4" />
      </button>

      {isOpen && (
        <div className="absolute right-0 mt-2 w-64 bg-surface-800 border border-surface-700 rounded-lg shadow-lg z-10">
          {profiles.map((profile) => (
            <button
              key={profile.id}
              onClick={() => triggerScan(profile.name)}
              className="w-full px-4 py-2 text-left hover:bg-surface-700 first:rounded-t-lg last:rounded-b-lg"
            >
              <div className="font-medium">{profile.name}</div>
              <div className="text-sm text-gray-400">{profile.type}</div>
            </button>
          ))}
        </div>
      )}
    </div>
  );
};
```

---

## API Integration

### API Utilities

```jsx
// src/utils/api.js - add compliance endpoints

export const complianceAPI = {
  getDashboard: () => api.get("/compliance/dashboard"),
  getDashboardHosts: (params) => api.get("/compliance/dashboard/hosts", { params }),
  getScanHistory: (hostId, params) => api.get(`/compliance/scans/${hostId}`, { params }),
  getLatestScan: (hostId) => api.get(`/compliance/scans/${hostId}/latest`),
  getScanResults: (scanId, params) => api.get(`/compliance/results/${scanId}`, { params }),
  getProfiles: () => api.get("/compliance/profiles"),
  triggerScan: (hostId, profile) => api.post(`/compliance/trigger/${hostId}`, { profile }),
};
```

---

## Routing

Add routes in `App.jsx`:

```jsx
// src/App.jsx

import Compliance from "./pages/Compliance";
import ScanResults from "./pages/ScanResults";

// In routes array:
{ path: "/compliance", element: <Compliance /> },
{ path: "/compliance/scan/:scanId", element: <ScanResults /> },
```

---

## Color Coding

| Score Range | Status | Color |
|-------------|--------|-------|
| 80-100% | Compliant | Green (`text-green-500`, `bg-green-500/20`) |
| 50-79% | Warning | Yellow (`text-yellow-500`, `bg-yellow-500/20`) |
| 0-49% | Critical | Red (`text-red-500`, `bg-red-500/20`) |

| Severity | Color |
|----------|-------|
| Critical | Red (`text-red-400`, `bg-red-500/20`) |
| High | Orange (`text-orange-400`, `bg-orange-500/20`) |
| Medium | Yellow (`text-yellow-400`, `bg-yellow-500/20`) |
| Low | Blue (`text-blue-400`, `bg-blue-500/20`) |

---

## Responsive Design

All components should be responsive:

- Dashboard cards stack on mobile
- Tables become card lists on small screens
- Charts resize appropriately
- Navigation collapses to hamburger menu

---

## Accessibility

- All interactive elements have focus states
- Color coding supplemented with icons
- Screen reader labels on status indicators
- Keyboard navigation for expandable sections
