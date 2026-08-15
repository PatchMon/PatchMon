import { describe, expect, it } from "vitest";
import {
	detectWebhookFormat,
	WEBHOOK_FORMATS,
	webhookFormatLabel,
} from "../../utils/webhookFormat";

// Mirrors TestIsSlackCompatibleWebhookURL in
// server-source-code/internal/queue/notification_webhook_body_test.go.
// Keep the two tables in step.
const MATTERMOST_TOKEN = "873nng3nmpbpxrfz8wtjzge8oa";
const ROCKETCHAT_ID = "abcdefghij1234567";
const ROCKETCHAT_TOKEN = "aBcDeF1234567890aBcDeF1234567890aBcDeF1234567890";

describe("detectWebhookFormat", () => {
	it.each([
		["discord", "https://discord.com/api/webhooks/123/tok"],
		["discord app domain", "https://discordapp.com/api/webhooks/123/tok"],
		["discord www", "https://www.discord.com/api/webhooks/123/tok"],
	])("%s -> discord", (_name, url) => {
		expect(detectWebhookFormat(url)).toBe(WEBHOOK_FORMATS.DISCORD);
	});

	it.each([
		["slack", "https://hooks.slack.com/services/T00000000/B00000000/abcdefgh"],
	])("%s -> slack", (_name, url) => {
		expect(detectWebhookFormat(url)).toBe(WEBHOOK_FORMATS.SLACK);
	});

	it.each([
		["mattermost", `https://chat.example.com/hooks/${MATTERMOST_TOKEN}`],
		[
			"subpath proxy",
			`https://example.com/mattermost/hooks/${MATTERMOST_TOKEN}`,
		],
		["trailing slash", `https://chat.example.com/hooks/${MATTERMOST_TOKEN}/`],
		[
			"explicit port",
			`https://chat.example.com:8065/hooks/${MATTERMOST_TOKEN}`,
		],
		["plain http", `http://chat.example.com/hooks/${MATTERMOST_TOKEN}`],
		["uppercase segment", `https://chat.example.com/Hooks/${MATTERMOST_TOKEN}`],
		[
			"rocket.chat",
			`https://chat.example.com/hooks/${ROCKETCHAT_ID}/${ROCKETCHAT_TOKEN}`,
		],
		[
			"query string ignored",
			`https://c.example.com/hooks/${MATTERMOST_TOKEN}?x=1`,
		],
		["fragment ignored", `https://c.example.com/hooks/${MATTERMOST_TOKEN}#f`],
	])("%s -> slack compatible", (_name, url) => {
		expect(detectWebhookFormat(url)).toBe(WEBHOOK_FORMATS.SLACK_COMPATIBLE);
	});

	it.each([
		["no token after hooks", "https://chat.example.com/hooks"],
		["empty token", "https://chat.example.com/hooks/"],
		["token too short", "https://chat.example.com/hooks/abc123"],
		[
			"more than two trailing segments",
			`https://chat.example.com/hooks/${MATTERMOST_TOKEN}/a/b`,
		],
		["zapier catch hook", "https://hooks.zapier.com/hooks/catch/123456/abcdef"],
		[
			"atlassian automation",
			"https://automation.atlassian.com/pro/hooks/1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d",
		],
		[
			"atlassian legacy domain",
			"https://automation.codebarrel.io/pro/hooks/1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d",
		],
		[
			"hostname alone does not count",
			`https://hooks.example.com/ingest/${MATTERMOST_TOKEN}`,
		],
		[
			"workato plural webhooks",
			`https://www.workato.com/webhooks/rest/${MATTERMOST_TOKEN}`,
		],
		[
			"n8n singular webhook",
			`https://n8n.example.com/webhook/${MATTERMOST_TOKEN}`,
		],
		[
			"percent-encoded separator is not a segment break",
			`https://chat.example.com/hooks%2F${MATTERMOST_TOKEN}`,
		],
		["unrelated endpoint", "https://example.com/api/notify"],
	])("%s -> generic", (_name, url) => {
		expect(detectWebhookFormat(url)).toBe(WEBHOOK_FORMATS.GENERIC);
	});

	it.each([
		["empty", ""],
		["whitespace only", "   "],
		["null", null],
		["undefined", undefined],
		["half typed", "https://"],
		[
			"scheme-less, cannot be delivered",
			`chat.example.com/hooks/${MATTERMOST_TOKEN}`,
		],
		["non-http scheme", `ftp://chat.example.com/hooks/${MATTERMOST_TOKEN}`],
	])("%s -> no hint", (_name, url) => {
		expect(detectWebhookFormat(url)).toBeNull();
	});

	it("trims surrounding whitespace from a pasted URL", () => {
		expect(
			detectWebhookFormat(
				`  https://c.example.com/hooks/${MATTERMOST_TOKEN}  `,
			),
		).toBe(WEBHOOK_FORMATS.SLACK_COMPATIBLE);
	});
});

describe("webhookFormatLabel", () => {
	it("names every format", () => {
		for (const format of Object.values(WEBHOOK_FORMATS)) {
			const meta = webhookFormatLabel(format);
			expect(meta?.label).toBeTruthy();
			expect(meta?.detail).toBeTruthy();
		}
	});

	it("returns null for an unknown format", () => {
		expect(webhookFormatLabel("nope")).toBeNull();
		expect(webhookFormatLabel(null)).toBeNull();
	});
});
