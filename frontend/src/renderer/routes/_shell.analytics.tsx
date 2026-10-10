import { createFileRoute } from "@tanstack/react-router";
import { UsageAnalyticsDashboard } from "../components/UsageAnalyticsDashboard";

export const Route = createFileRoute("/_shell/analytics")({
	component: UsageAnalyticsDashboard,
});
