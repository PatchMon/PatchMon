#!/usr/bin/env python3
"""
Docker Bench for Security scanner for PatchMon compliance.
Runs the CIS Docker Benchmark via docker-bench-security container.
"""

import json
import subprocess
import logging
import re
from datetime import datetime
from typing import Optional
import os
import tempfile

logger = logging.getLogger(__name__)


class DockerBenchScanner:
    """Runs Docker Bench for Security and parses results."""

    DOCKER_BENCH_IMAGE = "docker/docker-bench-security:latest"

    def __init__(self):
        self.docker_available = self._check_docker()

    def _check_docker(self) -> bool:
        """Check if Docker is available and running."""
        try:
            result = subprocess.run(
                ["docker", "info"],
                capture_output=True,
                text=True,
                timeout=10
            )
            return result.returncode == 0
        except (subprocess.TimeoutExpired, FileNotFoundError):
            return False

    def is_available(self) -> bool:
        """Check if Docker Bench can be run."""
        return self.docker_available

    def run_scan(self) -> dict:
        """
        Run Docker Bench for Security scan.
        Returns parsed results in PatchMon format.
        """
        if not self.docker_available:
            return {
                "status": "error",
                "error": "Docker is not available",
                "results": []
            }

        started_at = datetime.utcnow().isoformat() + "Z"

        try:
            # Pull latest docker-bench-security image
            logger.info("Pulling docker-bench-security image...")
            subprocess.run(
                ["docker", "pull", self.DOCKER_BENCH_IMAGE],
                capture_output=True,
                timeout=300
            )

            # Run docker-bench-security with JSON output
            logger.info("Running Docker Bench for Security...")
            result = subprocess.run(
                [
                    "docker", "run", "--rm",
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
                    self.DOCKER_BENCH_IMAGE,
                    "-l", "/tmp/docker-bench.log"
                ],
                capture_output=True,
                text=True,
                timeout=600
            )

            completed_at = datetime.utcnow().isoformat() + "Z"
            raw_output = result.stdout + result.stderr

            # Parse the output
            results = self._parse_output(raw_output)

            return {
                "status": "completed",
                "profile_name": "CIS Docker Benchmark",
                "profile_type": "docker-bench",
                "started_at": started_at,
                "completed_at": completed_at,
                "results": results,
                "raw_output": raw_output
            }

        except subprocess.TimeoutExpired:
            return {
                "status": "error",
                "error": "Docker Bench scan timed out",
                "results": []
            }
        except Exception as e:
            logger.error(f"Docker Bench scan failed: {e}")
            return {
                "status": "error",
                "error": str(e),
                "results": []
            }

    def _parse_output(self, output: str) -> list:
        """
        Parse Docker Bench output into structured results.
        """
        results = []
        current_section = ""

        # Regex patterns for parsing
        section_pattern = re.compile(r'^\[INFO\]\s+(\d+(?:\.\d+)*)\s+-\s+(.+)$')
        check_pattern = re.compile(r'^\[(PASS|WARN|INFO|NOTE)\]\s+(\d+(?:\.\d+)*)\s+-\s+(.+)$')

        for line in output.split('\n'):
            line = line.strip()

            # Check for section headers
            section_match = section_pattern.match(line)
            if section_match and '.' not in section_match.group(1):
                current_section = section_match.group(1)
                continue

            # Check for individual test results
            check_match = check_pattern.match(line)
            if check_match:
                status_raw = check_match.group(1)
                rule_ref = check_match.group(2)
                title = check_match.group(3)

                # Map Docker Bench status to our standard statuses
                status_map = {
                    "PASS": "pass",
                    "WARN": "fail",  # WARN in Docker Bench means failed check
                    "INFO": "skip",
                    "NOTE": "notapplicable"
                }

                # Determine severity based on section
                severity = self._get_severity(rule_ref)

                results.append({
                    "rule_ref": rule_ref,
                    "title": title,
                    "status": status_map.get(status_raw, "skip"),
                    "severity": severity,
                    "section": current_section,
                    "description": f"CIS Docker Benchmark check {rule_ref}",
                    "finding": f"Check {status_raw}: {title}"
                })

        return results

    def _get_severity(self, rule_ref: str) -> str:
        """
        Determine severity based on CIS Docker Benchmark section.
        """
        # Host Configuration (Section 1) - High
        # Docker daemon configuration (Section 2) - High
        # Docker daemon configuration files (Section 3) - Medium
        # Container Images and Build File (Section 4) - Medium
        # Container Runtime (Section 5) - High
        # Docker Security Operations (Section 6) - Medium
        # Docker Swarm Configuration (Section 7) - Low

        section = rule_ref.split('.')[0] if '.' in rule_ref else rule_ref

        severity_map = {
            "1": "high",
            "2": "high",
            "3": "medium",
            "4": "medium",
            "5": "high",
            "6": "medium",
            "7": "low"
        }

        return severity_map.get(section, "medium")


def run_docker_bench_scan() -> dict:
    """Convenience function to run a Docker Bench scan."""
    scanner = DockerBenchScanner()
    return scanner.run_scan()


if __name__ == "__main__":
    # Test run
    logging.basicConfig(level=logging.INFO)
    result = run_docker_bench_scan()
    print(json.dumps(result, indent=2))
