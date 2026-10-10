package controllers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/config"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd"
)

type fakeUsageSummaryService struct {
	projectID domain.ProjectID
	sessionID domain.SessionID
	filter    domain.UsageAnalyticsFilter
	items     []domain.CompactSessionUsage
	detail    domain.SessionUsageSummary
	analytics domain.UsageAnalyticsSummary
	err       error
}

func (f *fakeUsageSummaryService) ListCompact(_ context.Context, projectID domain.ProjectID) ([]domain.CompactSessionUsage, error) {
	f.projectID = projectID
	return f.items, f.err
}

func (f *fakeUsageSummaryService) Get(_ context.Context, sessionID domain.SessionID) (domain.SessionUsageSummary, error) {
	f.sessionID = sessionID
	return f.detail, f.err
}

func (f *fakeUsageSummaryService) GetAnalytics(_ context.Context, filter domain.UsageAnalyticsFilter) (domain.UsageAnalyticsSummary, error) {
	f.filter = filter
	return f.analytics, f.err
}

func newUsageTestServer(t *testing.T, svc *fakeUsageSummaryService) *httptest.Server {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return newUsageTestServerWithLogger(t, svc, log)
}

func newUsageTestServerWithLogger(t *testing.T, svc *fakeUsageSummaryService, log *slog.Logger) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(httpd.NewRouterWithControl(config.Config{}, log, nil, httpd.APIDeps{UsageSummary: svc}, httpd.ControlDeps{}))
	t.Cleanup(srv.Close)
	return srv
}

func TestUsageAPIListsCompactProjectUsage(t *testing.T) {
	inputCost := int64(300000000)
	processed := int64(12300)
	unavailableProcessed := int64(3)
	svc := &fakeUsageSummaryService{items: []domain.CompactSessionUsage{
		{
			SessionID: "reverb-12", ProcessedTokens: &processed, Incomplete: true,
			EstimatedCost: &domain.EstimatedCost{
				TotalNanos: 420000000, InputNanos: &inputCost,
				Coverage:            domain.EstimatedCostCoveragePartial,
				ProviderAttribution: domain.EstimatedCostProviderAttributionInferred,
			},
		},
		{SessionID: "unavailable", ProcessedTokens: &unavailableProcessed},
	}}
	srv := newUsageTestServer(t, svc)

	body, status, _ := doRequest(t, srv, http.MethodGet, "/api/v1/usage/sessions?projectId=reverb", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", status, body)
	}
	if svc.projectID != "reverb" {
		t.Fatalf("project id = %q, want reverb", svc.projectID)
	}
	var got struct {
		Sessions []struct {
			SessionID       string          `json:"sessionId"`
			ProcessedTokens int64           `json:"processedTokens"`
			TotalTokens     int64           `json:"totalTokens"`
			Incomplete      bool            `json:"incomplete"`
			EstimatedCost   json.RawMessage `json:"estimatedCost"`
		} `json:"sessions"`
	}
	mustJSON(t, body, &got)
	if len(got.Sessions) != 2 || got.Sessions[0].SessionID != "reverb-12" ||
		got.Sessions[0].ProcessedTokens != 12300 || got.Sessions[0].TotalTokens != 12300 ||
		!got.Sessions[0].Incomplete {
		t.Fatalf("response = %+v", got)
	}
	var cost struct {
		TotalNanos          int64  `json:"totalNanos"`
		InputNanos          *int64 `json:"inputNanos"`
		CachedInputNanos    *int64 `json:"cachedInputNanos"`
		Coverage            string `json:"coverage"`
		ProviderAttribution string `json:"providerAttribution"`
	}
	mustJSON(t, got.Sessions[0].EstimatedCost, &cost)
	if cost.TotalNanos != 420000000 || cost.InputNanos == nil || *cost.InputNanos != 300000000 ||
		cost.CachedInputNanos != nil || cost.Coverage != "partial" || cost.ProviderAttribution != "inferred" {
		t.Fatalf("estimated cost = %+v", cost)
	}
	if string(got.Sessions[1].EstimatedCost) != "null" {
		t.Fatalf("unavailable estimatedCost = %s, want explicit null", got.Sessions[1].EstimatedCost)
	}
}

