package controllers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apierr"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apispec"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/envelope"
	"github.com/aoagents/agent-orchestrator/backend/internal/procmem"
)

// UsageSummaryService is the controller-facing compact usage read contract.
type UsageSummaryService interface {
	ListCompact(context.Context, domain.ProjectID) ([]domain.CompactSessionUsage, error)
	Get(context.Context, domain.SessionID) (domain.SessionUsageSummary, error)
	GetAnalytics(context.Context, domain.UsageAnalyticsFilter) (domain.UsageAnalyticsSummary, error)
}

// SessionMemoryService samples resident memory per live session.
type SessionMemoryService interface {
	ListMemory(context.Context, domain.ProjectID) ([]domain.SessionMemory, error)
	// SystemMemory reports host RAM for the panel's total bar. Returns
	// procmem.ErrUnsupported where the platform can't be read; callers omit
	// the bar rather than fail the whole request.
	SystemMemory(context.Context) (domain.SystemMemory, error)
	// AppMemory is everything AO runs, for the topbar pressure indicator.
	AppMemory(context.Context) (domain.AppMemory, error)
}

// MemoryPressureReader reads the machine's memory-pressure verdict alone:
// cheap enough to poll while no other memory figure is on screen.
type MemoryPressureReader interface {
	Pressure(context.Context) (procmem.Pressure, error)
}

// SessionStepsReader lists a session's recent tool calls, oldest first.
type SessionStepsReader interface {
	Steps(id domain.SessionID) []domain.SessionStep
}

// UsageController owns compact dashboard usage routes.
type UsageController struct {
	Svc    UsageSummaryService
	Log    *slog.Logger
	Memory SessionMemoryService
	// Steps is optional: without it rows carry no activity.
	Steps SessionStepsReader
	// Pressure is optional: without it the pressure route is 501.
	Pressure MemoryPressureReader
}

// Register mounts usage routes on the supplied router.
func (c *UsageController) Register(r chi.Router) {
	r.Get("/usage/analytics", c.getAnalytics)
	r.Get("/usage/sessions", c.listSessions)
	r.Get("/usage/sessions/memory", c.listMemory)
	r.Get("/usage/memory/pressure", c.getPressure)
	r.Get("/usage/sessions/{sessionId}", c.getSession)
}

func (c *UsageController) getAnalytics(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, "GET", "/api/v1/usage/analytics")
		return
	}
	filter, err := usageAnalyticsFilterFromRequest(r)
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	summary, err := c.Svc.GetAnalytics(r.Context(), filter)
	if err != nil {
		c.Log.WarnContext(r.Context(), "failed to get usage analytics", "error", err)
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, usageAnalyticsResponse(summary))
}

func usageAnalyticsFilterFromRequest(r *http.Request) (domain.UsageAnalyticsFilter, error) {
	q := r.URL.Query()
	tz := strings.TrimSpace(q.Get("timezone"))
	loc := time.Local
	if tz != "" && tz != "Local" {
		loaded, err := time.LoadLocation(tz)
		if err != nil {
			return domain.UsageAnalyticsFilter{}, apierr.Invalid("USAGE_ANALYTICS_TIMEZONE_INVALID", "Invalid usage analytics timezone", map[string]any{"timezone": tz})
		}
		loc = loaded
	}
	start, err := parseUsageAnalyticsBoundary(q.Get("start"), loc)
	if err != nil {
		return domain.UsageAnalyticsFilter{}, apierr.Invalid("USAGE_ANALYTICS_START_INVALID", "Invalid usage analytics start", nil)
	}
	end, err := parseUsageAnalyticsBoundary(q.Get("end"), loc)
	if err != nil {
		return domain.UsageAnalyticsFilter{}, apierr.Invalid("USAGE_ANALYTICS_END_INVALID", "Invalid usage analytics end", nil)
	}
	return domain.UsageAnalyticsFilter{
		ProjectID: domain.ProjectID(strings.TrimSpace(q.Get("projectId"))),
		Harness:   domain.AgentHarness(strings.TrimSpace(q.Get("harness"))),
		ModelID:   strings.TrimSpace(q.Get("modelId")),
		Start:     start,
		End:       end,
		Timezone:  tz,
		Granularity: domain.UsageAnalyticsGranularity(
			strings.TrimSpace(q.Get("granularity")),
		),
	}, nil
}

