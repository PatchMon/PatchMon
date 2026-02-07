# Security Compliance Agent Specification

## Overview

This document defines the agent-side requirements for the Security Compliance feature. The agent is responsible for detecting available scanning tools, executing compliance scans, parsing results, and submitting them to the PatchMon server.

## Supported Scanners

### OpenSCAP (Host Compliance)

OpenSCAP is an open-source framework for SCAP-based security compliance checking.

### Docker Bench for Security

Docker Bench for Security is a script that checks for dozens of common best-practices around deploying Docker containers in production.

---

## OpenSCAP Integration

### Detection

The agent should detect OpenSCAP availability on startup and periodically:

```go
// compliance/openscap.go

type OpenSCAPDetection struct {
    Installed       bool   `json:"installed"`
    Version         string `json:"version"`
    OscapPath       string `json:"oscap_path"`
    ContentPath     string `json:"content_path"`
    AvailableProfiles []ProfileInfo `json:"available_profiles"`
}

type ProfileInfo struct {
    ID          string `json:"id"`
    Title       string `json:"title"`
    Description string `json:"description"`
    FilePath    string `json:"file_path"`
}

func DetectOpenSCAP() (*OpenSCAPDetection, error) {
    // Check if oscap is installed
    oscapPath, err := exec.LookPath("oscap")
    if err != nil {
        return &OpenSCAPDetection{Installed: false}, nil
    }

    // Get version
    cmd := exec.Command(oscapPath, "--version")
    output, err := cmd.Output()
    if err != nil {
        return nil, err
    }
    version := parseVersion(string(output))

    // Find SCAP content
    contentPaths := []string{
        "/usr/share/xml/scap/ssg/content",
        "/usr/share/scap-security-guide",
        "/opt/scap/content",
    }

    var contentPath string
    var profiles []ProfileInfo
    for _, path := range contentPaths {
        if _, err := os.Stat(path); err == nil {
            contentPath = path
            profiles = discoverProfiles(path)
            break
        }
    }

    return &OpenSCAPDetection{
        Installed:         true,
        Version:           version,
        OscapPath:         oscapPath,
        ContentPath:       contentPath,
        AvailableProfiles: profiles,
    }, nil
}
```

### Profile Discovery

Discover available SCAP content (Data Stream files):

```go
func discoverProfiles(contentPath string) []ProfileInfo {
    profiles := []ProfileInfo{}

    // Look for SCAP data stream files
    files, _ := filepath.Glob(filepath.Join(contentPath, "ssg-*-ds.xml"))

    for _, file := range files {
        // Extract profile info using oscap
        cmd := exec.Command("oscap", "info", file)
        output, err := cmd.Output()
        if err != nil {
            continue
        }

        // Parse profiles from output
        // Example output:
        // Profiles:
        //   xccdf_org.ssgproject.content_profile_cis_level1_server
        //     Title: CIS Ubuntu 22.04 LTS - Level 1 Server
        parsed := parseOscapInfo(string(output))
        for _, p := range parsed {
            p.FilePath = file
            profiles = append(profiles, p)
        }
    }

    return profiles
}
```

### Scan Execution

Execute OpenSCAP scan with specified profile:

```go
type ScanRequest struct {
    ProfileID   string `json:"profile_id"`
    ProfileName string `json:"profile_name"`
    DataStream  string `json:"data_stream"`
}

type ScanResult struct {
    ProfileName   string        `json:"profile_name"`
    ProfileType   string        `json:"profile_type"`
    Version       string        `json:"version"`
    StartedAt     time.Time     `json:"started_at"`
    CompletedAt   time.Time     `json:"completed_at"`
    Status        string        `json:"status"`
    Summary       ScanSummary   `json:"summary"`
    Results       []RuleResult  `json:"results"`
    RawOutput     string        `json:"raw_output,omitempty"`
}

type ScanSummary struct {
    TotalRules int     `json:"total_rules"`
    Passed     int     `json:"passed"`
    Failed     int     `json:"failed"`
    Warnings   int     `json:"warnings"`
    Skipped    int     `json:"skipped"`
    Score      float64 `json:"score"`
}

type RuleResult struct {
    RuleRef     string `json:"rule_ref"`
    Title       string `json:"title"`
    Description string `json:"description,omitempty"`
    Rationale   string `json:"rationale,omitempty"`
    Severity    string `json:"severity"`
    Section     string `json:"section,omitempty"`
    Status      string `json:"status"`
    Finding     string `json:"finding,omitempty"`
    Actual      string `json:"actual,omitempty"`
    Expected    string `json:"expected,omitempty"`
    Remediation string `json:"remediation,omitempty"`
}

func RunOpenSCAPScan(req ScanRequest) (*ScanResult, error) {
    startedAt := time.Now()

    // Create temp file for results
    resultsFile, _ := os.CreateTemp("", "oscap-results-*.xml")
    arfFile, _ := os.CreateTemp("", "oscap-arf-*.xml")
    defer os.Remove(resultsFile.Name())
    defer os.Remove(arfFile.Name())

    // Run oscap evaluation
    cmd := exec.Command("oscap", "xccdf", "eval",
        "--profile", req.ProfileID,
        "--results", resultsFile.Name(),
        "--results-arf", arfFile.Name(),
        req.DataStream,
    )

    rawOutput, _ := cmd.CombinedOutput()
    completedAt := time.Now()

    // Parse results from XML
    results, summary := parseXCCDFResults(resultsFile.Name())

    return &ScanResult{
        ProfileName:  req.ProfileName,
        ProfileType:  "openscap",
        StartedAt:    startedAt,
        CompletedAt:  completedAt,
        Status:       "completed",
        Summary:      summary,
        Results:      results,
        RawOutput:    string(rawOutput),
    }, nil
}
```

### XCCDF Results Parsing

Parse OpenSCAP XCCDF results XML:

```go
func parseXCCDFResults(resultsFile string) ([]RuleResult, ScanSummary) {
    data, _ := os.ReadFile(resultsFile)

    // Parse XML using encoding/xml
    type TestResult struct {
        RuleID  string `xml:"idref,attr"`
        Result  string `xml:"result"`
        Message string `xml:"message,omitempty"`
    }

    type Benchmark struct {
        TestResults []TestResult `xml:"TestResult>rule-result"`
    }

    var bench Benchmark
    xml.Unmarshal(data, &bench)

    results := []RuleResult{}
    summary := ScanSummary{}

    for _, tr := range bench.TestResults {
        result := RuleResult{
            RuleRef: tr.RuleID,
            Status:  mapResultStatus(tr.Result),
            Finding: tr.Message,
        }

        // Enrich with rule metadata from data stream
        enrichRuleMetadata(&result)

        results = append(results, result)

        // Update summary counts
        summary.TotalRules++
        switch result.Status {
        case "pass":
            summary.Passed++
        case "fail":
            summary.Failed++
        case "warn":
            summary.Warnings++
        case "skip", "notapplicable":
            summary.Skipped++
        }
    }

    // Calculate score
    if summary.TotalRules > 0 {
        summary.Score = float64(summary.Passed) / float64(summary.TotalRules) * 100
    }

    return results, summary
}

func mapResultStatus(oscapResult string) string {
    switch oscapResult {
    case "pass":
        return "pass"
    case "fail":
        return "fail"
    case "informational":
        return "warn"
    case "notselected", "notapplicable":
        return "notapplicable"
    case "notchecked":
        return "skip"
    case "error":
        return "error"
    default:
        return "skip"
    }
}
```

---

## Docker Bench Integration

### Detection

Check for Docker and Docker Bench availability:

