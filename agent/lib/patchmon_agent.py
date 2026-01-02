#!/usr/bin/env python3
"""
PatchMon Agent - Python implementation with compliance scanning support.
Handles WebSocket communication with PatchMon server and compliance scans.
"""

import asyncio
import json
import logging
import os
import sys
import aiohttp
from datetime import datetime
from typing import Optional

from lib.compliance.openscap import OpenSCAPScanner, get_system_info
from lib.compliance.docker_bench import DockerBenchScanner

logger = logging.getLogger(__name__)

# Configuration
PATCHMON_SERVER = os.environ.get("PATCHMON_SERVER", "https://patchmon.example.com")
API_ID = os.environ.get("API_ID", "")
API_KEY = os.environ.get("API_KEY", "")
COMPLIANCE_SCAN_INTERVAL = int(os.environ.get("COMPLIANCE_SCAN_INTERVAL", "86400"))  # 24 hours
COMPLIANCE_ENABLED = os.environ.get("COMPLIANCE_ENABLED", "true").lower() == "true"


class PatchMonAgent:
    """PatchMon agent with compliance scanning support."""

    def __init__(self, server_url: str, api_id: str, api_key: str):
        self.server_url = server_url.rstrip("/")
        self.api_id = api_id
        self.api_key = api_key
        self.ws: Optional[aiohttp.ClientWebSocketResponse] = None
        self.session: Optional[aiohttp.ClientSession] = None
        self.running = False

    async def start(self):
        """Start the agent."""
        self.running = True
        self.session = aiohttp.ClientSession()

        try:
            # Start tasks
            await asyncio.gather(
                self.websocket_handler(),
                self.schedule_compliance_scans(),
            )
        except asyncio.CancelledError:
            pass
        finally:
            await self.stop()

    async def stop(self):
        """Stop the agent."""
        self.running = False
        if self.ws and not self.ws.closed:
            await self.ws.close()
        if self.session and not self.session.closed:
            await self.session.close()

    async def websocket_handler(self):
        """Handle WebSocket connection to PatchMon server."""
        ws_url = f"{self.server_url.replace('https', 'wss').replace('http', 'ws')}/api/v1/agents/ws"

        while self.running:
            try:
                headers = {
                    "X-API-ID": self.api_id,
                    "X-API-KEY": self.api_key,
                }

                async with self.session.ws_connect(ws_url, headers=headers) as ws:
                    self.ws = ws
                    logger.info("Connected to PatchMon server")

                    async for msg in ws:
                        if msg.type == aiohttp.WSMsgType.TEXT:
                            await self.handle_message(json.loads(msg.data))
                        elif msg.type == aiohttp.WSMsgType.ERROR:
                            logger.error(f"WebSocket error: {ws.exception()}")
                            break

            except aiohttp.ClientError as e:
                logger.error(f"WebSocket connection failed: {e}")
                await asyncio.sleep(30)  # Retry after 30 seconds

            except asyncio.CancelledError:
                break

    async def handle_message(self, data: dict):
        """Handle incoming WebSocket messages."""
        msg_type = data.get("type")

        if msg_type == "compliance_scan":
            profile_type = data.get("profile_type", "all")
            profile_name = data.get("profile_name", "level1_server")
            logger.info(f"Received compliance scan request: {profile_type}")

            if profile_type in ["openscap", "all"]:
                await self.run_openscap_compliance(profile_name)

            if profile_type in ["docker-bench", "all"]:
                await self.run_docker_bench_compliance()

        elif msg_type == "ping":
            if self.ws and not self.ws.closed:
                await self.ws.send_json({"type": "pong"})

        else:
            logger.debug(f"Unhandled message type: {msg_type}")

    async def run_openscap_compliance(self, profile_name: str = "level1_server"):
        """Run OpenSCAP scan and submit results."""
        try:
            scanner = OpenSCAPScanner()

            if not scanner.is_available():
                logger.warning("OpenSCAP not available, skipping scan")
                return

            logger.info(f"Starting OpenSCAP scan with profile: {profile_name}")
            result = scanner.run_scan(profile_name)

            if result["status"] == "completed":
                # Submit results to server
                await self.submit_compliance_results(result)
                logger.info(f"OpenSCAP scan completed: {len(result['results'])} rules checked")
            else:
                logger.error(f"OpenSCAP scan failed: {result.get('error')}")

        except Exception as e:
            logger.error(f"Error running OpenSCAP scan: {e}")

    async def run_docker_bench_compliance(self):
        """Run Docker Bench scan and submit results."""
        try:
            scanner = DockerBenchScanner()

            if not scanner.is_available():
                logger.warning("Docker not available, skipping Docker Bench scan")
                return

            logger.info("Starting Docker Bench for Security scan...")
            result = scanner.run_scan()

            if result["status"] == "completed":
                # Submit results to server
                await self.submit_compliance_results(result)
                logger.info(f"Docker Bench scan completed: {len(result['results'])} checks")
            else:
                logger.error(f"Docker Bench scan failed: {result.get('error')}")

        except Exception as e:
            logger.error(f"Error running Docker Bench scan: {e}")

    async def submit_compliance_results(self, result: dict):
        """Submit compliance scan results to the server."""
        url = f"{self.server_url}/api/v1/compliance/scans"

        headers = {
            "X-API-ID": self.api_id,
            "X-API-KEY": self.api_key,
            "Content-Type": "application/json",
        }

        try:
            async with self.session.post(url, json=result, headers=headers) as resp:
                if resp.status == 200:
                    data = await resp.json()
                    logger.info(f"Scan results submitted: {data.get('message')}")
                else:
                    error = await resp.text()
                    logger.error(f"Failed to submit results: {resp.status} - {error}")

        except aiohttp.ClientError as e:
            logger.error(f"Failed to submit compliance results: {e}")

    async def schedule_compliance_scans(self):
        """Schedule periodic compliance scans."""
        if not COMPLIANCE_ENABLED:
            logger.info("Compliance scanning disabled")
            return

        while self.running:
            try:
                await asyncio.sleep(COMPLIANCE_SCAN_INTERVAL)

                # Run OpenSCAP if available
                oscap_scanner = OpenSCAPScanner()
                if oscap_scanner.is_available():
                    logger.info("Running scheduled OpenSCAP scan...")
                    await self.run_openscap_compliance()

                # Run Docker Bench if Docker is available
                docker_scanner = DockerBenchScanner()
                if docker_scanner.is_available():
                    logger.info("Running scheduled Docker Bench scan...")
                    await self.run_docker_bench_compliance()

            except asyncio.CancelledError:
                break
            except Exception as e:
                logger.error(f"Scheduled compliance scan error: {e}")