func TestUsageAPIShowsDetailedEstimatedCostAndProviderAttribution(t *testing.T) {
	input := int64(1000)
	uncached := int64(600)
	output := int64(200)
	zero := int64(0)
	cachedInput := int64(400)
	processed := int64(1200)
	svc := &fakeUsageSummaryService{detail: domain.SessionUsageSummary{
		SessionID: "reverb-12", Incomplete: true,
		Totals: domain.UsageMetricTotals{
			InputTokens: &input, CachedInputTokens: &cachedInput, UncachedInputTokens: &uncached,
			OutputTokens: &output, ProcessedTokens: &processed,
			EstimatedCost: &domain.EstimatedCost{
				TotalNanos: 135, InputNanos: &input, CachedInputNanos: &zero,
				OutputNanos: &output, Coverage: domain.EstimatedCostCoveragePartial,
				ProviderAttribution: domain.EstimatedCostProviderAttributionMixed,
			},
		},
		Harnesses: []domain.HarnessUsageSummary{{
			Harness: domain.HarnessCodex,
			Models: []domain.ModelUsageSummary{{
				ModelID: "gpt-5.6",
				Totals: domain.UsageMetricTotals{EstimatedCost: &domain.EstimatedCost{
					TotalNanos: 0, InputNanos: &zero, CachedInputNanos: &zero,
					OutputNanos: &zero, Coverage: domain.EstimatedCostCoverageComplete,
					ProviderAttribution: domain.EstimatedCostProviderAttributionObserved,
				}},
			}},
		}},
	}}
	srv := newUsageTestServer(t, svc)

	body, status, _ := doRequest(t, srv, http.MethodGet, "/api/v1/usage/sessions/reverb-12", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", status, body)
	}
	if svc.sessionID != "reverb-12" {
		t.Fatalf("session id = %q", svc.sessionID)
	}
	// Provider-shaped counters and per-metric provenance are no longer projected
	// onto this boundary; the bounded provider object owns them now.
	for _, forbidden := range []string{
		`"cost"`, `"valueNanos"`, `"pricingVersion"`,
		`"provenance"`, `"providerDetails"`, `"cacheWriteTokens"`, `"reasoningTokens"`,
	} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("detailed usage exposed %s: %s", forbidden, body)
		}
	}
	var got struct {
		SessionID  string `json:"sessionId"`
		Incomplete bool   `json:"incomplete"`
		Totals     struct {
			InputTokens         int64 `json:"inputTokens"`
			CachedInputTokens   int64 `json:"cachedInputTokens"`
			UncachedInputTokens int64 `json:"uncachedInputTokens"`
			OutputTokens        int64 `json:"outputTokens"`
			ProcessedTokens     int64 `json:"processedTokens"`
			CacheReadTokens     int64 `json:"cacheReadTokens"`
			EstimatedCost       struct {
				TotalNanos          int64  `json:"totalNanos"`
				InputNanos          *int64 `json:"inputNanos"`
				CachedInputNanos    *int64 `json:"cachedInputNanos"`
				Coverage            string `json:"coverage"`
				ProviderAttribution string `json:"providerAttribution"`
			} `json:"estimatedCost"`
		} `json:"totals"`
		Harnesses []struct {
			Models []struct {
				ProviderID string `json:"providerId"`
				ModelID    string `json:"modelId"`
				Totals     struct {
					EstimatedCost struct {
						TotalNanos          int64  `json:"totalNanos"`
						Coverage            string `json:"coverage"`
						ProviderAttribution string `json:"providerAttribution"`
					} `json:"estimatedCost"`
				} `json:"totals"`
			} `json:"models"`
		} `json:"harnesses"`
	}
	mustJSON(t, body, &got)
	if got.SessionID != "reverb-12" || !got.Incomplete || got.Totals.InputTokens != 1000 ||
		got.Totals.EstimatedCost.TotalNanos != 135 ||
		got.Totals.EstimatedCost.InputNanos == nil || *got.Totals.EstimatedCost.InputNanos != 1000 ||
		got.Totals.EstimatedCost.CachedInputNanos == nil || *got.Totals.EstimatedCost.CachedInputNanos != 0 ||
		got.Totals.EstimatedCost.Coverage != "partial" ||
		got.Totals.EstimatedCost.ProviderAttribution != "mixed" ||
		got.Totals.CachedInputTokens != 400 || got.Totals.UncachedInputTokens != 600 ||
		got.Totals.OutputTokens != 200 ||
		got.Totals.ProcessedTokens != 1200 || got.Totals.CacheReadTokens != 400 ||
		len(got.Harnesses) != 1 || len(got.Harnesses[0].Models) != 1 ||
		got.Harnesses[0].Models[0].ModelID != "gpt-5.6" ||
		got.Harnesses[0].Models[0].Totals.EstimatedCost.TotalNanos != 0 ||
		got.Harnesses[0].Models[0].Totals.EstimatedCost.Coverage != "complete" ||
		got.Harnesses[0].Models[0].Totals.EstimatedCost.ProviderAttribution != "observed" {
		t.Fatalf("response = %+v", got)
	}
}

