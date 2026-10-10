import { useQuery } from "@tanstack/react-query";
import type { components, operations } from "../../api/schema";
import { apiClient } from "../lib/api-client";

export type UsageAnalytics = components["schemas"]["UsageAnalyticsResponse"];
export type UsageAnalyticsBucket =
	components["schemas"]["UsageAnalyticsBucketResponse"];
export type UsageAnalyticsParams = NonNullable<
	operations["getUsageAnalytics"]["parameters"]["query"]
>;

export const usageAnalyticsQueryRoot = ["usage-analytics"] as const;

export const usageAnalyticsQueryKey = (params: UsageAnalyticsParams) =>
	[usageAnalyticsQueryRoot, params] as const;

export async function fetchUsageAnalytics(
	params: UsageAnalyticsParams,
): Promise<UsageAnalytics> {
	const { data, error } = await apiClient.GET("/api/v1/usage/analytics", {
		params: { query: params },
	});
	if (error) throw error;
	return data;
}

export function useUsageAnalytics(params: UsageAnalyticsParams) {
	return useQuery({
		queryKey: usageAnalyticsQueryKey(params),
		queryFn: () => fetchUsageAnalytics(params),
		refetchInterval: 15_000,
		retry: 1,
	});
}
