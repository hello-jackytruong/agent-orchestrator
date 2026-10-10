package usage

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apierr"
)

type usageSummaryStore interface {
	GetSession(context.Context, domain.SessionID) (domain.SessionRecord, bool, error)
	ListCompactSessionUsageAggregates(context.Context, domain.ProjectID) ([]domain.CompactSessionUsageAggregate, error)
	ListUsageModelAggregates(context.Context, domain.SessionID) ([]domain.UsageModelAggregate, error)
	GetUsageSessionIncomplete(context.Context, domain.SessionID) (bool, error)
	ListUsageAnalyticsHourlyAggregates(context.Context, domain.UsageAnalyticsFilter) ([]domain.UsageAnalyticsAggregate, error)
	GetUsageAnalyticsCoverage(context.Context, domain.UsageAnalyticsFilter) (domain.UsageAnalyticsCoverage, error)
}

// SummaryReader derives token and estimated-cost summaries from normalized
// usage events.
type SummaryReader struct{ store usageSummaryStore }

// NewSummaryReader constructs a usage summary reader.
func NewSummaryReader(store usageSummaryStore) *SummaryReader { return &SummaryReader{store: store} }

// ListCompact returns one batch suitable for dashboard cards.
func (r *SummaryReader) ListCompact(ctx context.Context, projectID domain.ProjectID) ([]domain.CompactSessionUsage, error) {
	if r == nil || r.store == nil {
		return nil, fmt.Errorf("usage summary store is unavailable")
	}
	rows, err := r.store.ListCompactSessionUsageAggregates(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.CompactSessionUsage, 0, len(rows))
	for _, row := range rows {
		estimatedCost, err := estimatedCost(row.Cost)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.CompactSessionUsage{
			SessionID: row.SessionID, ProcessedTokens: row.ProcessedTokens,
			Incomplete: row.Incomplete, EstimatedCost: estimatedCost,
		})
	}
	return out, nil
}

// Get returns detailed token and estimated-cost telemetry for one session.
func (r *SummaryReader) Get(ctx context.Context, sessionID domain.SessionID) (domain.SessionUsageSummary, error) {
	if r == nil || r.store == nil {
		return domain.SessionUsageSummary{}, fmt.Errorf("usage summary store is unavailable")
	}
	if _, ok, err := r.store.GetSession(ctx, sessionID); err != nil {
		return domain.SessionUsageSummary{}, err
	} else if !ok {
		return domain.SessionUsageSummary{}, apierr.NotFound("SESSION_NOT_FOUND", "Unknown session")
	}

	models, err := r.store.ListUsageModelAggregates(ctx, sessionID)
	if err != nil {
		return domain.SessionUsageSummary{}, err
	}
	visibleModels := make([]domain.UsageModelAggregate, 0, len(models))
	for _, model := range models {
		if strings.EqualFold(strings.TrimSpace(model.ModelID), "<synthetic>") {
			continue
		}
		visibleModels = append(visibleModels, model)
	}
	models = visibleModels
	incomplete, err := r.store.GetUsageSessionIncomplete(ctx, sessionID)
	if err != nil {
		return domain.SessionUsageSummary{}, err
	}
	totals, err := usageTotals(models)
	if err != nil {
		return domain.SessionUsageSummary{}, err
	}
	harnesses, err := harnessUsageSummaries(models)
	if err != nil {
		return domain.SessionUsageSummary{}, err
	}
	return domain.SessionUsageSummary{
		SessionID: sessionID, Incomplete: incomplete, Totals: totals, Harnesses: harnesses,
	}, nil
}

