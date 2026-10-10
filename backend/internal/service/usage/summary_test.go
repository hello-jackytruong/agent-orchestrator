package usage

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

type usageSummaryStoreStub struct {
	projectID  domain.ProjectID
	filter     domain.UsageAnalyticsFilter
	rows       []domain.CompactSessionUsageAggregate
	session    domain.SessionRecord
	found      bool
	incomplete bool
	models     []domain.UsageModelAggregate
	analytics  []domain.UsageAnalyticsAggregate
	coverage   domain.UsageAnalyticsCoverage
	calls      [6]int
}

func (s *usageSummaryStoreStub) ListCompactSessionUsageAggregates(_ context.Context, id domain.ProjectID) ([]domain.CompactSessionUsageAggregate, error) {
	s.projectID, s.calls[0] = id, s.calls[0]+1
	return s.rows, nil
}
func (s *usageSummaryStoreStub) GetSession(context.Context, domain.SessionID) (domain.SessionRecord, bool, error) {
	s.calls[1]++
	return s.session, s.found, nil
}
func (s *usageSummaryStoreStub) ListUsageModelAggregates(context.Context, domain.SessionID) ([]domain.UsageModelAggregate, error) {
	s.calls[2]++
	return s.models, nil
}
func (s *usageSummaryStoreStub) GetUsageSessionIncomplete(context.Context, domain.SessionID) (bool, error) {
	s.calls[3]++
	return s.incomplete, nil
}
func (s *usageSummaryStoreStub) ListUsageAnalyticsHourlyAggregates(_ context.Context, filter domain.UsageAnalyticsFilter) ([]domain.UsageAnalyticsAggregate, error) {
	s.calls[4]++
	s.filter = filter
	return s.analytics, nil
}
func (s *usageSummaryStoreStub) GetUsageAnalyticsCoverage(_ context.Context, filter domain.UsageAnalyticsFilter) (domain.UsageAnalyticsCoverage, error) {
	s.calls[5]++
	s.filter = filter
	return s.coverage, nil
}

func TestSummaryReaderListCompactUsesOneBatchRead(t *testing.T) {
	zeroProcessed, partialProcessed := int64(0), int64(120)
	store := &usageSummaryStoreStub{rows: []domain.CompactSessionUsageAggregate{
		{
			SessionID: "zero", ProcessedTokens: &zeroProcessed,
			Cost: completeCostAggregate(1, 0, 0, 0, 0),
		},
		{
			SessionID: "partial", ProcessedTokens: &partialProcessed, Incomplete: true,
			Cost: domain.UsageCostAggregate{
				EventCount:                    1,
				ObservedCostEventCount:        1,
				KnownInputCount:               1,
				KnownInputNanos:               30,
				UnpricedKnownInputNanos:       30,
				KnownCachedInputCount:         1,
				KnownCachedInputNanos:         0,
				UnpricedKnownCachedInputNanos: 0,
				KnownOutputCount:              1,
				KnownOutputNanos:              5,
				UnpricedKnownOutputNanos:      5,
			},
		},
	}}

	got, err := NewSummaryReader(store).ListCompact(context.Background(), "reverb")
	mustNoError(t, err)
	if store.calls[0] != 1 || store.projectID != "reverb" || len(got) != 2 {
		t.Fatalf("read=%d project=%q items=%+v", store.calls[0], store.projectID, got)
	}
	if got[0].EstimatedCost == nil || got[0].EstimatedCost.Coverage != domain.EstimatedCostCoverageComplete || got[0].EstimatedCost.TotalNanos != 0 {
		t.Fatalf("zero cost = %+v, want complete zero", got[0].EstimatedCost)
	}
	if got[1].ProcessedTokens == nil || *got[1].ProcessedTokens != 120 || !got[1].Incomplete ||
		got[1].EstimatedCost == nil ||
		got[1].EstimatedCost.Coverage != domain.EstimatedCostCoveragePartial || got[1].EstimatedCost.TotalNanos != 35 {
		t.Fatalf("partial compact summary = %+v", got[1])
	}
	if got[1].EstimatedCost.InputNanos == nil || *got[1].EstimatedCost.InputNanos != 30 ||
		got[1].EstimatedCost.CachedInputNanos == nil || *got[1].EstimatedCost.CachedInputNanos != 0 {
		t.Fatalf("partial components = %+v", got[1].EstimatedCost)
	}
}

