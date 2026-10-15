import { SiLinux } from "react-icons/si";
import { describe, expect, it } from "vitest";
import { getOSDisplayName, getOSIcon } from "../../utils/osIcons";

describe("Univention Corporate Server", () => {
	it("is named from the os_type the agent reports", () => {
		expect(getOSDisplayName("Univention Corporate Server")).toBe(
			"Univention Corporate Server",
		);
	});

	it("uses the generic Linux icon", () => {
		expect(getOSIcon("Univention Corporate Server")).toBe(SiLinux);
	});
});