func parseUsageAnalyticsBoundary(value string, loc *time.Location) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, nil
	}
	day, err := time.ParseInLocation("2006-01-02", value, loc)
	if err != nil {
		return time.Time{}, err
	}
	return day, nil
}

func (c *UsageController) listSessions(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, "GET", "/api/v1/usage/sessions")
		return
	}
	items, err := c.Svc.ListCompact(r.Context(), domain.ProjectID(r.URL.Query().Get("projectId")))
	if err != nil {
		c.Log.WarnContext(r.Context(), "failed to list compact session usage", "error", err)
		envelope.WriteError(w, r, err)
		return
	}
	out := make([]CompactSessionUsageResponse, 0, len(items))
	for _, item := range items {
		var totalTokens int64
		if item.ProcessedTokens != nil {
			totalTokens = *item.ProcessedTokens
		}
		out = append(out, CompactSessionUsageResponse{
			SessionID: item.SessionID, ProcessedTokens: item.ProcessedTokens,
			TotalTokens: totalTokens, Incomplete: item.Incomplete,
			EstimatedCost: estimatedCostResponse(item.EstimatedCost),
		})
	}
	envelope.WriteJSON(w, http.StatusOK, ListCompactSessionUsageResponse{Sessions: out})
}

func (c *UsageController) listMemory(w http.ResponseWriter, r *http.Request) {
	if c.Memory == nil {
		apispec.NotImplemented(w, r, "GET", "/api/v1/usage/sessions/memory")
		return
	}
	items, err := c.Memory.ListMemory(r.Context(), domain.ProjectID(r.URL.Query().Get("projectId")))
	if err != nil {
		if errors.Is(err, procmem.ErrUnsupported) {
			err = apierr.NotImplemented("MEMORY_UNSUPPORTED", err.Error())
		}
		envelope.WriteError(w, r, err)
		return
	}
	out := make([]SessionMemoryResponse, 0, len(items))
	for _, item := range items {
		row := sessionMemoryResponse(item)
		if c.Steps != nil {
			row.Activity = sessionActivityResponse(c.Steps.Steps(item.SessionID))
		}
		out = append(out, row)
	}
	var system *SystemMemoryResponse
	if sys, sysErr := c.Memory.SystemMemory(r.Context()); sysErr == nil {
		system = &SystemMemoryResponse{
			TotalBytes: sys.TotalBytes, AvailableBytes: sys.AvailableBytes,
			SwapTotalBytes: sys.SwapTotalBytes, SwapUsedBytes: sys.SwapUsedBytes, SwapBytesPerSec: sys.SwapBytesPerSec,
			CPUCount: sys.CPUCount, Load1: sys.Load1, CPUPercent: sys.CPUPercent, CPUMeasured: sys.CPUMeasured,
			PressureRaw: sys.PressureRaw, PressureSource: sys.PressureSource,
		}
	}
	var app *AppMemoryResponse
	if a, appErr := c.Memory.AppMemory(r.Context()); appErr == nil {
		app = &AppMemoryResponse{RSSBytes: a.RSSBytes, ProcessCount: a.ProcessCount, CPUPercent: a.CPUPercent, CPUMeasured: a.CPUMeasured}
		if a.Own.ProcessCount > 0 {
			own := sessionMemoryResponse(a.Own)
			app.Own = &own
		}
		for _, rv := range a.Reviewers {
			app.Reviewers = append(app.Reviewers, ReviewerMemoryResponse{
				ReviewID: rv.ReviewID, SessionID: rv.SessionID, Harness: string(rv.Harness),
				Memory: sessionMemoryResponse(rv.Memory),
			})
		}
	}
	envelope.WriteJSON(w, http.StatusOK, ListSessionMemoryResponse{Sessions: out, System: system, App: app})
}

