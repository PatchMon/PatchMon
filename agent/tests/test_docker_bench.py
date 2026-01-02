import unittest
from unittest.mock import patch, MagicMock
import json

from lib.compliance.docker_bench import DockerBenchScanner, run_docker_bench_scan


class TestDockerBenchScanner(unittest.TestCase):

    def test_docker_not_available(self):
        """Test behavior when Docker is not available."""
        with patch('subprocess.run') as mock_run:
            mock_run.side_effect = FileNotFoundError()
            scanner = DockerBenchScanner()
            self.assertFalse(scanner.is_available())

    def test_docker_available(self):
        """Test detection of Docker availability."""
        with patch('subprocess.run') as mock_run:
            mock_run.return_value = MagicMock(returncode=0)
            scanner = DockerBenchScanner()
            self.assertTrue(scanner.is_available())

    def test_parse_output(self):
        """Test parsing of Docker Bench output."""
        sample_output = """
[INFO] 1 - Host Configuration
[PASS] 1.1.1 - Ensure a separate partition for containers has been created
[WARN] 1.1.2 - Ensure only trusted users are allowed to control Docker daemon
[INFO] 2 - Docker daemon configuration
[PASS] 2.1 - Run the Docker daemon as a non-root user, if possible
[NOTE] 2.2 - Ensure network traffic is restricted between containers on the default bridge
"""
        scanner = DockerBenchScanner()
        scanner.docker_available = True  # Force available for test

        results = scanner._parse_output(sample_output)

        self.assertEqual(len(results), 4)
        self.assertEqual(results[0]["rule_ref"], "1.1.1")
        self.assertEqual(results[0]["status"], "pass")
        self.assertEqual(results[1]["status"], "fail")  # WARN maps to fail
        self.assertEqual(results[3]["status"], "notapplicable")  # NOTE maps to notapplicable

    def test_severity_mapping(self):
        """Test severity assignment by section."""
        scanner = DockerBenchScanner()

        self.assertEqual(scanner._get_severity("1.1.1"), "high")
        self.assertEqual(scanner._get_severity("2.5"), "high")
        self.assertEqual(scanner._get_severity("3.1"), "medium")
        self.assertEqual(scanner._get_severity("5.1"), "high")
        self.assertEqual(scanner._get_severity("7.1"), "low")


if __name__ == "__main__":
    unittest.main()
