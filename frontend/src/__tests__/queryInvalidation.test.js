import fs from "node:fs";
import path from "node:path";
import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";

/**
 * TanStack Query v5 changed invalidateQueries from the v4 positional
 * `invalidateQueries(queryKey)` to `invalidateQueries(filters, options)`, where
 * filters is an OBJECT.
 *
 * Passing a bare array means `filters.queryKey` is undefined, and a filter with
 * no queryKey matches EVERY query in the cache. Every such call therefore
 * invalidated and refetched the whole active cache instead of the one key it
 * named. On top of the refetch storms, this is what made the "unsaved edits get
 * wiped" bugs fire so readily: a mutation anywhere on a page refetched the
 * query feeding an unrelated form on the same page.
 */
describe("invalidateQueries filter semantics", () => {
	const seed = async () => {
		const qc = new QueryClient({
			defaultOptions: { queries: { retry: false } },
		});
		await qc.prefetchQuery({ queryKey: ["hosts"], queryFn: () => "hosts" });
		await qc.prefetchQuery({
			queryKey: ["settings"],
			queryFn: () => "settings",
		});
		await qc.prefetchQuery({
			queryKey: ["host", "abc"],
			queryFn: () => "host-abc",
		});
		return qc;
	};

	const invalidationState = (qc) =>
		Object.fromEntries(
			qc
				.getQueryCache()
				.findAll()
				.map((q) => [JSON.stringify(q.queryKey), q.state.isInvalidated]),
		);

	it("object form invalidates only the named key", async () => {
		const qc = await seed();
		await qc.invalidateQueries({ queryKey: ["settings"] });

		expect(invalidationState(qc)).toEqual({
			'["hosts"]': false,
			'["settings"]': true,
			'["host","abc"]': false,
		});
	});

	it("object form matches by key prefix, which is what the host queries rely on", async () => {
		const qc = await seed();
		await qc.invalidateQueries({ queryKey: ["host"] });

		const state = invalidationState(qc);
		expect(state['["host","abc"]']).toBe(true);
		expect(state['["hosts"]']).toBe(false);
		expect(state['["settings"]']).toBe(false);
	});

	it("the v4 bare-array form invalidates the entire cache (the defect)", async () => {
		const qc = await seed();
		await qc.invalidateQueries(["settings"]);

		// Everything is invalidated, not just ["settings"].
		expect(invalidationState(qc)).toEqual({
			'["hosts"]': true,
			'["settings"]': true,
			'["host","abc"]': true,
		});
	});
});

describe("source tree", () => {
	const SRC = path.resolve(__dirname, "..");

	const walk = (dir) =>
		fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
			const full = path.join(dir, entry.name);
			if (entry.isDirectory()) return walk(full);
			return /\.(jsx?|tsx?)$/.test(entry.name) ? [full] : [];
		});

	it("contains no v4-style bare-array invalidateQueries calls", () => {
		const offenders = walk(SRC)
			.filter((file) => !file.includes("__tests__"))
			.flatMap((file) => {
				const lines = fs.readFileSync(file, "utf8").split("\n");
				return lines
					.map((line, i) =>
						/invalidateQueries\(\s*\[/.test(line)
							? `${path.relative(SRC, file)}:${i + 1}`
							: null,
					)
					.filter(Boolean);
			});

		expect(offenders).toEqual([]);
	});
});