func (c *UsageController) getPressure(w http.ResponseWriter, r *http.Request) {
	if c.Pressure == nil {
		apispec.NotImplemented(w, r, "GET", "/api/v1/usage/memory/pressure")
		return
	}
	p, err := c.Pressure.Pressure(r.Context())
	if err != nil {
		if errors.Is(err, procmem.ErrUnsupported) {
			err = apierr.NotImplemented("MEMORY_UNSUPPORTED", err.Error())
		}
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, MemoryPressureResponse{PressureRaw: p.Raw, PressureSource: p.Source})
}

// recentStepsShown is how many finished steps a row lists; the window is a
// glance, not a log.
const recentStepsShown = 5

// sessionActivityResponse splits the ring into the step still running and the
// last few finished ones, newest first. Nil when the harness reported none.
func sessionActivityResponse(steps []domain.SessionStep) *SessionActivityResponse {
	if len(steps) == 0 {
		return nil
	}
	out := &SessionActivityResponse{Recent: []SessionStepResponse{}}
	for i := len(steps) - 1; i >= 0; i-- {
		step := stepResponse(steps[i])
		if steps[i].EndedAt.IsZero() {
			if out.Current == nil {
				out.Current = &step
			}
			continue
		}
		if len(out.Recent) < recentStepsShown {
			out.Recent = append(out.Recent, step)
		}
	}
	return out
}

func stepResponse(step domain.SessionStep) SessionStepResponse {
	out := SessionStepResponse{Tool: step.Tool, StartedAt: step.StartedAt, Failed: step.Failed}
	if !step.EndedAt.IsZero() {
		ended := step.EndedAt
		out.EndedAt = &ended
	}
	return out
}

func sessionMemoryResponse(item domain.SessionMemory) SessionMemoryResponse {
	procs := make([]SessionMemoryProcessResponse, 0, len(item.Processes))
	for _, p := range item.Processes {
		procs = append(procs, SessionMemoryProcessResponse{PID: p.PID, PPID: p.PPID, RSSBytes: p.RSSBytes, CPUPercent: p.CPUPercent, Command: p.Command})
	}
	return SessionMemoryResponse{
		SessionID: item.SessionID, RSSBytes: item.RSSBytes, ProcessCount: item.ProcessCount, CPUPercent: item.CPUPercent,
		SampledAt: item.SampledAt, Processes: procs,
	}
}

func (c *UsageController) getSession(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, "GET", "/api/v1/usage/sessions/{sessionId}")
		return
	}
	summary, err := c.Svc.Get(r.Context(), domain.SessionID(chi.URLParam(r, "sessionId")))
	if err != nil {
		c.Log.WarnContext(r.Context(), "failed to get session usage", "error", err)
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, sessionUsageResponse(summary))
}

func sessionUsageResponse(summary domain.SessionUsageSummary) SessionUsageResponse {
	harnesses := make([]UsageHarnessResponse, 0, len(summary.Harnesses))
	for _, harness := range summary.Harnesses {
		models := make([]UsageModelResponse, 0, len(harness.Models))
		for _, model := range harness.Models {
			models = append(models, UsageModelResponse{
				ModelID: model.ModelID, Totals: usageTotalsResponse(model.Totals),
			})
		}
		harnesses = append(harnesses, UsageHarnessResponse{
			Harness: string(harness.Harness), Totals: usageTotalsResponse(harness.Totals), Models: models,
		})
	}
	return SessionUsageResponse{
		SessionID: summary.SessionID, Incomplete: summary.Incomplete,
		Totals: usageTotalsResponse(summary.Totals), Harnesses: harnesses,
	}
}