func TestSummaryReaderGetPreservesStrongestPartialLowerBoundWithoutDoubleCounting(t *testing.T) {
	store := &usageSummaryStoreStub{
		found:      true,
		incomplete: true,
		session:    domain.SessionRecord{ID: "reverb-12", Harness: domain.HarnessCodex},
		models: []domain.UsageModelAggregate{
			{
				Harness: domain.HarnessClaudeCode, ModelID: "<synthetic>",
				Tokens: testUsageMetrics(0, 0, 0, 0),
			},
			{
				Harness: domain.HarnessCodex, ModelID: "gpt-5.6",
				Tokens: testUsageMetrics(1000, 400, 600, 200),
				Cost:   completeCostAggregate(1, 100, 20, 10, 70),
			},
			{
				Harness: domain.HarnessClaudeCode, ModelID: "claude-sonnet",
				Tokens: testUsageMetrics(100, 20, 80, 25),
				Cost: domain.UsageCostAggregate{
					EventCount:               1,
					ObservedCostEventCount:   1,
					KnownInputCount:          1,
					KnownInputNanos:          30,
					UnpricedKnownInputNanos:  30,
					KnownCachedInputCount:    1,
					KnownCachedInputNanos:    0,
					KnownOutputCount:         1,
					KnownOutputNanos:         5,
					UnpricedKnownOutputNanos: 5,
				},
			},
		},
	}

	got, err := NewSummaryReader(store).Get(context.Background(), "reverb-12")
	mustNoError(t, err)
	if !got.Incomplete {
		t.Fatal("token integrity failure did not remain independent from cost coverage")
	}
	if got.Totals.InputTokens == nil || *got.Totals.InputTokens != 1100 ||
		got.Totals.OutputTokens == nil || *got.Totals.OutputTokens != 225 ||
		got.Totals.ProcessedTokens == nil || *got.Totals.ProcessedTokens != 1325 {
		t.Fatalf("totals = %+v", got.Totals)
	}
	cost := got.Totals.EstimatedCost
	if cost == nil || cost.Coverage != domain.EstimatedCostCoveragePartial || cost.TotalNanos != 135 {
		t.Fatalf("session cost = %+v, want partial lower bound 100+30+5", cost)
	}
	if cost.InputNanos == nil || *cost.InputNanos != 50 ||
		cost.CachedInputNanos == nil || *cost.CachedInputNanos != 10 ||
		cost.OutputNanos == nil || *cost.OutputNanos != 75 {
		t.Fatalf("session component coverage = %+v", cost)
	}
	if len(got.Harnesses) != 2 || len(got.Harnesses[0].Models) != 1 || len(got.Harnesses[1].Models) != 1 ||
		got.Harnesses[0].Models[0].ModelID != "gpt-5.6" ||
		got.Harnesses[1].Models[0].ModelID != "claude-sonnet" {
		t.Fatalf("model grouping = %+v", got.Harnesses)
	}
	if got.Harnesses[0].Models[0].Totals.EstimatedCost == nil ||
		got.Harnesses[0].Models[0].Totals.EstimatedCost.Coverage != domain.EstimatedCostCoverageComplete ||
		got.Harnesses[1].Models[0].Totals.EstimatedCost == nil ||
		got.Harnesses[1].Models[0].Totals.EstimatedCost.TotalNanos != 35 {
		t.Fatalf("model costs = %+v", got.Harnesses)
	}
	for _, harness := range got.Harnesses {
		for _, model := range harness.Models {
			if model.ModelID == "<synthetic>" {
				t.Fatalf("synthetic model leaked into summary: %+v", got.Harnesses)
			}
		}
	}
	if got.Harnesses[0].Totals.ProcessedTokens == nil || *got.Harnesses[0].Totals.ProcessedTokens != 1200 ||
		got.Harnesses[0].Models[0].Totals.ProcessedTokens == nil || *got.Harnesses[0].Models[0].Totals.ProcessedTokens != 1200 ||
		got.Harnesses[1].Totals.ProcessedTokens == nil || *got.Harnesses[1].Totals.ProcessedTokens != 125 {
		t.Fatalf("processed totals by scope = %+v", got.Harnesses)
	}
	if store.calls != [6]int{0, 1, 1, 1, 0, 0} {
		t.Fatalf("store calls = %v", store.calls)
	}
}