```go
// compliance/dockerbench.go

type DockerBenchDetection struct {
    DockerInstalled   bool   `json:"docker_installed"`
    DockerVersion     string `json:"docker_version"`
    DockerBenchMethod string `json:"docker_bench_method"` // "container" or "script"
}

func DetectDockerBench() (*DockerBenchDetection, error) {
    // Check if Docker is installed
    dockerPath, err := exec.LookPath("docker")
    if err != nil {
        return &DockerBenchDetection{DockerInstalled: false}, nil
    }

    // Get Docker version
    cmd := exec.Command(dockerPath, "version", "--format", "{{.Server.Version}}")
    output, _ := cmd.Output()
    version := strings.TrimSpace(string(output))

    // Docker Bench is run via container (preferred method)
    return &DockerBenchDetection{
        DockerInstalled:   true,
        DockerVersion:     version,
        DockerBenchMethod: "container",
    }, nil
}
```

### Scan Execution

Run Docker Bench for Security:

```go
func RunDockerBenchScan() (*ScanResult, error) {
    startedAt := time.Now()

    // Run Docker Bench via container with JSON output
    cmd := exec.Command("docker", "run", "--rm",
        "--net", "host",
        "--pid", "host",
        "--userns", "host",
        "--cap-add", "audit_control",
        "-e", "DOCKER_CONTENT_TRUST=$DOCKER_CONTENT_TRUST",
        "-v", "/etc:/etc:ro",
        "-v", "/lib/systemd/system:/lib/systemd/system:ro",
        "-v", "/usr/bin/containerd:/usr/bin/containerd:ro",
        "-v", "/usr/bin/runc:/usr/bin/runc:ro",
        "-v", "/usr/lib/systemd:/usr/lib/systemd:ro",
        "-v", "/var/lib:/var/lib:ro",
        "-v", "/var/run/docker.sock:/var/run/docker.sock:ro",
        "--label", "docker_bench_security",
        "docker/docker-bench-security",
        "-l", "/dev/stdout",  // JSON output to stdout
    )

    output, err := cmd.Output()
    completedAt := time.Now()

    if err != nil {
        return &ScanResult{
            ProfileName: "CIS Docker",
            ProfileType: "docker-bench",
            StartedAt:   startedAt,
            CompletedAt: completedAt,
            Status:      "failed",
            RawOutput:   string(output),
        }, err
    }

    // Parse JSON output
    results, summary := parseDockerBenchOutput(output)

    return &ScanResult{
        ProfileName:  "CIS Docker",
        ProfileType:  "docker-bench",
        Version:      "1.5.0",
        StartedAt:    startedAt,
        CompletedAt:  completedAt,
        Status:       "completed",
        Summary:      summary,
        Results:      results,
        RawOutput:    string(output),
    }, nil
}
```

### Docker Bench Output Parsing

Parse Docker Bench JSON output:

