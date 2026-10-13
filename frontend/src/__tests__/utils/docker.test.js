import { describe, expect, it } from "vitest";
import {
	generateRegistryLink,
	getUpdateStatusMeta,
	shortDigest,
	shortImageId,
} from "../../utils/docker";

describe("generateRegistryLink", () => {
	it.each([
		["nginx", "docker-hub", "https://hub.docker.com/_/nginx"],
		["library/nginx", "docker-hub", "https://hub.docker.com/_/nginx"],
		[
			"portainer/portainer-ce",
			"docker-hub",
			"https://hub.docker.com/r/portainer/portainer-ce",
		],
		["docker.io/library/redis", "docker-hub", "https://hub.docker.com/_/redis"],
		[
			"ghcr.io/home-assistant/home-assistant",
			"github",
			"https://github.com/home-assistant/home-assistant/pkgs/container/home-assistant",
		],
		[
			"ghcr.io/home-assistant/home-assistant",
			"docker-hub",
			"https://github.com/home-assistant/home-assistant/pkgs/container/home-assistant",
		],
		[
			"quay.io/prometheus/node-exporter",
			"quay",
			"https://quay.io/repository/prometheus/node-exporter",
		],
		[
			"registry.gitlab.com/group/project",
			"gitlab",
			"https://gitlab.com/group/project/container_registry",
		],
		[
			"gcr.io/distroless/static",
			"google",
			"https://console.cloud.google.com/gcr/images/distroless/static",
		],
		[
			"registry.example.com/team/app",
			"private",
			"https://registry.example.com",
		],
	])("%s (%s)", (repository, source, expected) => {
		expect(generateRegistryLink(repository, source)).toBe(expected);
	});

	it.each([
		["localhost:5000/team/app", "private"],
		["localhost/team/app", "private"],
		["10.0.0.5:5000/app", "private"],
		["myregistry:5000/app", "private"],
		["local/myapp", "local"],
		["nginx", "local"],
		["<none>", "unknown"],
		["", "docker-hub"],
	])("returns null for %s (%s)", (repository, source) => {
		expect(generateRegistryLink(repository, source)).toBeNull();
	});

	it("does not treat a lookalike host as Google", () => {
		expect(generateRegistryLink("evil-gcr.io/x/y", "google")).toBe(
			"https://evil-gcr.io",
		);
	});
});

describe("shortImageId / shortDigest", () => {
	it("strips the sha256 prefix from image IDs", () => {
		expect(shortImageId(`sha256:${"a".repeat(64)}`)).toBe("a".repeat(12));
		expect(shortImageId("17868f7231bcdeadbeef")).toBe("17868f7231bc");
		expect(shortImageId(undefined)).toBe("");
	});

	it("keeps the sha256 prefix on digests", () => {
		expect(shortDigest(`sha256:${"b".repeat(64)}`)).toBe(
			`sha256:${"b".repeat(12)}`,
		);
		expect(shortDigest("")).toBe("");
	});
});

describe("getUpdateStatusMeta", () => {
	it.each([
		["update_available", "Update available", "badge-warning"],
		["recreate_needed", "Recreate needed", "badge-primary"],
		["up_to_date", "Up to date", "badge-success"],
		["unknown", "Unknown", "badge-secondary"],
		["local_build", "Local build", "badge-secondary"],
		["pinned", "Pinned digest", "badge-secondary"],
		["skipped", "Untagged", "badge-secondary"],
		["not_checked", "Not checked", "badge-secondary"],
	])("%s renders %s", (status, label, className) => {
		const meta = getUpdateStatusMeta(status);
		expect(meta.label).toBe(label);
		expect(meta.className).toBe(className);
	});

	it.each([
		undefined,
		null,
		"",
		"hasUpdates",
		"constructor",
		"__proto__",
	])("never claims up to date for %s", (status) => {
		const meta = getUpdateStatusMeta(status);
		expect(meta.status).toBe("not_checked");
		expect(meta.label).toBe("Not checked");
	});

	it("explains unknown results with the reason", () => {
		expect(
			getUpdateStatusMeta("unknown", { reason: "rate_limited" }).tooltip,
		).toMatch(/rate limited/);
		expect(getUpdateStatusMeta("unknown", { reason: "weird" }).tooltip).toBe(
			"The update check did not complete.",
		);
	});

	it("puts the check time in the up to date tooltip", () => {
		const meta = getUpdateStatusMeta("up_to_date", {
			checkedAt: "2026-10-13T12:00:00Z",
			formatTime: () => "5 min ago",
		});
		expect(meta.tooltip).toBe("Checked 5 min ago");
		expect(getUpdateStatusMeta("up_to_date").tooltip).toBe("");
	});
});