// Break caught: inferred prices were aggregated into the same dollar value as
// observed prices without preserving the distinction the UI needs to explain
// that the billing provider has not been confirmed.
func TestSummaryReaderReportsCostProviderAttributionAtEveryScope(t *testing.T) {
	observed := completeCostAggregate(1, 100, 20, 10, 70)
	inferred := completeCostAggregate(1, 200, 40, 20, 140)
	inferred.ObservedCostEventCount = 0
	inferred.InferredCostEventCount = 1
	store := &usageSummaryStoreStub{
		found:   true,
		session: domain.SessionRecord{ID: "reverb-12", Harness: domain.HarnessClaudeCode},
		models: []domain.UsageModelAggregate{
			{Harness: domain.HarnessClaudeCode, ModelID: "claude-observed", Tokens: testUsageMetrics(1, 0, 1, 1), Cost: observed},
			{Harness: domain.HarnessClaudeCode, ModelID: "claude-inferred", Tokens: testUsageMetrics(1, 0, 1, 1), Cost: inferred},
		},
	}

	got, err := NewSummaryReader(store).Get(context.Background(), "reverb-12")
	mustNoError(t, err)
	if got.Totals.EstimatedCost == nil ||
		got.Totals.EstimatedCost.ProviderAttribution != domain.EstimatedCostProviderAttributionMixed {
		t.Fatalf("session attribution = %+v, want mixed", got.Totals.EstimatedCost)
	}
	if len(got.Harnesses) != 1 || got.Harnesses[0].Totals.EstimatedCost == nil ||
		got.Harnesses[0].Totals.EstimatedCost.ProviderAttribution != domain.EstimatedCostProviderAttributionMixed {
		t.Fatalf("harness attribution = %+v, want mixed", got.Harnesses)
	}
	models := got.Harnesses[0].Models
	if len(models) != 2 || models[0].Totals.EstimatedCost == nil || models[1].Totals.EstimatedCost == nil ||
		models[0].Totals.EstimatedCost.ProviderAttribution != domain.EstimatedCostProviderAttributionObserved ||
		models[1].Totals.EstimatedCost.ProviderAttribution != domain.EstimatedCostProviderAttributionInferred {
		t.Fatalf("model attributions = %+v, want observed then inferred", models)
	}
}

func TestSummaryReaderReturnsUnavailableCostForZeroPartialLowerBound(t *testing.T) {
	store := &usageSummaryStoreStub{rows: []domain.CompactSessionUsageAggregate{{
		SessionID: "unknown", Cost: domain.UsageCostAggregate{EventCount: 1},
	}}}
	got, err := NewSummaryReader(store).ListCompact(context.Background(), "")
	mustNoError(t, err)
	if len(got) != 1 || got[0].EstimatedCost != nil {
		t.Fatalf("cost = %+v, want unavailable", got)
	}
}