func TestUsageAPIShowsAnalyticsBucketsAndCoverage(t *testing.T) {
	input := int64(100)
	output := int64(25)
	processed := int64(125)
	cost := int64(42)
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.FixedZone("ICT", 7*3600))
	end := start.Add(24 * time.Hour)
	svc := &fakeUsageSummaryService{analytics: domain.UsageAnalyticsSummary{
		Start: start, End: end, Timezone: "Asia/Ho_Chi_Minh", Granularity: domain.UsageAnalyticsDay,
		Totals: domain.UsageMetricTotals{
			InputTokens: &input, OutputTokens: &output, ProcessedTokens: &processed,
			EstimatedCost: &domain.EstimatedCost{
				TotalNanos: cost, Coverage: domain.EstimatedCostCoveragePartial,
				ProviderAttribution: domain.EstimatedCostProviderAttributionObserved,
			},
		},
		DailyBuckets: []domain.UsageAnalyticsBucket{{
			Start: start, End: end, EventCount: 2,
			Totals: domain.UsageMetricTotals{InputTokens: &input, OutputTokens: &output, ProcessedTokens: &processed},
		}},
		HourlyBuckets: []domain.UsageAnalyticsBucket{{Start: start, End: start.Add(time.Hour), EventCount: 1}},
		TimeSeries:    []domain.UsageAnalyticsBucket{{Start: start, End: end, EventCount: 2}},
		Projects: []domain.UsageAnalyticsProjectSummary{{
			ProjectID: "usage", ProjectName: "Usage", EventCount: 2,
			Totals: domain.UsageMetricTotals{ProcessedTokens: &processed},
		}},
		Models: []domain.UsageAnalyticsModelSummary{{
			Harness: domain.HarnessCodex, ModelID: "gpt-5", EventCount: 2,
			Totals: domain.UsageMetricTotals{ProcessedTokens: &processed},
		}},
		Harnesses: []domain.UsageAnalyticsHarnessSummary{{
			Harness: domain.HarnessCodex, EventCount: 2,
			Totals: domain.UsageMetricTotals{ProcessedTokens: &processed},
		}},
		Coverage: domain.UsageAnalyticsCoverage{
			EventCount: 2, PricedEventCount: 1, UnpricedEventCount: 1,
			SessionCount: 1, IncompleteSessionCount: 1, SourceCount: 1, PartialSourceCount: 1,
			SupportedSources: []domain.UsageSourceKind{domain.UsageSourceCodexRollout},
		},
	}}
	srv := newUsageTestServer(t, svc)

	body, status, _ := doRequest(t, srv, http.MethodGet, "/api/v1/usage/analytics?projectId=usage&harness=codex&modelId=gpt-5&start=2026-10-01&end=2026-10-02&timezone=Asia/Ho_Chi_Minh&granularity=day", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", status, body)
	}
	if svc.filter.ProjectID != "usage" || svc.filter.Harness != domain.HarnessCodex ||
		svc.filter.ModelID != "gpt-5" || svc.filter.Timezone != "Asia/Ho_Chi_Minh" ||
		svc.filter.Granularity != domain.UsageAnalyticsDay {
		t.Fatalf("filter = %+v", svc.filter)
	}
	var got struct {
		Timezone string `json:"timezone"`
		Totals   struct {
			ProcessedTokens int64 `json:"processedTokens"`
			EstimatedCost   struct {
				TotalNanos int64  `json:"totalNanos"`
				Coverage   string `json:"coverage"`
			} `json:"estimatedCost"`
		} `json:"totals"`
		DailyBuckets []struct {
			EventCount int64 `json:"eventCount"`
			Totals     struct {
				ProcessedTokens int64 `json:"processedTokens"`
			} `json:"totals"`
		} `json:"dailyBuckets"`
		Projects []struct {
			ProjectID   string `json:"projectId"`
			ProjectName string `json:"projectName"`
		} `json:"projects"`
		Coverage struct {
			UnpricedEventCount     int64    `json:"unpricedEventCount"`
			IncompleteSessionCount int64    `json:"incompleteSessionCount"`
			SupportedSources       []string `json:"supportedSources"`
		} `json:"coverage"`
	}
	mustJSON(t, body, &got)
	if got.Timezone != "Asia/Ho_Chi_Minh" || got.Totals.ProcessedTokens != 125 ||
		got.Totals.EstimatedCost.TotalNanos != 42 || got.Totals.EstimatedCost.Coverage != "partial" ||
		len(got.DailyBuckets) != 1 || got.DailyBuckets[0].EventCount != 2 ||
		got.DailyBuckets[0].Totals.ProcessedTokens != 125 ||
		len(got.Projects) != 1 || got.Projects[0].ProjectID != "usage" || got.Projects[0].ProjectName != "Usage" ||
		got.Coverage.UnpricedEventCount != 1 || got.Coverage.IncompleteSessionCount != 1 ||
		len(got.Coverage.SupportedSources) != 1 || got.Coverage.SupportedSources[0] != "codex_rollout" {
		t.Fatalf("analytics response = %+v", got)
	}
}

func TestUsageAPILogsServiceErrors(t *testing.T) {
	var logs bytes.Buffer
	srv := newUsageTestServerWithLogger(t, &fakeUsageSummaryService{err: errors.New("usage unavailable")}, slog.New(slog.NewTextHandler(&logs, nil)))

	for _, path := range []string{"/api/v1/usage/sessions?projectId=reverb", "/api/v1/usage/sessions/reverb-12"} {
		body, status, _ := doRequest(t, srv, http.MethodGet, path, "")
		if status != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body=%s", status, body)
		}
	}
	for _, message := range []string{"failed to list compact session usage", "failed to get session usage"} {
		if !strings.Contains(logs.String(), message) {
			t.Fatalf("logs = %q, want %q", logs.String(), message)
		}
	}
}
