import { describe, expect, it } from "vitest";
import { getRemoteAccessTabs } from "../../utils/remoteAccessTabs";

describe("getRemoteAccessTabs", () => {
	it("hides Terminal and RDP from users without can_use_remote_access", () => {
		expect(
			getRemoteAccessTabs({ isWindowsHost: true, canUseRemoteAccess: false }),
		).toEqual({ terminal: false, rdp: false });
	});

	it("shows Terminal on any host and RDP only on Windows hosts", () => {
		expect(
			getRemoteAccessTabs({ isWindowsHost: false, canUseRemoteAccess: true }),
		).toEqual({ terminal: true, rdp: false });
		expect(
			getRemoteAccessTabs({ isWindowsHost: true, canUseRemoteAccess: true }),
		).toEqual({ terminal: true, rdp: true });
	});
});