```go
type DockerBenchOutput struct {
    DockerBenchVersion string `json:"dockerbenchsec_version"`
    Start              int64  `json:"start"`
    Tests              []struct {
        ID     string `json:"id"`
        Desc   string `json:"desc"`
        Result string `json:"result"` // "PASS", "WARN", "INFO", "NOTE"
        Details string `json:"details,omitempty"`
        Items  []struct {
            ID     string `json:"id"`
            Desc   string `json:"desc"`
            Result string `json:"result"`
            Details string `json:"details,omitempty"`
            Remediation string `json:"remediation,omitempty"`
        } `json:"items,omitempty"`
    } `json:"tests"`
    End      int64 `json:"end"`
    Score    int   `json:"score"`
    Checks   int   `json:"checks"`
    Passed   int   `json:"passed"`
    Warnings int   `json:"warnings"`
    Info     int   `json:"info"`
}

func parseDockerBenchOutput(output []byte) ([]RuleResult, ScanSummary) {
    var bench DockerBenchOutput
    json.Unmarshal(output, &bench)

    results := []RuleResult{}
    summary := ScanSummary{
        TotalRules: bench.Checks,
        Passed:     bench.Passed,
        Warnings:   bench.Warnings,
        Score:      float64(bench.Passed) / float64(bench.Checks) * 100,
    }

    for _, test := range bench.Tests {
        for _, item := range test.Items {
            result := RuleResult{
                RuleRef:     item.ID,
                Title:       item.Desc,
                Section:     test.ID,
                Status:      mapDockerBenchStatus(item.Result),
                Finding:     item.Details,
                Remediation: item.Remediation,
                Severity:    inferSeverity(item.ID),
            }
            results = append(results, result)
        }
    }

    // Count failed (WARN is treated as fail for CIS purposes)
    for _, r := range results {
        if r.Status == "fail" {
            summary.Failed++
        }
    }

    return results, summary
}

func mapDockerBenchStatus(result string) string {
    switch result {
    case "PASS":
        return "pass"
    case "WARN":
        return "fail" // Docker Bench WARN = CIS fail
    case "INFO":
        return "warn"
    case "NOTE":
        return "skip"
    default:
        return "skip"
    }
}

func inferSeverity(ruleID string) string {
    // Infer severity from CIS section
    // Section 1: Host Configuration - High
    // Section 2: Docker daemon - High
    // Section 3: Docker daemon files - Medium
    // Section 4: Container Images - Medium
    // Section 5: Container Runtime - High
    // Section 6: Security Operations - Medium
    // Section 7: Swarm - Medium

    if strings.HasPrefix(ruleID, "1.") || strings.HasPrefix(ruleID, "2.") || strings.HasPrefix(ruleID, "5.") {
        return "high"
    }
    return "medium"
}
```

---

## Scheduling Configuration

### Agent Configuration

Add compliance settings to agent config.yml:

```yaml
# config.yml
server:
  url: "https://patchmon.example.com"

api:
  id: "patchmon_abc123"
  key: "your-api-key"

compliance:
  enabled: true

  openscap:
    enabled: true
    profile: "xccdf_org.ssgproject.content_profile_cis_level1_server"
    schedule: "0 3 * * 0"  # Weekly at 3 AM Sunday

  docker_bench:
    enabled: true
    schedule: "0 4 * * 0"  # Weekly at 4 AM Sunday
```

### Scheduler Implementation

```go
// compliance/scheduler.go

type ComplianceScheduler struct {
    config   *ComplianceConfig
    cron     *cron.Cron
    client   *APIClient
}

func NewComplianceScheduler(config *ComplianceConfig, client *APIClient) *ComplianceScheduler {
    return &ComplianceScheduler{
        config: config,
        cron:   cron.New(),
        client: client,
    }
}

func (s *ComplianceScheduler) Start() error {
    if !s.config.Enabled {
        return nil
    }

    if s.config.OpenSCAP.Enabled {
        s.cron.AddFunc(s.config.OpenSCAP.Schedule, func() {
            s.runOpenSCAPScan()
        })
    }

    if s.config.DockerBench.Enabled {
        s.cron.AddFunc(s.config.DockerBench.Schedule, func() {
            s.runDockerBenchScan()
        })
    }

    s.cron.Start()
    return nil
}

func (s *ComplianceScheduler) runOpenSCAPScan() {
    log.Info("Starting scheduled OpenSCAP compliance scan")

    result, err := RunOpenSCAPScan(ScanRequest{
        ProfileID: s.config.OpenSCAP.Profile,
    })

    if err != nil {
        log.Errorf("OpenSCAP scan failed: %v", err)
        return
    }

    // Submit results to server
    if err := s.client.SubmitComplianceScan(result); err != nil {
        log.Errorf("Failed to submit scan results: %v", err)
    }
}

func (s *ComplianceScheduler) runDockerBenchScan() {
    log.Info("Starting scheduled Docker Bench compliance scan")

    result, err := RunDockerBenchScan()
    if err != nil {
        log.Errorf("Docker Bench scan failed: %v", err)
        return
    }

    // Submit results to server
    if err := s.client.SubmitComplianceScan(result); err != nil {
        log.Errorf("Failed to submit scan results: %v", err)
    }
}
```

