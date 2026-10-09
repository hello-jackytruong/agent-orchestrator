import { beforeEach, describe, expect, it, vi } from "vitest";

const getMock = vi.hoisted(() => vi.fn());

vi.mock("../lib/host-clients", () => ({
	clientForSessionHost: () => ({ GET: (...args: unknown[]) => getMock(...args) }),
}));

import {
	fetchSessionTimeline,
	invalidateSessionTimeline,
	sessionTimelineQueryKey,
	sessionTimelineQueryRoot,
} from "./useSessionStatusTimeline";
import { QueryClient } from "@tanstack/react-query";

describe("useSessionStatusTimeline", () => {
	beforeEach(() => {
		getMock.mockReset();
	});

	it("fetches timeline for a session with default pagination", async () => {
		const mockResponse = {
			total: 2,
			limit: 100,
			offset: 0,
			items: [
				{
					id: "trans-1",
					sessionId: "sess-123",
					fromStatus: null,
					toStatus: "working",
					triggerSource: "agent",
					startedAt: "2026-10-09T08:00:00Z",
					createdAt: "2026-10-09T08:00:00Z",
				},
			],
		};
		getMock.mockResolvedValue({ data: mockResponse, error: undefined });

		const result = await fetchSessionTimeline("sess-123");

		expect(getMock).toHaveBeenCalledOnce();
		expect(getMock).toHaveBeenCalledWith("/api/v1/sessions/{sessionId}/timeline", {
			params: {
				path: { sessionId: "sess-123" },
				query: { limit: 100, offset: 0 },
			},
		});
		expect(result).toEqual(mockResponse);
	});

	it("passes custom limit and offset when specified", async () => {
		getMock.mockResolvedValue({ data: { total: 0, limit: 20, offset: 40, items: [] }, error: undefined });

		await fetchSessionTimeline("sess-123", undefined, 20, 40);

		expect(getMock).toHaveBeenCalledWith("/api/v1/sessions/{sessionId}/timeline", {
			params: {
				path: { sessionId: "sess-123" },
				query: { limit: 20, offset: 40 },
			},
		});
	});

	it("throws when API returns an error", async () => {
		getMock.mockResolvedValue({ data: undefined, error: { message: "Session not found" } });

		await expect(fetchSessionTimeline("unknown-session")).rejects.toEqual({ message: "Session not found" });
	});

	it("constructs query keys beneath the shared timeline root", () => {
		expect(sessionTimelineQueryKey("sess-1")).toEqual([
			...sessionTimelineQueryRoot,
			"sess-1",
			100,
			0,
		]);

		expect(sessionTimelineQueryKey("sess-1", "host-remote", 50, 10)).toEqual([
			...sessionTimelineQueryRoot,
			"host-remote",
			"sess-1",
			50,
			10,
		]);
	});

	it("invalidates queries for a specific session", async () => {
		const client = new QueryClient();
		const localKey = sessionTimelineQueryKey("sess-1");
		const otherKey = sessionTimelineQueryKey("sess-2");
		client.setQueryData(localKey, { items: [] });
		client.setQueryData(otherKey, { items: [] });

		await invalidateSessionTimeline(client, "sess-1");

		expect(client.getQueryState(localKey)?.isInvalidated).toBe(true);
		expect(client.getQueryState(otherKey)?.isInvalidated).toBe(false);
	});
});