func usageAnalyticsResponse(summary domain.UsageAnalyticsSummary) UsageAnalyticsResponse {
	bucket := func(in []domain.UsageAnalyticsBucket) []UsageAnalyticsBucketResponse {
		out := make([]UsageAnalyticsBucketResponse, 0, len(in))
		for _, item := range in {
			out = append(out, UsageAnalyticsBucketResponse{
				Start: item.Start, End: item.End, EventCount: item.EventCount,
				Totals: usageTotalsResponse(item.Totals),
			})
		}
		return out
	}
	projects := make([]UsageAnalyticsProjectResponse, 0, len(summary.Projects))
	for _, item := range summary.Projects {
		projects = append(projects, UsageAnalyticsProjectResponse{
			ProjectID: item.ProjectID, ProjectName: item.ProjectName,
			EventCount: item.EventCount, Totals: usageTotalsResponse(item.Totals),
		})
	}
	models := make([]UsageAnalyticsModelResponse, 0, len(summary.Models))
	for _, item := range summary.Models {
		models = append(models, UsageAnalyticsModelResponse{
			Harness: string(item.Harness), ModelID: item.ModelID,
			EventCount: item.EventCount, Totals: usageTotalsResponse(item.Totals),
		})
	}
	harnesses := make([]UsageAnalyticsHarnessResponse, 0, len(summary.Harnesses))
	for _, item := range summary.Harnesses {
		harnesses = append(harnesses, UsageAnalyticsHarnessResponse{
			Harness: string(item.Harness), EventCount: item.EventCount,
			Totals: usageTotalsResponse(item.Totals),
		})
	}
	sources := make([]string, 0, len(summary.Coverage.SupportedSources))
	for _, source := range summary.Coverage.SupportedSources {
		sources = append(sources, string(source))
	}
	return UsageAnalyticsResponse{
		Start: summary.Start, End: summary.End, Timezone: summary.Timezone,
		Granularity: string(summary.Granularity), Totals: usageTotalsResponse(summary.Totals),
		DailyBuckets: bucket(summary.DailyBuckets), HourlyBuckets: bucket(summary.HourlyBuckets),
		TimeSeries: bucket(summary.TimeSeries), Projects: projects, Models: models,
		Harnesses: harnesses,
		Coverage: UsageAnalyticsCoverageResponse{
			EventCount: summary.Coverage.EventCount, PricedEventCount: summary.Coverage.PricedEventCount,
			UnpricedEventCount: summary.Coverage.UnpricedEventCount, SessionCount: summary.Coverage.SessionCount,
			IncompleteSessionCount: summary.Coverage.IncompleteSessionCount, SourceCount: summary.Coverage.SourceCount,
			PartialSourceCount: summary.Coverage.PartialSourceCount, SupportedSources: sources,
		},
	}
}

func usageTotalsResponse(totals domain.UsageMetricTotals) UsageTotalsResponse {
	return UsageTotalsResponse{
		InputTokens: totals.InputTokens, CachedInputTokens: totals.CachedInputTokens,
		UncachedInputTokens: totals.UncachedInputTokens,
		OutputTokens:        totals.OutputTokens, ProcessedTokens: totals.ProcessedTokens,
		CacheReadTokens: totals.CachedInputTokens,
		EstimatedCost:   estimatedCostResponse(totals.EstimatedCost),
	}
}

func estimatedCostResponse(cost *domain.EstimatedCost) *EstimatedCostResponse {
	if cost == nil {
		return nil
	}
	return &EstimatedCostResponse{
		TotalNanos: cost.TotalNanos, InputNanos: cost.InputNanos,
		CachedInputNanos: cost.CachedInputNanos, OutputNanos: cost.OutputNanos,
		Coverage:            string(cost.Coverage),
		ProviderAttribution: string(cost.ProviderAttribution),
	}
}
