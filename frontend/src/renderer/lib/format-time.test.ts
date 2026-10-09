import { describe, expect, it } from "vitest";
import { formatDurationCompact, formatTimeTerse } from "./format-time";

describe("formatTimeTerse", () => {
	const now = new Date("2026-08-26T12:00:00Z");

	it.each([
		["2026-08-26T11:59:30Z", "now"],
		["2026-08-26T11:55:00Z", "5m"],
		["2026-08-26T09:00:00Z", "3h"],
		["2026-08-14T12:00:00Z", "12d"],
	])("formats %s as %s", (timestamp, expected) => {
		expect(formatTimeTerse(timestamp, now)).toBe(expected);
	});
});

describe("formatDurationCompact", () => {
	it.each([
		[500, "< 1s"],
		[3_000, "< 5s"],
		[25_000, "25s"],
		[252_000, "4m 12s"],
		[3_600_000, "1h"],
		[3_720_000, "1h 2m"],
	])("formats %d ms as %s", (ms, expected) => {
		expect(formatDurationCompact(ms)).toBe(expected);
	});
});
