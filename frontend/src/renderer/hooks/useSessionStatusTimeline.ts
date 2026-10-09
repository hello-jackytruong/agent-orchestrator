import { useQuery, type QueryClient } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { clientForSessionHost } from "../lib/host-clients";

export type SessionTimelineResponse = components["schemas"]["SessionTimelineResponse"];
export type SessionTimelineItem = components["schemas"]["SessionTimelineItemResponse"];

export const sessionTimelineQueryRoot = ["sessions", "timeline"] as const;

export const sessionTimelineQueryKey = (
	sessionId: string,
	hostId?: string,
	limit = 100,
	offset = 0,
) =>
	hostId
		? ([...sessionTimelineQueryRoot, hostId, sessionId, limit, offset] as const)
		: ([...sessionTimelineQueryRoot, sessionId, limit, offset] as const);

export async function fetchSessionTimeline(
	sessionId: string,
	hostId?: string,
	limit = 100,
	offset = 0,
): Promise<SessionTimelineResponse> {
	const { data, error } = await clientForSessionHost(hostId).GET(
		"/api/v1/sessions/{sessionId}/timeline",
		{
			params: {
				path: { sessionId },
				query: { limit, offset },
			},
		},
	);
	if (error) throw error;
	return data;
}

export function useSessionStatusTimeline(
	sessionId: string,
	options?: {
		limit?: number;
		offset?: number;
		enabled?: boolean;
		hostId?: string;
	},
) {
	const limit = options?.limit ?? 100;
	const offset = options?.offset ?? 0;
	const enabled = (options?.enabled ?? true) && Boolean(sessionId);

	return useQuery({
		queryKey: sessionTimelineQueryKey(sessionId, options?.hostId, limit, offset),
		queryFn: () => fetchSessionTimeline(sessionId, options?.hostId, limit, offset),
		enabled,
		retry: 1,
	});
}

export async function invalidateSessionTimeline(
	queryClient: QueryClient,
	sessionId?: string,
	hostId?: string,
): Promise<void> {
	if (!sessionId) {
		await queryClient.invalidateQueries({ queryKey: sessionTimelineQueryRoot });
		return;
	}
	if (hostId) {
		await queryClient.invalidateQueries({
			queryKey: [...sessionTimelineQueryRoot, hostId, sessionId],
		});
	} else {
		await queryClient.invalidateQueries({
			queryKey: [...sessionTimelineQueryRoot, sessionId],
		});
	}
}