// GetAnalytics returns project-wide token and estimated-cost analytics for a
// bounded period. It keeps event-level null semantics by aggregating only from
// canonical storage groups.
func (r *SummaryReader) GetAnalytics(ctx context.Context, filter domain.UsageAnalyticsFilter) (domain.UsageAnalyticsSummary, error) {
	if r == nil || r.store == nil {
		return domain.UsageAnalyticsSummary{}, fmt.Errorf("usage summary store is unavailable")
	}
	normalized, loc, err := normalizeAnalyticsFilter(filter)
	if err != nil {
		return domain.UsageAnalyticsSummary{}, err
	}
	rows, err := r.store.ListUsageAnalyticsHourlyAggregates(ctx, normalized)
	if err != nil {
		return domain.UsageAnalyticsSummary{}, err
	}
	coverage, err := r.store.GetUsageAnalyticsCoverage(ctx, normalized)
	if err != nil {
		return domain.UsageAnalyticsSummary{}, err
	}
	totals, err := analyticsTotals(rows)
	if err != nil {
		return domain.UsageAnalyticsSummary{}, err
	}
	daily, err := analyticsDailyBuckets(rows, normalized.Start, normalized.End, loc)
	if err != nil {
		return domain.UsageAnalyticsSummary{}, err
	}
	hourly, err := analyticsHourlyBuckets(rows, normalized.Start, normalized.End, loc)
	if err != nil {
		return domain.UsageAnalyticsSummary{}, err
	}
	series, err := analyticsTimeSeries(rows, normalized, loc, daily, hourly)
	if err != nil {
		return domain.UsageAnalyticsSummary{}, err
	}
	projects, err := analyticsProjectSummaries(rows)
	if err != nil {
		return domain.UsageAnalyticsSummary{}, err
	}
	models, err := analyticsModelSummaries(rows)
	if err != nil {
		return domain.UsageAnalyticsSummary{}, err
	}
	harnesses, err := analyticsHarnessSummaries(rows)
	if err != nil {
		return domain.UsageAnalyticsSummary{}, err
	}
	return domain.UsageAnalyticsSummary{
		Start: normalized.Start.In(loc), End: normalized.End.In(loc), Timezone: normalized.Timezone,
		Granularity: normalized.Granularity, Totals: totals,
		DailyBuckets: daily, HourlyBuckets: hourly, TimeSeries: series,
		Projects: projects, Models: models, Harnesses: harnesses, Coverage: coverage,
	}, nil
}

func normalizeAnalyticsFilter(filter domain.UsageAnalyticsFilter) (domain.UsageAnalyticsFilter, *time.Location, error) {
	tz := strings.TrimSpace(filter.Timezone)
	var loc *time.Location
	var err error
	if tz == "" || tz == "Local" {
		loc = time.Local
		tz = loc.String()
	} else {
		loc, err = time.LoadLocation(tz)
		if err != nil {
			return domain.UsageAnalyticsFilter{}, nil, apierr.Invalid("USAGE_ANALYTICS_TIMEZONE_INVALID", "Invalid usage analytics timezone", map[string]any{"timezone": tz})
		}
	}
	granularity := filter.Granularity
	if granularity == "" {
		granularity = domain.UsageAnalyticsDay
	}
	switch granularity {
	case domain.UsageAnalyticsHour, domain.UsageAnalyticsDay, domain.UsageAnalyticsWeek:
	default:
		return domain.UsageAnalyticsFilter{}, nil, apierr.Invalid("USAGE_ANALYTICS_GRANULARITY_INVALID", "Invalid usage analytics granularity", map[string]any{"granularity": granularity})
	}
	start, end := filter.Start, filter.End
	if end.IsZero() {
		end = time.Now().In(loc)
	}
	if start.IsZero() {
		localEnd := dayStart(end.In(loc), loc).AddDate(0, 0, 1)
		start = localEnd.AddDate(-1, 0, 0)
		end = localEnd
	}
	if !end.After(start) {
		return domain.UsageAnalyticsFilter{}, nil, apierr.Invalid("USAGE_ANALYTICS_RANGE_INVALID", "Usage analytics end must be after start", nil)
	}
	filter.Start = start.UTC()
	filter.End = end.UTC()
	filter.Timezone = tz
	filter.Granularity = granularity
	filter.ModelID = strings.TrimSpace(filter.ModelID)
	return filter, loc, nil
}

type analyticsAccumulator struct {
	rows []domain.UsageAnalyticsAggregate
}