---

## On-Demand Execution via WebSocket

### WebSocket Message Handling

Handle `compliance_trigger` messages from server:

```go
// ws/handler.go

type ComplianceTriggerMessage struct {
    Type        string `json:"type"`    // "compliance_trigger"
    Profile     string `json:"profile"` // Profile name or "docker-bench"
    RequestID   string `json:"request_id"`
}

func (h *WSHandler) handleMessage(msg []byte) {
    var base struct {
        Type string `json:"type"`
    }
    json.Unmarshal(msg, &base)

    switch base.Type {
    case "compliance_trigger":
        var trigger ComplianceTriggerMessage
        json.Unmarshal(msg, &trigger)
        h.handleComplianceTrigger(trigger)
    // ... other message types
    }
}

func (h *WSHandler) handleComplianceTrigger(trigger ComplianceTriggerMessage) {
    log.Infof("Received compliance scan trigger for profile: %s", trigger.Profile)

    go func() {
        var result *ScanResult
        var err error

        if trigger.Profile == "docker-bench" || trigger.Profile == "CIS Docker" {
            result, err = RunDockerBenchScan()
        } else {
            // Find matching OpenSCAP profile
            detection, _ := DetectOpenSCAP()
            var profileInfo *ProfileInfo
            for _, p := range detection.AvailableProfiles {
                if p.Title == trigger.Profile || p.ID == trigger.Profile {
                    profileInfo = &p
                    break
                }
            }

            if profileInfo == nil {
                log.Errorf("Profile not found: %s", trigger.Profile)
                return
            }

            result, err = RunOpenSCAPScan(ScanRequest{
                ProfileID:   profileInfo.ID,
                ProfileName: profileInfo.Title,
                DataStream:  profileInfo.FilePath,
            })
        }

        if err != nil {
            log.Errorf("On-demand scan failed: %v", err)
            return
        }

        // Submit results
        if err := h.client.SubmitComplianceScan(result); err != nil {
            log.Errorf("Failed to submit scan results: %v", err)
        }
    }()
}
```

---

## JSON Output Format

### Scan Submission Payload

The agent submits scan results in this format to `POST /api/v1/compliance/scans`:

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
      "description": "The /tmp directory is a world-writable directory...",
      "rationale": "Placing /tmp on a separate partition enables...",
      "severity": "medium",
      "section": "1.1.1",
      "status": "fail",
      "finding": "/tmp is not a separate partition",
      "actual": "none",
      "expected": "separate partition",
      "remediation": "Create a separate /tmp partition during installation"
    }
  ],
  "raw_output": "... optional full scanner output ..."
}
```

---

## Prerequisites Detection API

The agent exposes compliance capability info via the regular update endpoint or a dedicated check:

```go
type ComplianceCapabilities struct {
    OpenSCAP    *OpenSCAPDetection    `json:"openscap,omitempty"`
    DockerBench *DockerBenchDetection `json:"docker_bench,omitempty"`
}

func GetComplianceCapabilities() *ComplianceCapabilities {
    caps := &ComplianceCapabilities{}

    if oscap, err := DetectOpenSCAP(); err == nil {
        caps.OpenSCAP = oscap
    }

    if db, err := DetectDockerBench(); err == nil {
        caps.DockerBench = db
    }

    return caps
}
```

This information can be included in the regular host update payload to inform the server what compliance capabilities are available on each host.

---

## Installation Requirements

### OpenSCAP Dependencies

For Ubuntu/Debian:
```bash
apt-get install -y openscap-scanner scap-security-guide
```

For RHEL/CentOS:
```bash
yum install -y openscap-scanner scap-security-guide
```

### Docker Bench Dependencies

No installation required - runs as a Docker container. Requires:
- Docker daemon running
- Access to Docker socket
- Agent running with privileges to run Docker commands