func TestSummaryReaderAnalyticsBucketsTimezoneFiltersAndNullTotals(t *testing.T) {
	loc := time.FixedZone("ICT", 7*3600)
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, loc)
	end := start.Add(48 * time.Hour)
	cost := completeCostAggregate(1, 100, 70, 10, 30)
	partialCost := domain.UsageCostAggregate{
		EventCount:               1,
		ObservedCostEventCount:   1,
		KnownInputCount:          1,
		KnownInputNanos:          20,
		UnpricedKnownInputNanos:  20,
		KnownCachedInputCount:    1,
		KnownCachedInputNanos:    0,
		KnownOutputCount:         1,
		KnownOutputNanos:         5,
		UnpricedKnownOutputNanos: 5,
	}
	store := &usageSummaryStoreStub{
		analytics: []domain.UsageAnalyticsAggregate{
			{
				BucketHourUTC: time.Date(2026, 9, 30, 17, 0, 0, 0, time.UTC),
				ProjectID:     "usage", ProjectName: "Usage",
				Harness: domain.HarnessCodex, ModelID: "gpt-5",
				Tokens: testUsageMetrics(100, 40, 60, 20), Cost: cost,
			},
			{
				BucketHourUTC: time.Date(2026, 9, 30, 18, 0, 0, 0, time.UTC),
				ProjectID:     "usage", ProjectName: "Usage",
				Harness: domain.HarnessClaudeCode, ModelID: "claude-sonnet",
				Tokens: testUsageMetrics(50, 0, 50, 10), Cost: partialCost,
			},
			{
				BucketHourUTC: time.Date(2026, 9, 30, 19, 0, 0, 0, time.UTC),
				ProjectID:     "other", ProjectName: "Other",
				Harness: domain.HarnessCodex, ModelID: "unknown-model",
				Tokens: domain.UsageTokenMetrics{OutputTokens: value(5)}, Cost: domain.UsageCostAggregate{EventCount: 1},
			},
		},
		coverage: domain.UsageAnalyticsCoverage{
			EventCount: 3, PricedEventCount: 1, UnpricedEventCount: 2,
			SessionCount: 2, IncompleteSessionCount: 1, SourceCount: 3, PartialSourceCount: 1,
			SupportedSources: []domain.UsageSourceKind{domain.UsageSourceCodexRollout},
		},
	}

	got, err := NewSummaryReader(store).GetAnalytics(context.Background(), domain.UsageAnalyticsFilter{
		ProjectID: "usage", Harness: domain.HarnessCodex, ModelID: "gpt-5",
		Start: start, End: end, Timezone: "Asia/Ho_Chi_Minh", Granularity: domain.UsageAnalyticsDay,
	})
	mustNoError(t, err)
	if store.calls != [6]int{0, 0, 0, 0, 1, 1} ||
		!store.filter.Start.Equal(start.UTC()) || !store.filter.End.Equal(end.UTC()) ||
		store.filter.ProjectID != "usage" || store.filter.Harness != domain.HarnessCodex ||
		store.filter.ModelID != "gpt-5" {
		t.Fatalf("store calls/filter = %v %+v", store.calls, store.filter)
	}
	if got.Timezone != "Asia/Ho_Chi_Minh" || got.Granularity != domain.UsageAnalyticsDay ||
		!got.Start.Equal(start) || !got.End.Equal(end) {
		t.Fatalf("range = %s %s %s %s", got.Start, got.End, got.Timezone, got.Granularity)
	}
	if got.Totals.InputTokens != nil || got.Totals.ProcessedTokens != nil {
		t.Fatalf("unknown event did not keep aggregate token totals nullable: %+v", got.Totals)
	}
	if got.Totals.OutputTokens == nil || *got.Totals.OutputTokens != 35 ||
		got.Totals.EstimatedCost == nil || got.Totals.EstimatedCost.TotalNanos != 125 ||
		got.Totals.EstimatedCost.Coverage != domain.EstimatedCostCoveragePartial {
		t.Fatalf("totals = %+v", got.Totals)
	}
	if len(got.DailyBuckets) != 2 || got.DailyBuckets[0].EventCount != 3 || got.DailyBuckets[1].EventCount != 0 {
		t.Fatalf("daily buckets = %+v", got.DailyBuckets)
	}
	if len(got.HourlyBuckets) != 48 || got.HourlyBuckets[0].EventCount != 1 || got.HourlyBuckets[1].EventCount != 1 ||
		got.HourlyBuckets[2].EventCount != 1 || got.HourlyBuckets[3].EventCount != 0 {
		t.Fatalf("hourly buckets len/events = %d %+v", len(got.HourlyBuckets), got.HourlyBuckets[:4])
	}
	if len(got.TimeSeries) != 2 || got.TimeSeries[0].EventCount != 3 || got.TimeSeries[1].EventCount != 0 {
		t.Fatalf("series = %+v", got.TimeSeries)
	}
	if len(got.Projects) != 2 || got.Projects[0].ProjectID != "usage" || got.Projects[1].ProjectID != "other" {
		t.Fatalf("projects = %+v", got.Projects)
	}
	if got.Coverage.UnpricedEventCount != 2 || got.Coverage.IncompleteSessionCount != 1 ||
		len(got.Coverage.SupportedSources) != 1 || got.Coverage.SupportedSources[0] != domain.UsageSourceCodexRollout {
		t.Fatalf("coverage = %+v", got.Coverage)
	}
}