func (a *analyticsAccumulator) add(row domain.UsageAnalyticsAggregate) { a.rows = append(a.rows, row) }

func (a analyticsAccumulator) totals() (domain.UsageMetricTotals, error) {
	return analyticsTotals(a.rows)
}

func analyticsTotals(rows []domain.UsageAnalyticsAggregate) (domain.UsageMetricTotals, error) {
	if len(rows) == 0 {
		return domain.UsageMetricTotals{}, nil
	}
	var costs domain.UsageCostAggregate
	var input, cached, uncached, output int64
	inputKnown, cachedKnown, uncachedKnown, outputKnown := true, true, true, true
	for _, row := range rows {
		if err := mergeUsageCostAggregate(&costs, row.Cost); err != nil {
			return domain.UsageMetricTotals{}, err
		}
		if row.Tokens.InputTokens == nil {
			inputKnown = false
		} else {
			var err error
			input, err = checkedUsageAdd("input tokens", input, *row.Tokens.InputTokens)
			if err != nil {
				return domain.UsageMetricTotals{}, err
			}
		}
		if row.Tokens.CachedInputTokens == nil {
			cachedKnown = false
		} else {
			var err error
			cached, err = checkedUsageAdd("cached input tokens", cached, *row.Tokens.CachedInputTokens)
			if err != nil {
				return domain.UsageMetricTotals{}, err
			}
		}
		if row.Tokens.UncachedInputTokens == nil {
			uncachedKnown = false
		} else {
			var err error
			uncached, err = checkedUsageAdd("uncached input tokens", uncached, *row.Tokens.UncachedInputTokens)
			if err != nil {
				return domain.UsageMetricTotals{}, err
			}
		}
		if row.Tokens.OutputTokens == nil {
			outputKnown = false
		} else {
			var err error
			output, err = checkedUsageAdd("output tokens", output, *row.Tokens.OutputTokens)
			if err != nil {
				return domain.UsageMetricTotals{}, err
			}
		}
	}
	estimate, err := estimatedCost(costs)
	if err != nil {
		return domain.UsageMetricTotals{}, err
	}
	totals := domain.UsageMetricTotals{
		InputTokens:         knownInt64(input, inputKnown),
		CachedInputTokens:   knownInt64(cached, cachedKnown),
		UncachedInputTokens: knownInt64(uncached, uncachedKnown),
		OutputTokens:        knownInt64(output, outputKnown),
		EstimatedCost:       estimate,
	}
	if totals.InputTokens != nil && totals.OutputTokens != nil {
		processed, err := checkedUsageAdd("processed tokens", *totals.InputTokens, *totals.OutputTokens)
		if err != nil {
			return domain.UsageMetricTotals{}, err
		}
		totals.ProcessedTokens = &processed
	}
	return totals, nil
}

func knownInt64(value int64, ok bool) *int64 {
	if !ok {
		return nil
	}
	return &value
}

func analyticsHourlyBuckets(rows []domain.UsageAnalyticsAggregate, start, end time.Time, loc *time.Location) ([]domain.UsageAnalyticsBucket, error) {
	grouped := make(map[int64]analyticsAccumulator)
	for _, row := range rows {
		key := row.BucketHourUTC.UTC().Unix()
		acc := grouped[key]
		acc.add(row)
		grouped[key] = acc
	}
	first := start.UTC().Truncate(time.Hour)
	out := make([]domain.UsageAnalyticsBucket, 0, int(end.Sub(first).Hours())+1)
	for cursor := first; cursor.Before(end); cursor = cursor.Add(time.Hour) {
		acc := grouped[cursor.Unix()]
		totals, err := acc.totals()
		if err != nil {
			return nil, err
		}
		out = append(out, domain.UsageAnalyticsBucket{
			Start: cursor.In(loc), End: cursor.Add(time.Hour).In(loc),
			Totals: totals, EventCount: analyticsEventCount(acc.rows),
		})
	}
	return out, nil
}

