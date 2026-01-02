#!/usr/bin/env python3
"""
OpenSCAP scanner for PatchMon compliance.
Runs CIS benchmarks using oscap tool and SCAP Security Guide content.
"""

import json
import subprocess
import logging
import os
import re
import xml.etree.ElementTree as ET
from datetime import datetime
from typing import Optional, List, Dict
from pathlib import Path
import tempfile
import platform

logger = logging.getLogger(__name__)

class OpenSCAPScanner:
    """Runs OpenSCAP CIS benchmark scans and parses results."""

    # Common locations for SCAP content
    SCAP_CONTENT_PATHS = [
        "/usr/share/xml/scap/ssg/content",
        "/usr/share/scap-security-guide",
        "/usr/share/openscap/scap-content",
    ]

    # Profile mappings for different OS families
    PROFILE_MAP = {
        "ubuntu": {
            "22.04": {
                "datastream": "ssg-ubuntu2204-ds.xml",
                "profiles": {
                    "level1_server": "xccdf_org.ssgproject.content_profile_cis_level1_server",
                    "level2_server": "xccdf_org.ssgproject.content_profile_cis_level2_server",
                    "level1_workstation": "xccdf_org.ssgproject.content_profile_cis_level1_workstation",
                }
            },
            "20.04": {
                "datastream": "ssg-ubuntu2004-ds.xml",
                "profiles": {
                    "level1_server": "xccdf_org.ssgproject.content_profile_cis_level1_server",
                    "level2_server": "xccdf_org.ssgproject.content_profile_cis_level2_server",
                }
            }
        },
        "debian": {
            "11": {
                "datastream": "ssg-debian11-ds.xml",
                "profiles": {
                    "level1_server": "xccdf_org.ssgproject.content_profile_cis_level1_server",
                    "level2_server": "xccdf_org.ssgproject.content_profile_cis_level2_server",
                }
            },
            "12": {
                "datastream": "ssg-debian12-ds.xml",
                "profiles": {
                    "level1_server": "xccdf_org.ssgproject.content_profile_cis_level1_server",
                }
            }
        },
        "rhel": {
            "8": {
                "datastream": "ssg-rhel8-ds.xml",
                "profiles": {
                    "level1_server": "xccdf_org.ssgproject.content_profile_cis",
                    "level2_server": "xccdf_org.ssgproject.content_profile_cis_server_l1",
                }
            },
            "9": {
                "datastream": "ssg-rhel9-ds.xml",
                "profiles": {
                    "level1_server": "xccdf_org.ssgproject.content_profile_cis",
                }
            }
        },
        "centos": {
            "8": {
                "datastream": "ssg-centos8-ds.xml",
                "profiles": {
                    "level1_server": "xccdf_org.ssgproject.content_profile_cis",
                }
            }
        },
        "rocky": {
            "8": {
                "datastream": "ssg-rl8-ds.xml",
                "profiles": {
                    "level1_server": "xccdf_org.ssgproject.content_profile_cis",
                }
            },
            "9": {
                "datastream": "ssg-rl9-ds.xml",
                "profiles": {
                    "level1_server": "xccdf_org.ssgproject.content_profile_cis",
                }
            }
        }
    }

    def __init__(self):
        self.oscap_available = self._check_oscap()
        self.scap_content_path = self._find_scap_content()
        self.os_info = self._detect_os()

    def _check_oscap(self) -> bool:
        """Check if oscap is installed."""
        try:
            result = subprocess.run(
                ["oscap", "--version"],
                capture_output=True,
                text=True,
                timeout=10
            )
            return result.returncode == 0
        except (subprocess.TimeoutExpired, FileNotFoundError):
            return False

    def _find_scap_content(self) -> Optional[str]:
        """Find SCAP Security Guide content directory."""
        for path in self.SCAP_CONTENT_PATHS:
            if os.path.isdir(path):
                return path
        return None

    def _detect_os(self) -> Dict[str, str]:
        """Detect the operating system family and version."""
        os_info = {
            "family": None,
            "version": None,
            "name": None
        }

        # Try /etc/os-release first
        try:
            with open("/etc/os-release") as f:
                content = f.read()

            for line in content.split('\n'):
                if line.startswith("ID="):
                    os_id = line.split('=')[1].strip('"').lower()
                    # Map common IDs to our family names
                    family_map = {
                        "ubuntu": "ubuntu",
                        "debian": "debian",
                        "rhel": "rhel",
                        "centos": "centos",
                        "rocky": "rocky",
                        "almalinux": "rhel",
                        "fedora": "rhel",
                    }
                    os_info["family"] = family_map.get(os_id, os_id)
                    os_info["name"] = os_id

                elif line.startswith("VERSION_ID="):
                    version = line.split('=')[1].strip('"')
                    # Get major version
                    os_info["version"] = version.split('.')[0] if '.' in version else version

        except FileNotFoundError:
            pass

        return os_info

    def is_available(self) -> bool:
        """Check if OpenSCAP scanning is available."""
        return self.oscap_available and self.scap_content_path is not None

    def get_available_profiles(self) -> List[Dict]:
        """Get list of available profiles for this system."""
        profiles = []

        if not self.os_info["family"] or not self.os_info["version"]:
            return profiles

        os_profiles = self.PROFILE_MAP.get(self.os_info["family"], {})
        version_info = os_profiles.get(self.os_info["version"], {})

        if version_info:
            datastream = version_info.get("datastream")
            datastream_path = os.path.join(self.scap_content_path, datastream) if self.scap_content_path else None

            if datastream_path and os.path.exists(datastream_path):
                for profile_name, profile_id in version_info.get("profiles", {}).items():
                    profiles.append({
                        "name": profile_name,
                        "id": profile_id,
                        "datastream": datastream_path,
                        "os_family": self.os_info["family"],
                        "os_version": self.os_info["version"]
                    })

        return profiles

    def run_scan(self, profile_name: str = "level1_server") -> dict:
        """
        Run OpenSCAP scan with specified profile.
        Returns parsed results in PatchMon format.
        """
        if not self.is_available():
            return {
                "status": "error",
                "error": "OpenSCAP is not available",
                "results": []
            }

        profiles = self.get_available_profiles()
        profile = next((p for p in profiles if p["name"] == profile_name), None)

        if not profile:
            available = [p["name"] for p in profiles]
            return {
                "status": "error",
                "error": f"Profile '{profile_name}' not available. Available: {available}",
                "results": []
            }

        started_at = datetime.utcnow().isoformat() + "Z"

        try:
            with tempfile.TemporaryDirectory() as tmpdir:
                results_xml = os.path.join(tmpdir, "results.xml")
                report_html = os.path.join(tmpdir, "report.html")

                logger.info(f"Running OpenSCAP scan with profile: {profile['id']}")

                # Run oscap scan
                cmd = [
                    "oscap", "xccdf", "eval",
                    "--profile", profile["id"],
                    "--results", results_xml,
                    "--report", report_html,
                    profile["datastream"]
                ]

                result = subprocess.run(
                    cmd,
                    capture_output=True,
                    text=True,
                    timeout=1800  # 30 minute timeout
                )

                completed_at = datetime.utcnow().isoformat() + "Z"

                # Parse results XML
                results = self._parse_results_xml(results_xml)

                # Determine profile display name
                os_name = f"{self.os_info['name'].title()} {self.os_info['version']}"
                profile_display = f"CIS {os_name} {profile_name.replace('_', ' ').title()}"

                return {
                    "status": "completed",
                    "profile_name": profile_display,
                    "profile_type": "openscap",
                    "started_at": started_at,
                    "completed_at": completed_at,
                    "results": results,
                    "raw_output": result.stdout + result.stderr
                }

        except subprocess.TimeoutExpired:
            return {
                "status": "error",
                "error": "OpenSCAP scan timed out",
                "results": []
            }
        except Exception as e:
            logger.error(f"OpenSCAP scan failed: {e}")
            return {
                "status": "error",
                "error": str(e),
                "results": []
            }

    def _parse_results_xml(self, results_file: str) -> List[Dict]:
        """Parse OpenSCAP results XML into structured format."""
        results = []

        if not os.path.exists(results_file):
            return results

        try:
            tree = ET.parse(results_file)
            root = tree.getroot()

            # Handle XML namespaces
            ns = {
                'xccdf': 'http://checklists.nist.gov/xccdf/1.2',
                'xccdf11': 'http://checklists.nist.gov/xccdf/1.1'
            }

            # Find all rule-result elements
            for rule_result in root.iter():
                if 'rule-result' in rule_result.tag:
                    rule_id = rule_result.get('idref', '')

                    # Get result status
                    result_elem = rule_result.find('.//{http://checklists.nist.gov/xccdf/1.2}result')
                    if result_elem is None:
                        result_elem = rule_result.find('.//{http://checklists.nist.gov/xccdf/1.1}result')

                    if result_elem is not None:
                        status_raw = result_elem.text.lower()
                    else:
                        status_raw = "unknown"

                    # Map OpenSCAP status to our standard
                    status_map = {
                        "pass": "pass",
                        "fail": "fail",
                        "error": "error",
                        "unknown": "skip",
                        "notapplicable": "notapplicable",
                        "notchecked": "skip",
                        "notselected": "skip",
                        "informational": "skip",
                        "fixed": "pass"
                    }

                    status = status_map.get(status_raw, "skip")

                    # Try to get rule title and other info
                    title = self._get_rule_title(root, rule_id, ns)
                    severity = self._get_rule_severity(root, rule_id, ns)

                    # Extract section number from rule ID if possible
                    section_match = re.search(r'_(\d+(?:_\d+)+)_', rule_id)
                    section = section_match.group(1).replace('_', '.') if section_match else None

                    results.append({
                        "rule_ref": rule_id,
                        "title": title or rule_id,
                        "status": status,
                        "severity": severity,
                        "section": section,
                        "description": None,
                        "finding": f"Rule {status_raw}"
                    })

        except ET.ParseError as e:
            logger.error(f"Failed to parse OpenSCAP results: {e}")

        return results

    def _get_rule_title(self, root: ET.Element, rule_id: str, ns: dict) -> Optional[str]:
        """Extract rule title from XCCDF document."""
        for rule in root.iter():
            if 'Rule' in rule.tag and rule.get('id') == rule_id:
                for title in rule.iter():
                    if 'title' in title.tag:
                        return title.text
        return None

    def _get_rule_severity(self, root: ET.Element, rule_id: str, ns: dict) -> str:
        """Extract rule severity from XCCDF document."""
        for rule in root.iter():
            if 'Rule' in rule.tag and rule.get('id') == rule_id:
                severity = rule.get('severity', 'medium')
                return severity.lower()
        return "medium"


def run_openscap_scan(profile: str = "level1_server") -> dict:
    """Convenience function to run an OpenSCAP scan."""
    scanner = OpenSCAPScanner()
    return scanner.run_scan(profile)


def get_system_info() -> dict:
    """Get system information for compliance reporting."""
    scanner = OpenSCAPScanner()
    return {
        "oscap_available": scanner.oscap_available,
        "scap_content_path": scanner.scap_content_path,
        "os_info": scanner.os_info,
        "available_profiles": scanner.get_available_profiles()
    }


if __name__ == "__main__":
    # Test run
    logging.basicConfig(level=logging.INFO)

    print("System Info:")
    info = get_system_info()
    print(json.dumps(info, indent=2))

    if info["oscap_available"]:
        print("\nRunning scan...")
        result = run_openscap_scan()
        print(json.dumps(result, indent=2))
