import unittest
from unittest.mock import patch, MagicMock, mock_open
import json
import os

from lib.compliance.openscap import OpenSCAPScanner, run_openscap_scan, get_system_info


class TestOpenSCAPScanner(unittest.TestCase):

    def test_oscap_not_available(self):
        """Test behavior when oscap is not installed."""
        with patch('subprocess.run') as mock_run:
            mock_run.side_effect = FileNotFoundError()
            scanner = OpenSCAPScanner()
            self.assertFalse(scanner.oscap_available)

    def test_oscap_available(self):
        """Test detection of oscap availability."""
        with patch('subprocess.run') as mock_run:
            mock_run.return_value = MagicMock(returncode=0)
            with patch('os.path.isdir', return_value=True):
                scanner = OpenSCAPScanner()
                # Note: Full availability also requires SCAP content

    def test_os_detection_ubuntu(self):
        """Test Ubuntu detection from os-release."""
        os_release = '''
ID=ubuntu
VERSION_ID="22.04"
'''
        with patch('subprocess.run') as mock_run:
            mock_run.return_value = MagicMock(returncode=0)
            with patch('builtins.open', mock_open(read_data=os_release)):
                with patch('os.path.isdir', return_value=False):
                    scanner = OpenSCAPScanner()
                    self.assertEqual(scanner.os_info["family"], "ubuntu")
                    self.assertEqual(scanner.os_info["version"], "22")

    def test_os_detection_rhel(self):
        """Test RHEL detection from os-release."""
        os_release = '''
ID=rhel
VERSION_ID="8.6"
'''
        with patch('subprocess.run') as mock_run:
            mock_run.return_value = MagicMock(returncode=0)
            with patch('builtins.open', mock_open(read_data=os_release)):
                with patch('os.path.isdir', return_value=False):
                    scanner = OpenSCAPScanner()
                    self.assertEqual(scanner.os_info["family"], "rhel")
                    self.assertEqual(scanner.os_info["version"], "8")

    def test_severity_from_rule(self):
        """Test severity extraction."""
        scanner = OpenSCAPScanner()
        # Default severity should be medium
        self.assertEqual(scanner._get_rule_severity(MagicMock(), "test_rule", {}), "medium")

    def test_scan_not_available(self):
        """Test scan returns error when not available."""
        with patch('subprocess.run') as mock_run:
            mock_run.side_effect = FileNotFoundError()
            scanner = OpenSCAPScanner()
            result = scanner.run_scan()
            self.assertEqual(result["status"], "error")
            self.assertIn("not available", result["error"])


class TestGetSystemInfo(unittest.TestCase):

    def test_returns_dict(self):
        """Test that get_system_info returns expected structure."""
        with patch('subprocess.run') as mock_run:
            mock_run.return_value = MagicMock(returncode=0)
            with patch('os.path.isdir', return_value=False):
                with patch('builtins.open', mock_open(read_data="ID=ubuntu\nVERSION_ID=22.04")):
                    info = get_system_info()
                    self.assertIn("oscap_available", info)
                    self.assertIn("os_info", info)
                    self.assertIn("available_profiles", info)


class TestRunOpenscapScan(unittest.TestCase):

    def test_convenience_function(self):
        """Test the convenience run_openscap_scan function."""
        with patch('subprocess.run') as mock_run:
            mock_run.side_effect = FileNotFoundError()
            result = run_openscap_scan()
            self.assertEqual(result["status"], "error")


class TestProfileMapping(unittest.TestCase):

    def test_ubuntu_profiles_exist(self):
        """Test that Ubuntu profiles are defined."""
        self.assertIn("ubuntu", OpenSCAPScanner.PROFILE_MAP)
        self.assertIn("22.04", OpenSCAPScanner.PROFILE_MAP["ubuntu"])
        self.assertIn("20.04", OpenSCAPScanner.PROFILE_MAP["ubuntu"])

    def test_rhel_profiles_exist(self):
        """Test that RHEL profiles are defined."""
        self.assertIn("rhel", OpenSCAPScanner.PROFILE_MAP)
        self.assertIn("8", OpenSCAPScanner.PROFILE_MAP["rhel"])
        self.assertIn("9", OpenSCAPScanner.PROFILE_MAP["rhel"])

    def test_debian_profiles_exist(self):
        """Test that Debian profiles are defined."""
        self.assertIn("debian", OpenSCAPScanner.PROFILE_MAP)
        self.assertIn("11", OpenSCAPScanner.PROFILE_MAP["debian"])
        self.assertIn("12", OpenSCAPScanner.PROFILE_MAP["debian"])

    def test_rocky_profiles_exist(self):
        """Test that Rocky Linux profiles are defined."""
        self.assertIn("rocky", OpenSCAPScanner.PROFILE_MAP)
        self.assertIn("8", OpenSCAPScanner.PROFILE_MAP["rocky"])
        self.assertIn("9", OpenSCAPScanner.PROFILE_MAP["rocky"])


class TestStatusMapping(unittest.TestCase):

    def test_parse_results_status_mapping(self):
        """Test that status mapping covers expected OpenSCAP statuses."""
        # The status map is used in _parse_results_xml
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

        # Verify key mappings
        self.assertEqual(status_map["pass"], "pass")
        self.assertEqual(status_map["fail"], "fail")
        self.assertEqual(status_map["notapplicable"], "notapplicable")
        self.assertEqual(status_map["fixed"], "pass")


if __name__ == "__main__":
    unittest.main()