func TestSummaryReaderAnalyticsRejectsInvalidRangeAndTimezone(t *testing.T) {
	reader := NewSummaryReader(&usageSummaryStoreStub{})
	_, err := reader.GetAnalytics(context.Background(), domain.UsageAnalyticsFilter{Timezone: "Not/AZone"})
	if err == nil {
		t.Fatal("invalid timezone returned nil error")
	}
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	_, err = reader.GetAnalytics(context.Background(), domain.UsageAnalyticsFilter{Start: now, End: now, Timezone: "UTC"})
	if err == nil {
		t.Fatal("invalid range returned nil error")
	}
}

func TestSummaryReaderRejectsAggregateOverflow(t *testing.T) {
	t.Run("partial lower bound", func(t *testing.T) {
		store := &usageSummaryStoreStub{rows: []domain.CompactSessionUsageAggregate{{
			SessionID: "overflow",
			Cost: domain.UsageCostAggregate{
				EventCount:              2,
				PricedEventCount:        1,
				PricedTotalNanos:        math.MaxInt64,
				KnownInputCount:         1,
				KnownInputNanos:         1,
				UnpricedKnownInputNanos: 1,
			},
		}}}
		if _, err := NewSummaryReader(store).ListCompact(context.Background(), ""); err == nil {
			t.Fatal("partial cost overflow returned nil error")
		}
	})

	t.Run("detail cost groups", func(t *testing.T) {
		store := &usageSummaryStoreStub{found: true, models: []domain.UsageModelAggregate{
			{
				Harness: domain.HarnessCodex, ModelID: "one",
				Tokens: testUsageMetrics(1, 0, 1, 0),
				Cost:   completeCostAggregate(1, math.MaxInt64, 0, 0, 0),
			},
			{
				Harness: domain.HarnessCodex, ModelID: "two",
				Tokens: testUsageMetrics(1, 0, 1, 0),
				Cost:   completeCostAggregate(1, 1, 0, 0, 0),
			},
		}}
		if _, err := NewSummaryReader(store).Get(context.Background(), "overflow"); err == nil {
			t.Fatal("detailed cost overflow returned nil error")
		}
	})
}

func testUsageMetrics(input, cachedInput, uncachedInput, output int64) domain.UsageTokenMetrics {
	return domain.UsageTokenMetrics{
		InputTokens: &input, CachedInputTokens: &cachedInput, UncachedInputTokens: &uncachedInput,
		OutputTokens: &output,
	}
}

func value(v int64) *int64 { return &v }

func TestSummaryReaderGetReturnsUnavailableMetricsWithoutEvents(t *testing.T) {
	store := &usageSummaryStoreStub{found: true, session: domain.SessionRecord{ID: "empty"}}
	got, err := NewSummaryReader(store).Get(context.Background(), "empty")
	mustNoError(t, err)
	if got.Totals.InputTokens != nil || got.Totals.OutputTokens != nil ||
		got.Totals.ProcessedTokens != nil || got.Totals.EstimatedCost != nil || len(got.Harnesses) != 0 {
		t.Fatalf("empty usage = %+v", got)
	}
}

func completeCostAggregate(events, total, input, cachedInput, output int64) domain.UsageCostAggregate {
	return domain.UsageCostAggregate{
		EventCount: events, PricedEventCount: events, PricedTotalNanos: total,
		ObservedCostEventCount: events,
		KnownInputCount:        events, KnownInputNanos: input,
		KnownCachedInputCount: events, KnownCachedInputNanos: cachedInput,
		KnownOutputCount: events, KnownOutputNanos: output,
	}
}