func analyticsDailyBuckets(rows []domain.UsageAnalyticsAggregate, start, end time.Time, loc *time.Location) ([]domain.UsageAnalyticsBucket, error) {
	grouped := make(map[string]analyticsAccumulator)
	for _, row := range rows {
		local := row.BucketHourUTC.In(loc)
		key := dayStart(local, loc).Format("2006-01-02")
		acc := grouped[key]
		acc.add(row)
		grouped[key] = acc
	}
	localStart := dayStart(start.In(loc), loc)
	localEnd := end.In(loc)
	out := make([]domain.UsageAnalyticsBucket, 0)
	for cursor := localStart; cursor.Before(localEnd); cursor = cursor.AddDate(0, 0, 1) {
		acc := grouped[cursor.Format("2006-01-02")]
		totals, err := acc.totals()
		if err != nil {
			return nil, err
		}
		out = append(out, domain.UsageAnalyticsBucket{
			Start: cursor, End: cursor.AddDate(0, 0, 1),
			Totals: totals, EventCount: analyticsEventCount(acc.rows),
		})
	}
	return out, nil
}

func analyticsTimeSeries(rows []domain.UsageAnalyticsAggregate, filter domain.UsageAnalyticsFilter, loc *time.Location, daily, hourly []domain.UsageAnalyticsBucket) ([]domain.UsageAnalyticsBucket, error) {
	switch filter.Granularity {
	case domain.UsageAnalyticsHour:
		return hourly, nil
	case domain.UsageAnalyticsDay:
		return daily, nil
	case domain.UsageAnalyticsWeek:
		return analyticsWeeklyBuckets(rows, filter.Start, filter.End, loc)
	default:
		return nil, fmt.Errorf("unsupported usage analytics granularity %q", filter.Granularity)
	}
}

func analyticsWeeklyBuckets(rows []domain.UsageAnalyticsAggregate, start, end time.Time, loc *time.Location) ([]domain.UsageAnalyticsBucket, error) {
	grouped := make(map[string]analyticsAccumulator)
	for _, row := range rows {
		key := weekStart(row.BucketHourUTC.In(loc), loc).Format("2006-01-02")
		acc := grouped[key]
		acc.add(row)
		grouped[key] = acc
	}
	localStart := weekStart(start.In(loc), loc)
	localEnd := end.In(loc)
	out := make([]domain.UsageAnalyticsBucket, 0)
	for cursor := localStart; cursor.Before(localEnd); cursor = cursor.AddDate(0, 0, 7) {
		acc := grouped[cursor.Format("2006-01-02")]
		totals, err := acc.totals()
		if err != nil {
			return nil, err
		}
		out = append(out, domain.UsageAnalyticsBucket{
			Start: cursor, End: cursor.AddDate(0, 0, 7),
			Totals: totals, EventCount: analyticsEventCount(acc.rows),
		})
	}
	return out, nil
}