def load_credentials():
    """Load credentials from environment or credentials file."""
    global API_ID, API_KEY, PATCHMON_SERVER

    # Try credentials file
    credentials_file = "/etc/patchmon/credentials"
    if os.path.exists(credentials_file):
        try:
            with open(credentials_file) as f:
                for line in f:
                    line = line.strip()
                    if line.startswith("API_ID="):
                        API_ID = line.split("=", 1)[1].strip('"\'')
                    elif line.startswith("API_KEY="):
                        API_KEY = line.split("=", 1)[1].strip('"\'')
                    elif line.startswith("PATCHMON_SERVER="):
                        PATCHMON_SERVER = line.split("=", 1)[1].strip('"\'')
        except Exception as e:
            logger.warning(f"Failed to read credentials file: {e}")

    # Environment variables override file
    API_ID = os.environ.get("API_ID", API_ID)
    API_KEY = os.environ.get("API_KEY", API_KEY)
    PATCHMON_SERVER = os.environ.get("PATCHMON_SERVER", PATCHMON_SERVER)


async def main():
    """Main entry point."""
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
    )

    load_credentials()

    if not API_ID or not API_KEY:
        logger.error("API_ID and API_KEY must be set")
        sys.exit(1)

    logger.info(f"Starting PatchMon agent, connecting to {PATCHMON_SERVER}")

    # Log system info
    info = get_system_info()
    logger.info(f"OpenSCAP available: {info['oscap_available']}")
    if info['os_info']['family']:
        logger.info(f"OS: {info['os_info']['name']} {info['os_info']['version']}")
    if info['available_profiles']:
        logger.info(f"Available profiles: {[p['name'] for p in info['available_profiles']]}")

    # Log Docker Bench availability
    docker_scanner = DockerBenchScanner()
    logger.info(f"Docker Bench available: {docker_scanner.is_available()}")

    agent = PatchMonAgent(PATCHMON_SERVER, API_ID, API_KEY)
    await agent.start()


if __name__ == "__main__":
    asyncio.run(main())
