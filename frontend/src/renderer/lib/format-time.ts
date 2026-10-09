import { formatTimeCompact as formatPortableTimeCompact } from "@aoagents/product-ui";
import { appI18n, type MessageKey } from "../i18n";

export function formatTimeCompact(isoDate: string | null | undefined): string {
	return formatPortableTimeCompact(isoDate, {
		translate: (key, values) => appI18n.t(key as MessageKey, values),
	});
}

/** Extra-terse relative time for space-constrained navigation rows. */
export function formatTimeTerse(
	isoDate: string | null | undefined,
	now: number | Date = Date.now(),
): string {
	if (!isoDate) return "now";
	const timestamp = new Date(isoDate).getTime();
	if (!Number.isFinite(timestamp)) return "now";
	const nowMs = now instanceof Date ? now.getTime() : now;
	const diffMinutes = Math.floor((nowMs - timestamp) / 60_000);
	if (diffMinutes < 1) return "now";
	if (diffMinutes < 60) return `${diffMinutes}m`;
	const diffHours = Math.floor(diffMinutes / 60);
	if (diffHours < 24) return `${diffHours}h`;
	return `${Math.floor(diffHours / 24)}d`;
}

/** Formats duration in milliseconds to a compact label: "< 1s", "< 5s", "25s", "4m 12s", "1h 2m". */
export function formatDurationCompact(durationMs: number): string {
	if (durationMs < 1000) return "< 1s";
	if (durationMs < 5000) return "< 5s";
	const total = Math.max(0, Math.round(durationMs / 1000));
	if (total < 60) return `${total}s`;
	const minutes = Math.floor(total / 60);
	const seconds = total % 60;
	if (minutes < 60) {
		return seconds > 0 ? `${minutes}m ${seconds}s` : `${minutes}m`;
	}
	const hours = Math.floor(minutes / 60);
	const remMinutes = minutes % 60;
	return remMinutes > 0 ? `${hours}h ${remMinutes}m` : `${hours}h`;
}
