import { describe, expect, it } from "vitest";
import {
	hostIsFreeBSD,
	hostIsWindows,
	showsSELinuxStatus,
} from "../../utils/hostPlatform";

describe("hostIsWindows / hostIsFreeBSD", () => {
	it("detects Windows from os_type or expected_platform", () => {
		expect(hostIsWindows({ os_type: "Windows Server 2022" })).toBe(true);
		expect(hostIsWindows({ expected_platform: "windows" })).toBe(true);
		expect(hostIsWindows({ os_type: "Ubuntu" })).toBe(false);
		expect(hostIsWindows(undefined)).toBe(false);
	});

	it("detects FreeBSD from os_type, expected_platform or pkg", () => {
		expect(hostIsFreeBSD({ os_type: "FreeBSD" })).toBe(true);
		expect(hostIsFreeBSD({ expected_platform: "freebsd" })).toBe(true);
		expect(hostIsFreeBSD({ os_type: "", package_manager: "pkg" })).toBe(true);
		expect(
			hostIsFreeBSD({ os_type: "Rocky Linux", package_manager: "dnf" }),
		).toBe(false);
	});
});

describe("showsSELinuxStatus", () => {
	it("shows the reported status on Linux hosts", () => {
		expect(
			showsSELinuxStatus({ os_type: "Rocky Linux", selinux_status: "enabled" }),
		).toBe(true);
		expect(
			showsSELinuxStatus({ os_type: "Ubuntu", selinux_status: "disabled" }),
		).toBe(true);
	});

	it("hides it on Windows and FreeBSD hosts, which always report disabled", () => {
		expect(
			showsSELinuxStatus({
				os_type: "Windows Server 2022",
				selinux_status: "disabled",
			}),
		).toBe(false);
		expect(
			showsSELinuxStatus({ os_type: "FreeBSD", selinux_status: "disabled" }),
		).toBe(false);
		expect(
			showsSELinuxStatus({
				package_manager: "pkg",
				selinux_status: "disabled",
			}),
		).toBe(false);
	});

	it("hides it when nothing is reported", () => {
		expect(showsSELinuxStatus({ os_type: "Ubuntu" })).toBe(false);
		expect(showsSELinuxStatus(null)).toBe(false);
	});
});