func analyticsProjectSummaries(rows []domain.UsageAnalyticsAggregate) ([]domain.UsageAnalyticsProjectSummary, error) {
	type projectKey struct {
		id   domain.ProjectID
		name string
	}
	order := make([]projectKey, 0)
	grouped := make(map[projectKey]analyticsAccumulator)
	for _, row := range rows {
		key := projectKey{row.ProjectID, row.ProjectName}
		if _, ok := grouped[key]; !ok {
			order = append(order, key)
		}
		acc := grouped[key]
		acc.add(row)
		grouped[key] = acc
	}
	out := make([]domain.UsageAnalyticsProjectSummary, 0, len(order))
	for _, key := range order {
		acc := grouped[key]
		totals, err := acc.totals()
		if err != nil {
			return nil, err
		}
		out = append(out, domain.UsageAnalyticsProjectSummary{
			ProjectID: key.id, ProjectName: key.name, Totals: totals, EventCount: analyticsEventCount(acc.rows),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return processedSortValue(out[i].Totals) > processedSortValue(out[j].Totals)
	})
	return out, nil
}

func analyticsModelSummaries(rows []domain.UsageAnalyticsAggregate) ([]domain.UsageAnalyticsModelSummary, error) {
	type modelKey struct {
		harness domain.AgentHarness
		model   string
	}
	order := make([]modelKey, 0)
	grouped := make(map[modelKey]analyticsAccumulator)
	for _, row := range rows {
		key := modelKey{row.Harness, row.ModelID}
		if _, ok := grouped[key]; !ok {
			order = append(order, key)
		}
		acc := grouped[key]
		acc.add(row)
		grouped[key] = acc
	}
	out := make([]domain.UsageAnalyticsModelSummary, 0, len(order))
	for _, key := range order {
		acc := grouped[key]
		totals, err := acc.totals()
		if err != nil {
			return nil, err
		}
		out = append(out, domain.UsageAnalyticsModelSummary{
			Harness: key.harness, ModelID: key.model, Totals: totals, EventCount: analyticsEventCount(acc.rows),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return processedSortValue(out[i].Totals) > processedSortValue(out[j].Totals)
	})
	return out, nil
}

func analyticsHarnessSummaries(rows []domain.UsageAnalyticsAggregate) ([]domain.UsageAnalyticsHarnessSummary, error) {
	order := make([]domain.AgentHarness, 0)
	grouped := make(map[domain.AgentHarness]analyticsAccumulator)
	for _, row := range rows {
		if _, ok := grouped[row.Harness]; !ok {
			order = append(order, row.Harness)
		}
		acc := grouped[row.Harness]
		acc.add(row)
		grouped[row.Harness] = acc
	}
	out := make([]domain.UsageAnalyticsHarnessSummary, 0, len(order))
	for _, harness := range order {
		acc := grouped[harness]
		totals, err := acc.totals()
		if err != nil {
			return nil, err
		}
		out = append(out, domain.UsageAnalyticsHarnessSummary{
			Harness: harness, Totals: totals, EventCount: analyticsEventCount(acc.rows),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return processedSortValue(out[i].Totals) > processedSortValue(out[j].Totals)
	})
	return out, nil
}

func analyticsEventCount(rows []domain.UsageAnalyticsAggregate) int64 {
	var total int64
	for _, row := range rows {
		total += row.Cost.EventCount
	}
	return total
}

func processedSortValue(totals domain.UsageMetricTotals) int64 {
	if totals.ProcessedTokens == nil {
		return -1
	}
	return *totals.ProcessedTokens
}

func dayStart(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

func weekStart(t time.Time, loc *time.Location) time.Time {
	localDay := dayStart(t, loc)
	offset := (int(localDay.Weekday()) + 6) % 7
	return localDay.AddDate(0, 0, -offset)
}

func usageTotals(models []domain.UsageModelAggregate) (domain.UsageMetricTotals, error) {
	if len(models) == 0 {
		return domain.UsageMetricTotals{}, nil
	}
	var costs domain.UsageCostAggregate
	for _, model := range models {
		if err := mergeUsageCostAggregate(&costs, model.Cost); err != nil {
			return domain.UsageMetricTotals{}, err
		}
	}
	estimate, err := estimatedCost(costs)
	if err != nil {
		return domain.UsageMetricTotals{}, err
	}
	input := aggregateMetric(models, func(model domain.UsageModelAggregate) *int64 { return model.Tokens.InputTokens })
	output := aggregateMetric(models, func(model domain.UsageModelAggregate) *int64 { return model.Tokens.OutputTokens })
	totals := domain.UsageMetricTotals{
		InputTokens:       input,
		CachedInputTokens: aggregateMetric(models, func(model domain.UsageModelAggregate) *int64 { return model.Tokens.CachedInputTokens }),
		UncachedInputTokens: aggregateMetric(models, func(model domain.UsageModelAggregate) *int64 {
			return model.Tokens.UncachedInputTokens
		}),
		OutputTokens:  output,
		EstimatedCost: estimate,
	}
	if input != nil && output != nil {
		processed := *input + *output
		totals.ProcessedTokens = &processed
	}
	return totals, nil
}

// aggregateMetric sums one metric across models. One uncollected counter makes
// the whole sum unknown rather than silently under-reporting it.
func aggregateMetric(models []domain.UsageModelAggregate, selectMetric func(domain.UsageModelAggregate) *int64) *int64 {
	var total int64
	for _, model := range models {
		value := selectMetric(model)
		if value == nil {
			return nil
		}
		total += *value
	}
	return &total
}

func harnessUsageSummaries(models []domain.UsageModelAggregate) ([]domain.HarnessUsageSummary, error) {
	order := make([]domain.AgentHarness, 0)
	grouped := make(map[domain.AgentHarness][]domain.UsageModelAggregate)
	for _, model := range models {
		if _, ok := grouped[model.Harness]; !ok {
			order = append(order, model.Harness)
		}
		grouped[model.Harness] = append(grouped[model.Harness], model)
	}
	out := make([]domain.HarnessUsageSummary, 0, len(order))
	for _, harness := range order {
		rows := grouped[harness]
		totals, err := usageTotals(rows)
		if err != nil {
			return nil, err
		}
		summary := domain.HarnessUsageSummary{Harness: harness, Totals: totals}
		for _, row := range rows {
			modelTotals, err := usageTotals([]domain.UsageModelAggregate{row})
			if err != nil {
				return nil, err
			}
			summary.Models = append(summary.Models, domain.ModelUsageSummary{
				ModelID: row.ModelID, Totals: modelTotals,
			})
		}
		out = append(out, summary)
	}
	return out, nil
}

func estimatedCost(raw domain.UsageCostAggregate) (*domain.EstimatedCost, error) {
	if err := validateUsageCostAggregate(raw); err != nil {
		return nil, err
	}
	if raw.EventCount == 0 {
		return nil, nil
	}
	coverage := domain.EstimatedCostCoverageComplete
	total := raw.PricedTotalNanos
	if raw.PricedEventCount != raw.EventCount {
		coverage = domain.EstimatedCostCoveragePartial
		var err error
		for _, component := range []struct {
			name  string
			value int64
		}{
			{"input cost", raw.UnpricedKnownInputNanos},
			{"cached input cost", raw.UnpricedKnownCachedInputNanos},
			{"output cost", raw.UnpricedKnownOutputNanos},
		} {
			total, err = checkedUsageAdd(component.name, total, component.value)
			if err != nil {
				return nil, err
			}
		}
		if total == 0 {
			return nil, nil
		}
	}
	providerAttribution, err := estimatedCostProviderAttribution(raw)
	if err != nil {
		return nil, err
	}
	return &domain.EstimatedCost{
		TotalNanos:          total,
		InputNanos:          knownComponent(raw.EventCount, raw.KnownInputCount, raw.KnownInputNanos),
		CachedInputNanos:    knownComponent(raw.EventCount, raw.KnownCachedInputCount, raw.KnownCachedInputNanos),
		OutputNanos:         knownComponent(raw.EventCount, raw.KnownOutputCount, raw.KnownOutputNanos),
		Coverage:            coverage,
		ProviderAttribution: providerAttribution,
	}, nil
}

func estimatedCostProviderAttribution(raw domain.UsageCostAggregate) (domain.EstimatedCostProviderAttribution, error) {
	switch {
	case raw.ObservedCostEventCount > 0 && raw.InferredCostEventCount > 0:
		return domain.EstimatedCostProviderAttributionMixed, nil
	case raw.InferredCostEventCount > 0:
		return domain.EstimatedCostProviderAttributionInferred, nil
	case raw.ObservedCostEventCount > 0:
		return domain.EstimatedCostProviderAttributionObserved, nil
	default:
		return "", fmt.Errorf("usage estimated cost has no provider attribution")
	}
}

func knownComponent(eventCount, knownCount, value int64) *int64 {
	if eventCount == knownCount {
		return &value
	}
	return nil
}

func mergeUsageCostAggregate(dst *domain.UsageCostAggregate, src domain.UsageCostAggregate) error {
	if err := validateUsageCostAggregate(src); err != nil {
		return err
	}
	fields := []struct {
		name string
		dst  *int64
		src  int64
	}{
		{"cost event count", &dst.EventCount, src.EventCount},
		{"priced event count", &dst.PricedEventCount, src.PricedEventCount},
		{"priced total cost", &dst.PricedTotalNanos, src.PricedTotalNanos},
		{"observed cost event count", &dst.ObservedCostEventCount, src.ObservedCostEventCount},
		{"inferred cost event count", &dst.InferredCostEventCount, src.InferredCostEventCount},
		{"known input count", &dst.KnownInputCount, src.KnownInputCount},
		{"known input cost", &dst.KnownInputNanos, src.KnownInputNanos},
		{"unpriced known input cost", &dst.UnpricedKnownInputNanos, src.UnpricedKnownInputNanos},
		{"known cached input count", &dst.KnownCachedInputCount, src.KnownCachedInputCount},
		{"known cached input cost", &dst.KnownCachedInputNanos, src.KnownCachedInputNanos},
		{"unpriced known cached input cost", &dst.UnpricedKnownCachedInputNanos, src.UnpricedKnownCachedInputNanos},
		{"known output count", &dst.KnownOutputCount, src.KnownOutputCount},
		{"known output cost", &dst.KnownOutputNanos, src.KnownOutputNanos},
		{"unpriced known output cost", &dst.UnpricedKnownOutputNanos, src.UnpricedKnownOutputNanos},
	}
	for _, field := range fields {
		value, err := checkedUsageAdd(field.name, *field.dst, field.src)
		if err != nil {
			return err
		}
		*field.dst = value
	}
	return nil
}

func validateUsageCostAggregate(raw domain.UsageCostAggregate) error {
	values := []struct {
		name  string
		value int64
	}{
		{"event count", raw.EventCount}, {"priced event count", raw.PricedEventCount}, {"priced total cost", raw.PricedTotalNanos},
		{"observed cost event count", raw.ObservedCostEventCount}, {"inferred cost event count", raw.InferredCostEventCount},
		{"known input count", raw.KnownInputCount}, {"known input cost", raw.KnownInputNanos}, {"unpriced known input cost", raw.UnpricedKnownInputNanos},
		{"known cached input count", raw.KnownCachedInputCount}, {"known cached input cost", raw.KnownCachedInputNanos}, {"unpriced known cached input cost", raw.UnpricedKnownCachedInputNanos},
		{"known output count", raw.KnownOutputCount}, {"known output cost", raw.KnownOutputNanos}, {"unpriced known output cost", raw.UnpricedKnownOutputNanos},
	}
	for _, item := range values {
		if item.value < 0 {
			return fmt.Errorf("usage %s must be nonnegative", item.name)
		}
	}
	if raw.PricedEventCount > raw.EventCount || raw.KnownInputCount > raw.EventCount ||
		raw.KnownCachedInputCount > raw.EventCount || raw.KnownOutputCount > raw.EventCount ||
		raw.ObservedCostEventCount > raw.EventCount || raw.InferredCostEventCount > raw.EventCount ||
		raw.InferredCostEventCount > raw.EventCount-raw.ObservedCostEventCount {
		return fmt.Errorf("usage cost coverage count exceeds event count")
	}
	if raw.UnpricedKnownInputNanos > raw.KnownInputNanos ||
		raw.UnpricedKnownCachedInputNanos > raw.KnownCachedInputNanos ||
		raw.UnpricedKnownOutputNanos > raw.KnownOutputNanos {
		return fmt.Errorf("usage unpriced component cost exceeds known component cost")
	}
	return nil
}

func checkedUsageAdd(label string, left, right int64) (int64, error) {
	if left < 0 || right < 0 {
		return 0, fmt.Errorf("usage %s must be nonnegative", label)
	}
	if left > math.MaxInt64-right {
		return 0, fmt.Errorf("usage %s overflows int64", label)
	}
	return left + right, nil
}
