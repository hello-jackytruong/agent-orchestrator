import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type {
	UsageAnalytics,
	UsageAnalyticsParams,
} from "../hooks/useUsageAnalytics";
import { UsageAnalyticsDashboard } from "./UsageAnalyticsDashboard";
import { TooltipProvider } from "./ui/tooltip";

const mocks = vi.hoisted(() => ({
	useUsageAnalytics: vi.fn(),
}));

vi.mock("../hooks/useWorkspaceQuery", () => ({
	useWorkspaceQuery: () => ({
		data: [{ id: "project-1", name: "Demo project" }],
	}),
}));

vi.mock("../hooks/useUsageAnalytics", () => ({
	useUsageAnalytics: mocks.useUsageAnalytics,
}));

function dateKey(iso: string): string {
	const date = new Date(iso);
	const year = date.getFullYear();
	const month = `${date.getMonth() + 1}`.padStart(2, "0");
	const day = `${date.getDate()}`.padStart(2, "0");
	return `${year}-${month}-${day}`;
}

function shiftDateKey(value: string, days: number): string {
	const [year, month, day] = value.split("-").map(Number);
	return dateKey(new Date(year, month - 1, day + days).toISOString());
}

function dayLabel(iso: string): string {
	return new Intl.DateTimeFormat(undefined, {
		day: "numeric",
		month: "short",
	}).format(new Date(iso));
}

function totals(processedTokens: number | null = 0): UsageAnalytics["totals"] {
	return {
		cacheReadTokens: processedTokens == null ? null : 100,
		cachedInputTokens: processedTokens == null ? null : 100,
		estimatedCost:
			processedTokens == null
				? null
				: {
						cachedInputNanos: 10,
						coverage: "partial",
						inputNanos: 20,
						outputNanos: 30,
						providerAttribution: "mixed",
						totalNanos: 60,
					},
		inputTokens: processedTokens == null ? null : 300,
		outputTokens: processedTokens == null ? null : 200,
		processedTokens,
		uncachedInputTokens: processedTokens == null ? null : 200,
	};
}

function bucket(
	start: string,
	processedTokens: number,
	eventCount = 1,
): UsageAnalytics["dailyBuckets"][number] {
	return {
		start,
		end: new Date(new Date(start).getTime() + 60 * 60 * 1000).toISOString(),
		eventCount,
		totals: totals(processedTokens),
	};
}

const analytics: UsageAnalytics = {
	coverage: {
		eventCount: 3,
		incompleteSessionCount: 0,
		partialSourceCount: 1,
		pricedEventCount: 2,
		sessionCount: 2,
		sourceCount: 2,
		supportedSources: ["claude_main", "codex_rollout"],
		unpricedEventCount: 1,
	},
	dailyBuckets: [
		bucket("2026-01-05T00:00:00Z", 1200),
		bucket("2026-01-06T00:00:00Z", 600),
	],
	end: "2026-01-07T00:00:00Z",
	granularity: "day",
	harnesses: [{ eventCount: 3, harness: "codex", totals: totals(1800) }],
	hourlyBuckets: [
		bucket("2026-01-05T09:00:00Z", 800),
		bucket("2026-01-05T10:00:00Z", 400),
	],
	models: [
		{ eventCount: 2, harness: "codex", modelId: "gpt-5", totals: totals(1400) },
		{
			eventCount: 1,
			harness: "claude-code",
			modelId: "",
			totals: totals(null),
		},
	],
	projects: [
		{
			eventCount: 3,
			projectId: "project-1",
			projectName: "Demo project",
			totals: totals(1800),
		},
	],
	start: "2026-01-05T00:00:00Z",
	timeSeries: [
		bucket("2026-01-05T00:00:00Z", 1200),
		bucket("2026-01-06T00:00:00Z", 600),
	],
	timezone: "Asia/Ho_Chi_Minh",
	totals: totals(1800),
};

describe("UsageAnalyticsDashboard", () => {
	beforeEach(() => {
		mocks.useUsageAnalytics.mockReset();
		mocks.useUsageAnalytics.mockImplementation(
			(_params: UsageAnalyticsParams) => ({
				data: analytics,
				isError: false,
				isFetching: false,
				isLoading: false,
				refetch: vi.fn(),
			}),
		);
	});

	function renderDashboard() {
		return render(
			<TooltipProvider>
				<UsageAnalyticsDashboard />
			</TooltipProvider>,
		);
	}

	it("renders the usage analytics surface from persisted usage data", () => {
		renderDashboard();

		expect(
			screen.getByRole("heading", { name: "Usage analytics" }),
		).toBeInTheDocument();
		expect(screen.getByText("Processed tokens")).toBeInTheDocument();
		expect(screen.getByText("Daily heatmap")).toBeInTheDocument();
		expect(screen.getByText("Hourly heatmap")).toBeInTheDocument();
		expect(screen.getByText("Project comparison")).toBeInTheDocument();
		expect(screen.getByText("Model comparison")).toBeInTheDocument();
		expect(screen.getAllByText("Demo project")).not.toHaveLength(0);
		expect(screen.getByText(/stored pricing only/i)).toBeInTheDocument();
	});

	it("drills into a selected day by switching to a custom hourly range", async () => {
		const user = userEvent.setup();
		renderDashboard();

		const firstDay = analytics.dailyBuckets[0].start;
		await user.click(
			screen.getByRole("button", { name: new RegExp(dayLabel(firstDay)) }),
		);

		await waitFor(() => {
			const params = mocks.useUsageAnalytics.mock.calls.at(-1)?.[0] as
				| UsageAnalyticsParams
				| undefined;
			const start = dateKey(firstDay);
			expect(params).toMatchObject({
				start,
				end: shiftDateKey(start, 1),
				granularity: "hour",
			});
		});
	});
});
