import { useMemo, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import {
	Activity,
	AlertTriangle,
	BarChart3,
	CircleDollarSign,
	Clock3,
	DatabaseZap,
	Filter,
	Gauge,
	RefreshCw,
} from "lucide-react";
import type { components } from "../../api/schema";
import {
	useUsageAnalytics,
	type UsageAnalytics,
	type UsageAnalyticsBucket,
	type UsageAnalyticsParams,
} from "../hooks/useUsageAnalytics";
import { formatEstimatedCost, formatCostNanos } from "../lib/format-cost";
import { formatTokenCount } from "../lib/format-token-count";
import { cn } from "../lib/utils";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip";

type UsageTotals = components["schemas"]["UsageTotalsResponse"];
type Metric = "tokens" | "cost";
type HeatmapMode = "daily" | "weekly" | "cumulative";
type RangePreset = "30d" | "90d" | "365d" | "custom";

const presetDays: Record<Exclude<RangePreset, "custom">, number> = {
	"30d": 30,
	"90d": 90,
	"365d": 365,
};

function localDateKey(date = new Date()): string {
	const year = date.getFullYear();
	const month = `${date.getMonth() + 1}`.padStart(2, "0");
	const day = `${date.getDate()}`.padStart(2, "0");
	return `${year}-${month}-${day}`;
}

function shiftDateKey(dateKey: string, days: number): string {
	const [year, month, day] = dateKey.split("-").map(Number);
	return localDateKey(new Date(year, month - 1, day + days));
}

function dateRangeForPreset(
	preset: RangePreset,
	customStart: string,
	customEnd: string,
) {
	const today = localDateKey();
	if (preset === "custom") {
		return { start: customStart, end: shiftDateKey(customEnd || today, 1) };
	}
	return {
		start: shiftDateKey(today, -presetDays[preset] + 1),
		end: shiftDateKey(today, 1),
	};
}

function bucketValue(
	bucket: UsageAnalyticsBucket,
	metric: Metric,
): number | null {
	if (metric === "cost")
		return (
			bucket.totals.estimatedCost?.totalNanos ??
			(bucket.eventCount === 0 ? 0 : null)
		);
	return bucket.totals.processedTokens ?? (bucket.eventCount === 0 ? 0 : null);
}

function totalsValue(totals: UsageTotals, metric: Metric): number | null {
	if (metric === "cost") return totals.estimatedCost?.totalNanos ?? null;
	return totals.processedTokens ?? null;
}

function formatMetricValue(
	value: number | null | undefined,
	metric: Metric,
	fallback: string,
): string {
	if (value == null) return fallback;
	return metric === "cost"
		? (formatCostNanos(value) ?? fallback)
		: formatTokenCount(value);
}

function intensity(value: number | null, max: number): string {
	if (value == null) return "bg-foreground/[0.06] border-border/40";
	if (value === 0 || max <= 0) return "bg-foreground/[0.08] border-transparent";
	const ratio = value / max;
	if (ratio > 0.75) return "bg-sky-400 border-sky-300/70";
	if (ratio > 0.45) return "bg-sky-500/85 border-sky-400/60";
	if (ratio > 0.2) return "bg-sky-600/70 border-sky-500/50";
	return "bg-sky-800/70 border-sky-600/50";
}

function weekStartKey(dateKey: string): string {
	const [year, month, day] = dateKey.split("-").map(Number);
	const date = new Date(year, month - 1, day);
	const offset = (date.getDay() + 6) % 7;
	return localDateKey(new Date(year, month - 1, day - offset));
}

function dayKey(iso: string): string {
	return localDateKey(new Date(iso));
}

function monthLabel(iso: string): string {
	return new Intl.DateTimeFormat(undefined, { month: "short" }).format(
		new Date(iso),
	);
}

function dayLabel(iso: string): string {
	return new Intl.DateTimeFormat(undefined, {
		day: "numeric",
		month: "short",
	}).format(new Date(iso));
}

function dateTimeLabel(iso: string): string {
	return new Intl.DateTimeFormat(undefined, {
		dateStyle: "medium",
		timeStyle: "short",
	}).format(new Date(iso));
}

function weekDayLabel(day: number): string {
	const base = new Date(2026, 0, 4 + day);
	return new Intl.DateTimeFormat(undefined, { weekday: "short" }).format(base);
}

function LoadingState() {
	return (
		<div className="grid gap-3 p-5">
			<div className="h-8 w-48 animate-pulse rounded-md bg-foreground/[0.08]" />
			<div className="grid gap-3 md:grid-cols-4">
				{Array.from({ length: 4 }, (_, index) => (
					<div
						className="h-24 animate-pulse rounded-lg bg-foreground/[0.06]"
						key={index}
					/>
				))}
			</div>
			<div className="h-72 animate-pulse rounded-lg bg-foreground/[0.06]" />
		</div>
	);
}

function EmptyState() {
	const { t } = useTranslation();
	return (
		<div className="flex min-h-80 flex-col items-center justify-center gap-3 px-6 text-center">
			<DatabaseZap
				className="size-8 text-muted-foreground"
				strokeWidth={1.7}
				aria-hidden="true"
			/>
			<div>
				<h2 className="text-base font-medium text-foreground">
					{t("usageAnalytics.emptyTitle")}
				</h2>
				<p className="mt-1 max-w-md text-sm leading-5 text-muted-foreground">
					{t("usageAnalytics.emptyDescription")}
				</p>
			</div>
		</div>
	);
}

function SummaryTile({
	icon,
	label,
	value,
	detail,
}: {
	icon: ReactNode;
	label: string;
	value: string;
	detail?: string;
}) {
	return (
		<div className="rounded-lg border border-border/70 bg-card/70 px-4 py-3">
			<div className="flex items-center gap-2 text-xs text-muted-foreground">
				<span
					className="grid size-6 place-items-center rounded-md bg-foreground/[0.05]"
					aria-hidden="true"
				>
					{icon}
				</span>
				<span className="truncate">{label}</span>
			</div>
			<div className="mt-3 font-mono text-xl font-medium tabular-nums text-foreground">
				{value}
			</div>
			{detail ? (
				<div className="mt-1 truncate text-xs text-muted-foreground">
					{detail}
				</div>
			) : null}
		</div>
	);
}

function DailyHeatmap({
	data,
	metric,
	mode,
	onSelectDay,
}: {
	data: UsageAnalytics;
	metric: Metric;
	mode: HeatmapMode;
	onSelectDay: (dateKey: string) => void;
}) {
	const { t } = useTranslation();
	const values = useMemo(() => {
		let running = 0;
		const weekly = new Map<string, number | null>();
		for (const bucket of data.dailyBuckets) {
			const value = bucketValue(bucket, metric);
			const key = weekStartKey(dayKey(bucket.start));
			if (value == null) {
				if (!weekly.has(key)) weekly.set(key, null);
			} else {
				weekly.set(key, (weekly.get(key) ?? 0) + value);
			}
		}
		return data.dailyBuckets.map((bucket) => {
			const raw = bucketValue(bucket, metric);
			if (mode === "cumulative") {
				if (raw != null) running += raw;
				return {
					bucket,
					value: raw == null && bucket.eventCount > 0 ? null : running,
				};
			}
			if (mode === "weekly")
				return {
					bucket,
					value: weekly.get(weekStartKey(dayKey(bucket.start))) ?? 0,
				};
			return { bucket, value: raw };
		});
	}, [data.dailyBuckets, metric, mode]);
	const max = Math.max(0, ...values.map((item) => item.value ?? 0));
	const columns = Math.max(1, Math.ceil(values.length / 7));
	const monthMarks = values.filter((item, index) => {
		if (index === 0) return true;
		const current = new Date(item.bucket.start);
		const previous = new Date(
			values[index - 1]?.bucket.start ?? item.bucket.start,
		);
		return current.getDate() <= 7 && previous.getMonth() !== current.getMonth();
	});

	return (
		<section className="rounded-lg border border-border/70 bg-card/60 p-4">
			<div className="mb-3 flex flex-wrap items-center justify-between gap-3">
				<div>
					<h2 className="text-sm font-medium text-foreground">
						{t("usageAnalytics.dailyHeatmap")}
					</h2>
					<p className="mt-1 text-xs text-muted-foreground">
						{t("usageAnalytics.dailyHeatmapHint")}
					</p>
				</div>
			</div>
			<div className="overflow-x-auto pb-1">
				<div className="relative min-w-max pt-5">
					<div
						className="absolute left-0 top-0 flex text-caption text-muted-foreground"
						style={{ width: columns * 16 }}
					>
						{monthMarks.map((item) => {
							const index = values.indexOf(item);
							return (
								<span
									key={item.bucket.start}
									className="absolute"
									style={{ left: Math.floor(index / 7) * 16 }}
								>
									{monthLabel(item.bucket.start)}
								</span>
							);
						})}
					</div>
					<div
						className="grid grid-flow-col grid-rows-7 gap-1"
						style={{ gridTemplateColumns: `repeat(${columns}, 12px)` }}
					>
						{values.map(({ bucket, value }) => (
							<Tooltip key={bucket.start}>
								<TooltipTrigger asChild>
									<button
										aria-label={t("usageAnalytics.bucketAria", {
											date: dayLabel(bucket.start),
											value: formatMetricValue(
												value,
												metric,
												t("usageAnalytics.unknown"),
											),
										})}
										className={cn(
											"size-3 rounded-[3px] border",
											intensity(value, max),
										)}
										onClick={() => onSelectDay(dayKey(bucket.start))}
										type="button"
									/>
								</TooltipTrigger>
								<TooltipContent side="top">
									{dayLabel(bucket.start)} ·{" "}
									{formatMetricValue(
										value,
										metric,
										t("usageAnalytics.unknown"),
									)}
								</TooltipContent>
							</Tooltip>
						))}
					</div>
				</div>
			</div>
			<div className="mt-3 flex items-center justify-end gap-1 text-caption text-muted-foreground">
				<span>{t("usageAnalytics.less")}</span>
				{[0, 1, 2, 3, 4].map((level) => (
					<span
						key={level}
						aria-hidden="true"
						className={cn("size-2.5 rounded-[2px] border", intensity(level, 4))}
					/>
				))}
				<span>{t("usageAnalytics.more")}</span>
			</div>
		</section>
	);
}

function HourlyMatrix({
	data,
	metric,
}: {
	data: UsageAnalytics;
	metric: Metric;
}) {
	const { t } = useTranslation();
	const cells = useMemo(() => {
		const grouped = new Map<string, { value: number | null; events: number }>();
		for (const bucket of data.hourlyBuckets) {
			const date = new Date(bucket.start);
			const key = `${date.getDay()}-${date.getHours()}`;
			const current = grouped.get(key) ?? { value: 0, events: 0 };
			const value = bucketValue(bucket, metric);
			grouped.set(key, {
				events: current.events + bucket.eventCount,
				value:
					current.value == null || value == null
						? current.events + bucket.eventCount > 0
							? null
							: 0
						: current.value + value,
			});
		}
		return grouped;
	}, [data.hourlyBuckets, metric]);
	const max = Math.max(
		0,
		...Array.from(cells.values()).map((cell) => cell.value ?? 0),
	);
	return (
		<section className="rounded-lg border border-border/70 bg-card/60 p-4">
			<h2 className="text-sm font-medium text-foreground">
				{t("usageAnalytics.hourlyHeatmap")}
			</h2>
			<div className="mt-4 overflow-x-auto">
				<div className="grid min-w-[640px] grid-cols-[52px_repeat(24,minmax(18px,1fr))] gap-1 text-caption">
					<span />
					{Array.from({ length: 24 }, (_, hour) => (
						<span className="text-center text-muted-foreground" key={hour}>
							{hour}
						</span>
					))}
					{Array.from({ length: 7 }, (_, day) => (
						<FragmentRow
							day={day}
							key={day}
							cells={cells}
							max={max}
							metric={metric}
						/>
					))}
				</div>
			</div>
		</section>
	);
}

function FragmentRow({
	day,
	cells,
	max,
	metric,
}: {
	day: number;
	cells: Map<string, { value: number | null; events: number }>;
	max: number;
	metric: Metric;
}) {
	const { t } = useTranslation();
	return (
		<>
			<span className="flex items-center text-muted-foreground">
				{weekDayLabel(day)}
			</span>
			{Array.from({ length: 24 }, (_, hour) => {
				const cell = cells.get(`${day}-${hour}`) ?? { value: 0, events: 0 };
				return (
					<Tooltip key={hour}>
						<TooltipTrigger asChild>
							<button
								aria-label={t("usageAnalytics.hourAria", {
									day: weekDayLabel(day),
									hour,
									value: formatMetricValue(
										cell.value,
										metric,
										t("usageAnalytics.unknown"),
									),
								})}
								className={cn(
									"h-5 rounded-[3px] border",
									intensity(cell.value, max),
								)}
								type="button"
							/>
						</TooltipTrigger>
						<TooltipContent side="top">
							{weekDayLabel(day)} {hour}:00 ·{" "}
							{formatMetricValue(
								cell.value,
								metric,
								t("usageAnalytics.unknown"),
							)}
						</TooltipContent>
					</Tooltip>
				);
			})}
		</>
	);
}

function TimeSeriesChart({
	data,
	metric,
}: {
	data: UsageAnalytics;
	metric: Metric;
}) {
	const { t } = useTranslation();
	const width = 720;
	const height = 180;
	const pad = 18;
	const points = data.timeSeries.map((bucket, index) => ({
		index,
		bucket,
		value: bucketValue(bucket, metric),
	}));
	const max = Math.max(1, ...points.map((point) => point.value ?? 0));
	const x = (index: number) =>
		points.length <= 1
			? pad
			: pad + (index / (points.length - 1)) * (width - pad * 2);
	const y = (value: number | null) =>
		height - pad - ((value ?? 0) / max) * (height - pad * 2);
	const path = points
		.map(
			(point, index) =>
				`${index === 0 ? "M" : "L"} ${x(point.index).toFixed(1)} ${y(point.value).toFixed(1)}`,
		)
		.join(" ");
	return (
		<div className="mt-3 overflow-x-auto">
			<svg
				className="h-[180px] min-w-[560px] text-sky-400"
				viewBox={`0 0 ${width} ${height}`}
				role="img"
				aria-label={t("usageAnalytics.timeSeries")}
			>
				<line
					x1={pad}
					x2={width - pad}
					y1={height - pad}
					y2={height - pad}
					stroke="currentColor"
					strokeOpacity="0.16"
				/>
				<path
					d={path}
					fill="none"
					stroke="currentColor"
					strokeWidth="2.5"
					strokeLinecap="round"
					strokeLinejoin="round"
				/>
				{points.map((point) => (
					<circle
						className="fill-background stroke-current"
						cx={x(point.index)}
						cy={y(point.value)}
						key={point.bucket.start}
						r="4"
						strokeWidth="2"
					>
						<title>{`${dateTimeLabel(point.bucket.start)} · ${formatMetricValue(point.value, metric, t("usageAnalytics.unknown"))}`}</title>
					</circle>
				))}
			</svg>
		</div>
	);
}

function ComparisonTable({ data }: { data: UsageAnalytics }) {
	const { t } = useTranslation();
	const rows = data.projects.map((project) => ({
		id: project.projectId,
		name:
			project.projectName ||
			project.projectId ||
			t("usageAnalytics.standaloneProject"),
		tokens: project.totals.processedTokens,
		cost: project.totals.estimatedCost?.totalNanos ?? null,
		share:
			data.totals.processedTokens && project.totals.processedTokens
				? project.totals.processedTokens / data.totals.processedTokens
				: null,
	}));
	return (
		<section className="rounded-lg border border-border/70 bg-card/60 p-4">
			<h2 className="text-sm font-medium text-foreground">
				{t("usageAnalytics.projectComparison")}
			</h2>
			<div className="mt-3 overflow-x-auto">
				<table className="w-full min-w-[520px] text-sm">
					<thead className="text-left text-xs text-muted-foreground">
						<tr className="border-b border-border/60">
							<th className="py-2 font-medium">
								{t("usageAnalytics.project")}
							</th>
							<th className="py-2 text-right font-medium">
								{t("usageAnalytics.tokens")}
							</th>
							<th className="py-2 text-right font-medium">
								{t("usageAnalytics.cost")}
							</th>
							<th className="py-2 text-right font-medium">
								{t("usageAnalytics.share")}
							</th>
						</tr>
					</thead>
					<tbody>
						{rows.map((row) => (
							<tr
								className="border-b border-border/40 last:border-0"
								key={row.id}
							>
								<td className="py-2 text-foreground">{row.name}</td>
								<td className="py-2 text-right font-mono tabular-nums">
									{formatMetricValue(
										row.tokens,
										"tokens",
										t("usageAnalytics.unknown"),
									)}
								</td>
								<td className="py-2 text-right font-mono tabular-nums">
									{formatMetricValue(
										row.cost,
										"cost",
										t("usageAnalytics.unknown"),
									)}
								</td>
								<td className="py-2 text-right font-mono tabular-nums">
									{row.share == null
										? t("usageAnalytics.unknown")
										: `${Math.round(row.share * 100)}%`}
								</td>
							</tr>
						))}
					</tbody>
				</table>
			</div>
		</section>
	);
}

function ModelTable({
	data,
	metric,
}: {
	data: UsageAnalytics;
	metric: Metric;
}) {
	const { t } = useTranslation();
	const total = totalsValue(data.totals, metric);
	return (
		<section className="rounded-lg border border-border/70 bg-card/60 p-4">
			<h2 className="text-sm font-medium text-foreground">
				{t("usageAnalytics.modelComparison")}
			</h2>
			<div className="mt-3 overflow-x-auto">
				<table className="w-full min-w-[600px] text-sm">
					<thead className="text-left text-xs text-muted-foreground">
						<tr className="border-b border-border/60">
							<th className="py-2 font-medium">{t("usageAnalytics.model")}</th>
							<th className="py-2 font-medium">
								{t("usageAnalytics.harness")}
							</th>
							<th className="py-2 text-right font-medium">
								{t("usageAnalytics.metric")}
							</th>
							<th className="py-2 text-right font-medium">
								{t("usageAnalytics.share")}
							</th>
						</tr>
					</thead>
					<tbody>
						{data.models.map((row) => {
							const value = totalsValue(row.totals, metric);
							const share = total && value ? value / total : null;
							return (
								<tr
									className="border-b border-border/40 last:border-0"
									key={`${row.harness}-${row.modelId}`}
								>
									<td className="py-2 font-mono text-xs text-foreground">
										{row.modelId || t("usageAnalytics.unknownModel")}
									</td>
									<td className="py-2 text-muted-foreground">{row.harness}</td>
									<td className="py-2 text-right font-mono tabular-nums">
										{formatMetricValue(
											value,
											metric,
											t("usageAnalytics.unknown"),
										)}
									</td>
									<td className="py-2 text-right font-mono tabular-nums">
										{share == null
											? t("usageAnalytics.unknown")
											: `${Math.round(share * 100)}%`}
									</td>
								</tr>
							);
						})}
					</tbody>
				</table>
			</div>
		</section>
	);
}

function CoverageNotice({ data }: { data: UsageAnalytics }) {
	const { t } = useTranslation();
	const cost = data.totals.estimatedCost;
	return (
		<div className="rounded-lg border border-border/70 bg-card/60 px-4 py-3 text-xs leading-5 text-muted-foreground">
			<div className="flex items-start gap-2">
				<AlertTriangle
					className="mt-0.5 size-4 shrink-0 text-warning"
					strokeWidth={1.8}
					aria-hidden="true"
				/>
				<p>
					{t("usageAnalytics.coverageNote", {
						sessions: data.coverage.sessionCount,
						sources: data.coverage.sourceCount,
						partialSources: data.coverage.partialSourceCount,
						unpriced: data.coverage.unpricedEventCount,
						coverage: cost?.coverage ?? t("usageAnalytics.unavailable"),
						attribution:
							cost?.providerAttribution ?? t("usageAnalytics.unavailable"),
						sourcesList: data.coverage.supportedSources.join(", "),
					})}
				</p>
			</div>
		</div>
	);
}

export function UsageAnalyticsDashboard() {
	const { t } = useTranslation();
	const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || "Local";
	const [preset, setPreset] = useState<RangePreset>("90d");
	const [customStart, setCustomStart] = useState(
		shiftDateKey(localDateKey(), -29),
	);
	const [customEnd, setCustomEnd] = useState(localDateKey());
	const [projectId, setProjectId] = useState("all");
	const [modelId, setModelId] = useState("all");
	const [harness, setHarness] = useState("all");
	const [metric, setMetric] = useState<Metric>("tokens");
	const [mode, setMode] = useState<HeatmapMode>("daily");
	const [granularity, setGranularity] = useState<"hour" | "day" | "week">(
		"day",
	);
	const range = dateRangeForPreset(preset, customStart, customEnd);
	const params: UsageAnalyticsParams = {
		start: range.start,
		end: range.end,
		timezone,
		granularity,
		...(projectId === "all" ? {} : { projectId }),
		...(modelId === "all" ? {} : { modelId }),
		...(harness === "all" ? {} : { harness }),
	};
	const query = useUsageAnalytics(params);
	const data = query.data;
	const modelOptions = useMemo(
		() =>
			Array.from(
				new Set(data?.models.map((row) => row.modelId).filter(Boolean) ?? []),
			).sort(),
		[data],
	);
	const harnessOptions = useMemo(
		() =>
			Array.from(
				new Set(
					data?.harnesses.map((row) => row.harness).filter(Boolean) ?? [],
				),
			).sort(),
		[data],
	);
	const projectOptions = useMemo(
		() =>
			data?.projects
				.filter((project) => project.projectId)
				.map((project) => ({
					id: project.projectId,
					name: project.projectName || project.projectId,
				})) ?? [],
		[data],
	);
	const selectDay = (dateKeyValue: string) => {
		setPreset("custom");
		setCustomStart(dateKeyValue);
		setCustomEnd(dateKeyValue);
		setGranularity("hour");
	};

	return (
		<div className="flex h-full min-h-0 flex-col bg-background text-foreground">
			<div className="workspace-topbar-container center-panel-titlebar flex h-toolbar shrink-0 items-center gap-2 border-b border-border-strong px-4">
				<BarChart3
					className="size-4 text-muted-foreground"
					strokeWidth={1.8}
					aria-hidden="true"
				/>
				<h1 className="text-sm font-medium text-foreground">
					{t("usageAnalytics.title")}
				</h1>
				<Button
					className="ml-auto h-8 gap-2 px-2.5"
					onClick={() => void query.refetch()}
					size="sm"
					variant="ghost"
				>
					<RefreshCw
						className={cn("size-3.5", query.isFetching && "animate-spin")}
						aria-hidden="true"
					/>
					{t("usageAnalytics.refresh")}
				</Button>
			</div>
			<div className="min-h-0 flex-1 overflow-auto">
				<div className="mx-auto flex w-full max-w-[1480px] flex-col gap-4 p-5">
					<section className="rounded-lg border border-border/70 bg-card/60 p-4">
						<div className="flex flex-wrap items-end gap-3">
							<div className="grid gap-1.5">
								<label className="text-xs text-muted-foreground">
									{t("usageAnalytics.range")}
								</label>
								<Select
									value={preset}
									onValueChange={(value) => setPreset(value as RangePreset)}
								>
									<SelectTrigger className="w-32">
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="30d">
											{t("usageAnalytics.range30")}
										</SelectItem>
										<SelectItem value="90d">
											{t("usageAnalytics.range90")}
										</SelectItem>
										<SelectItem value="365d">
											{t("usageAnalytics.range365")}
										</SelectItem>
										<SelectItem value="custom">
											{t("usageAnalytics.custom")}
										</SelectItem>
									</SelectContent>
								</Select>
							</div>
							{preset === "custom" ? (
								<>
									<Input
										aria-label={t("usageAnalytics.startDate")}
										className="w-36"
										type="date"
										value={customStart}
										onChange={(event) => setCustomStart(event.target.value)}
									/>
									<Input
										aria-label={t("usageAnalytics.endDate")}
										className="w-36"
										type="date"
										value={customEnd}
										onChange={(event) => setCustomEnd(event.target.value)}
									/>
								</>
							) : null}
							<div className="grid gap-1.5">
								<label className="text-xs text-muted-foreground">
									{t("usageAnalytics.project")}
								</label>
								<Select value={projectId} onValueChange={setProjectId}>
									<SelectTrigger className="w-44">
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="all">
											{t("usageAnalytics.allProjects")}
										</SelectItem>
										{projectOptions.map((project) => (
											<SelectItem key={project.id} value={project.id}>
												{project.name}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
							</div>
							<div className="grid gap-1.5">
								<label className="text-xs text-muted-foreground">
									{t("usageAnalytics.harness")}
								</label>
								<Select value={harness} onValueChange={setHarness}>
									<SelectTrigger className="w-40">
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="all">
											{t("usageAnalytics.allHarnesses")}
										</SelectItem>
										{harnessOptions.map((item) => (
											<SelectItem key={item} value={item}>
												{item}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
							</div>
							<div className="grid gap-1.5">
								<label className="text-xs text-muted-foreground">
									{t("usageAnalytics.model")}
								</label>
								<Select value={modelId} onValueChange={setModelId}>
									<SelectTrigger className="w-44">
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="all">
											{t("usageAnalytics.allModels")}
										</SelectItem>
										{modelOptions.map((item) => (
											<SelectItem key={item} value={item}>
												{item}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
							</div>
							<div className="grid gap-1.5">
								<label className="text-xs text-muted-foreground">
									{t("usageAnalytics.metric")}
								</label>
								<Select
									value={metric}
									onValueChange={(value) => setMetric(value as Metric)}
								>
									<SelectTrigger className="w-32">
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="tokens">
											{t("usageAnalytics.tokens")}
										</SelectItem>
										<SelectItem value="cost">
											{t("usageAnalytics.cost")}
										</SelectItem>
									</SelectContent>
								</Select>
							</div>
							<div className="grid gap-1.5">
								<label className="text-xs text-muted-foreground">
									{t("usageAnalytics.series")}
								</label>
								<Select
									value={granularity}
									onValueChange={(value) =>
										setGranularity(value as "hour" | "day" | "week")
									}
								>
									<SelectTrigger className="w-32">
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="hour">
											{t("usageAnalytics.hour")}
										</SelectItem>
										<SelectItem value="day">
											{t("usageAnalytics.day")}
										</SelectItem>
										<SelectItem value="week">
											{t("usageAnalytics.week")}
										</SelectItem>
									</SelectContent>
								</Select>
							</div>
						</div>
					</section>

					{query.isLoading ? <LoadingState /> : null}
					{query.isError ? (
						<div className="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">
							{t("usageAnalytics.error")}
						</div>
					) : null}
					{data && data.coverage.eventCount === 0 ? <EmptyState /> : null}
					{data && data.coverage.eventCount > 0 ? (
						<>
							<div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
								<SummaryTile
									icon={<Gauge className="size-3.5" />}
									label={t("usageAnalytics.processed")}
									value={formatMetricValue(
										data.totals.processedTokens,
										"tokens",
										t("usageAnalytics.unknown"),
									)}
									detail={t("usageAnalytics.processedDetail")}
								/>
								<SummaryTile
									icon={<Activity className="size-3.5" />}
									label={t("usageAnalytics.inputOutput")}
									value={`${formatMetricValue(data.totals.inputTokens, "tokens", t("usageAnalytics.unknown"))} / ${formatMetricValue(data.totals.outputTokens, "tokens", t("usageAnalytics.unknown"))}`}
								/>
								<SummaryTile
									icon={<Filter className="size-3.5" />}
									label={t("usageAnalytics.cachedUncached")}
									value={`${formatMetricValue(data.totals.cachedInputTokens, "tokens", t("usageAnalytics.unknown"))} / ${formatMetricValue(data.totals.uncachedInputTokens, "tokens", t("usageAnalytics.unknown"))}`}
								/>
								<SummaryTile
									icon={<CircleDollarSign className="size-3.5" />}
									label={t("usageAnalytics.estimatedCost")}
									value={
										formatEstimatedCost(data.totals.estimatedCost) ??
										t("usageAnalytics.unknown")
									}
									detail={
										data.totals.estimatedCost
											? t("usageAnalytics.costDetail", {
													coverage: data.totals.estimatedCost.coverage,
												})
											: undefined
									}
								/>
							</div>
							<CoverageNotice data={data} />
							<div className="grid gap-4 xl:grid-cols-[minmax(0,1.45fr)_minmax(380px,0.85fr)]">
								<DailyHeatmap
									data={data}
									metric={metric}
									mode={mode}
									onSelectDay={selectDay}
								/>
								<section className="rounded-lg border border-border/70 bg-card/60 p-4">
									<div className="mb-3 flex items-center justify-between gap-3">
										<h2 className="text-sm font-medium text-foreground">
											{t("usageAnalytics.timeSeries")}
										</h2>
										<div
											className="inline-flex rounded-lg bg-foreground/[0.04] p-1"
											aria-label={t("usageAnalytics.aggregation")}
										>
											{(["daily", "weekly", "cumulative"] as HeatmapMode[]).map(
												(item) => (
													<button
														key={item}
														className={cn(
															"h-7 rounded-md px-2.5 text-xs font-medium text-muted-foreground hover:text-foreground",
															mode === item &&
																"bg-interactive-active text-foreground",
														)}
														onClick={() => setMode(item)}
														type="button"
													>
														{t(`usageAnalytics.mode.${item}`)}
													</button>
												),
											)}
										</div>
									</div>
									<TimeSeriesChart data={data} metric={metric} />
								</section>
							</div>
							<HourlyMatrix data={data} metric={metric} />
							<div className="grid gap-4 xl:grid-cols-2">
								<ComparisonTable data={data} />
								<ModelTable data={data} metric={metric} />
							</div>
							<div className="text-xs text-muted-foreground">
								<Clock3 className="mr-1 inline size-3.5" aria-hidden="true" />
								{t("usageAnalytics.timezoneNote", { timezone: data.timezone })}
							</div>
						</>
					) : null}
				</div>
			</div>
		</div>
	);
}
