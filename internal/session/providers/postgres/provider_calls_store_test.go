/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session/api"
)

const (
	pcNamespaceDefault  = "default"
	pcAgentChatbot      = "chatbot"
	pcAgentSupport      = "support"
	pcProviderOpenAI    = "openai"
	pcProviderAnthropic = "anthropic"
	pcModelGPT4         = "gpt-4"
	pcModelGPT4Mini     = "gpt-4o-mini"
	pcModelSonnet       = "claude-3-5-sonnet"
	// Provider CRD names. Both openai-* are the same provider type ("openai")
	// but distinct CRDs — the case the provider_name dimension must split.
	pcProviderNameOpenAIPrimary = "openai-primary"
	pcProviderNameOpenAICheap   = "openai-cheap"
	pcProviderNameAnthropicMain = "anthropic-main"
)

func newProviderCallsStore(t *testing.T) (*ProviderCallsStoreImpl, *EvalStoreImpl) {
	t.Helper()
	pool := freshDB(t)
	return NewProviderCallsStore(pool), NewEvalStore(pool)
}

// seedSessionWithAgent inserts a session with the given namespace + agent so
// the provider_calls FK + the namespace/agent JOIN have data to match.
func seedSessionWithAgent(t *testing.T, store *EvalStoreImpl, sessionID, namespace, agent string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	_, err := store.pool.Exec(ctx, `INSERT INTO sessions (
		id, agent_name, namespace, workspace_name, status,
		created_at, updated_at, message_count, tool_call_count,
		total_input_tokens, total_output_tokens, estimated_cost_usd, tags
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		sessionID, agent, namespace, namespace, "active",
		now, now, 0, 0, 0, 0, 0, []string{},
	)
	require.NoError(t, err)
}

type pcRow struct {
	sessionID    string
	namespace    string
	agentName    string
	provider     string
	providerName string
	model        string
	inputTokens  int64
	outputTokens int64
	cachedTokens int64
	costUSD      float64
	durationMs   int64
	createdAt    time.Time
}

func insertProviderCall(t *testing.T, store *ProviderCallsStoreImpl, r pcRow) {
	t.Helper()
	id := uuid.New().String()
	var providerName any
	if r.providerName != "" {
		providerName = r.providerName
	}
	_, err := store.pool.Exec(context.Background(), `
		INSERT INTO provider_calls (
			id, session_id, namespace, agent_name, provider, provider_name, model, status,
			input_tokens, output_tokens, cached_tokens, cost_usd, duration_ms,
			created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'completed', $8, $9, $10, $11, $12, $13)`,
		id, r.sessionID, r.namespace, r.agentName, r.provider, providerName, r.model,
		r.inputTokens, r.outputTokens, r.cachedTokens, r.costUSD, r.durationMs,
		r.createdAt,
	)
	require.NoError(t, err)
}

// pcFixtureDay1/Day2 anchor the fixture's two distinct days relative to "now"
// so they always land inside the provider_calls rolling partition window
// (CURRENT_DATE-28 .. CURRENT_DATE+14, set in migration 000017). Hardcoded
// calendar dates eventually slide out of that window once enough time passes
// and inserts fail with "no partition of relation provider_calls found for
// row" (SQLSTATE 23514). Anchored at noon UTC, 8/7 days ago, so the UTC date
// (and thus the GroupByTimeDay key) is stable regardless of run time.
var (
	pcFixtureDay1 = time.Now().UTC().AddDate(0, 0, -8).Truncate(24 * time.Hour).Add(12 * time.Hour)
	pcFixtureDay2 = pcFixtureDay1.AddDate(0, 0, 1)
)

// seedProviderCallsFixture: two sessions in the `default` namespace with
// chatbot + support agents, calls across two days, two providers, three
// models, with deterministic token + cost values for assertion math.
func seedProviderCallsFixture(t *testing.T, pcStore *ProviderCallsStoreImpl, evalStore *EvalStoreImpl) {
	t.Helper()
	const (
		sessChatbot = "11111111-1111-1111-1111-111111111111"
		sessSupport = "22222222-2222-2222-2222-222222222222"
	)
	seedSessionWithAgent(t, evalStore, sessChatbot, pcNamespaceDefault, pcAgentChatbot)
	seedSessionWithAgent(t, evalStore, sessSupport, pcNamespaceDefault, pcAgentSupport)

	day1 := pcFixtureDay1
	day2 := pcFixtureDay2

	rows := []pcRow{
		// chatbot · openai-primary gpt-4 — day1: 100 in / 200 out / $0.01, duration 100ms
		{sessChatbot, pcNamespaceDefault, pcAgentChatbot, pcProviderOpenAI, pcProviderNameOpenAIPrimary, pcModelGPT4, 100, 200, 0, 0.01, 100, day1},
		// chatbot · openai-primary gpt-4 — day1: 150 in / 250 out / $0.02, duration 200ms
		{sessChatbot, pcNamespaceDefault, pcAgentChatbot, pcProviderOpenAI, pcProviderNameOpenAIPrimary, pcModelGPT4, 150, 250, 50, 0.02, 200, day1},
		// chatbot · openai-cheap gpt-4o-mini — day2: 50 / 100 / $0.001, duration 80ms
		{sessChatbot, pcNamespaceDefault, pcAgentChatbot, pcProviderOpenAI, pcProviderNameOpenAICheap, pcModelGPT4Mini, 50, 100, 0, 0.001, 80, day2},
		// support · anthropic-main sonnet — day2: 300 / 500 / $0.05, duration 500ms
		{sessSupport, pcNamespaceDefault, pcAgentSupport, pcProviderAnthropic, pcProviderNameAnthropicMain, pcModelSonnet, 300, 500, 0, 0.05, 500, day2},
	}
	for _, r := range rows {
		insertProviderCall(t, pcStore, r)
	}
}

// --- AggregateProviderCalls -------------------------------------------------

func TestAggregateProviderCalls_GroupByProvider_Count(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pcStore, evalStore := newProviderCallsStore(t)
	seedProviderCallsFixture(t, pcStore, evalStore)

	rows, err := pcStore.AggregateProviderCalls(context.Background(), api.ProviderCallAggregateOpts{
		Namespace: pcNamespaceDefault,
		GroupBy:   []api.ProviderCallAggregateGroupBy{api.ProviderCallAggregateGroupByProvider},
		Metric:    api.ProviderCallAggregateMetricCount,
	})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	byKey := map[string]*api.ProviderCallAggregateRow{}
	for _, r := range rows {
		byKey[r.Key] = r
	}
	assert.InDelta(t, 3, byKey[pcProviderOpenAI].Value, 0.001)
	assert.InDelta(t, 1, byKey[pcProviderAnthropic].Value, 0.001)
}

// TestAggregateProviderCalls_GroupByProviderName_Count proves the fix for the
// "all providers show the same numbers" bug: two providers of the same type
// ("openai") are attributed separately by their CRD name.
func TestAggregateProviderCalls_GroupByProviderName_Count(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pcStore, evalStore := newProviderCallsStore(t)
	seedProviderCallsFixture(t, pcStore, evalStore)

	rows, err := pcStore.AggregateProviderCalls(context.Background(), api.ProviderCallAggregateOpts{
		Namespace: pcNamespaceDefault,
		GroupBy:   []api.ProviderCallAggregateGroupBy{api.ProviderCallAggregateGroupByProviderName},
		Metric:    api.ProviderCallAggregateMetricCount,
	})
	require.NoError(t, err)
	require.Len(t, rows, 3)
	byKey := map[string]float64{}
	for _, r := range rows {
		byKey[r.Key] = r.Value
	}
	// Same-type openai providers no longer collapse: primary has 2 calls, cheap 1.
	assert.InDelta(t, 2, byKey[pcProviderNameOpenAIPrimary], 0.001)
	assert.InDelta(t, 1, byKey[pcProviderNameOpenAICheap], 0.001)
	assert.InDelta(t, 1, byKey[pcProviderNameAnthropicMain], 0.001)
}

// TestAggregateProviderCalls_FilterByProviderName narrows to a single CRD.
func TestAggregateProviderCalls_FilterByProviderName(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pcStore, evalStore := newProviderCallsStore(t)
	seedProviderCallsFixture(t, pcStore, evalStore)

	rows, err := pcStore.AggregateProviderCalls(context.Background(), api.ProviderCallAggregateOpts{
		Namespace:    pcNamespaceDefault,
		ProviderName: pcProviderNameOpenAIPrimary,
		GroupBy:      []api.ProviderCallAggregateGroupBy{api.ProviderCallAggregateGroupByProvider},
		Metric:       api.ProviderCallAggregateMetricCount,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, pcProviderOpenAI, rows[0].Key)
	assert.InDelta(t, 2, rows[0].Value, 0.001)
}

func TestAggregateProviderCalls_GroupByAgent_SumCostUSD(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pcStore, evalStore := newProviderCallsStore(t)
	seedProviderCallsFixture(t, pcStore, evalStore)

	rows, err := pcStore.AggregateProviderCalls(context.Background(), api.ProviderCallAggregateOpts{
		Namespace: pcNamespaceDefault,
		GroupBy:   []api.ProviderCallAggregateGroupBy{api.ProviderCallAggregateGroupByAgent},
		Metric:    api.ProviderCallAggregateMetricSumCostUSD,
	})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	byKey := map[string]float64{}
	for _, r := range rows {
		byKey[r.Key] = r.Value
	}
	// chatbot: 0.01 + 0.02 + 0.001 = 0.031
	assert.InDelta(t, 0.031, byKey[pcAgentChatbot], 0.0001)
	// support: 0.05
	assert.InDelta(t, 0.05, byKey[pcAgentSupport], 0.0001)
}

func TestAggregateProviderCalls_SumTokens(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pcStore, evalStore := newProviderCallsStore(t)
	seedProviderCallsFixture(t, pcStore, evalStore)

	rows, err := pcStore.AggregateProviderCalls(context.Background(), api.ProviderCallAggregateOpts{
		Namespace: pcNamespaceDefault,
		AgentName: pcAgentChatbot,
		GroupBy:   []api.ProviderCallAggregateGroupBy{api.ProviderCallAggregateGroupByAgent},
		Metric:    api.ProviderCallAggregateMetricSumTokens,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	// chatbot input+output: (100+200) + (150+250) + (50+100) = 850
	assert.InDelta(t, 850, rows[0].Value, 0.001)
}

func TestAggregateProviderCalls_TimeDay_CostSeries(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pcStore, evalStore := newProviderCallsStore(t)
	seedProviderCallsFixture(t, pcStore, evalStore)

	rows, err := pcStore.AggregateProviderCalls(context.Background(), api.ProviderCallAggregateOpts{
		Namespace: pcNamespaceDefault,
		AgentName: pcAgentChatbot,
		GroupBy:   []api.ProviderCallAggregateGroupBy{api.ProviderCallAggregateGroupByTimeDay},
		Metric:    api.ProviderCallAggregateMetricSumCostUSD,
	})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	// Sorted ASC by time.
	assert.Equal(t, pcFixtureDay1.Format("2006-01-02"), rows[0].Key)
	assert.InDelta(t, 0.03, rows[0].Value, 0.0001)
	assert.Equal(t, pcFixtureDay2.Format("2006-01-02"), rows[1].Key)
	assert.InDelta(t, 0.001, rows[1].Value, 0.0001)
}

func TestAggregateProviderCalls_P95Duration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pcStore, evalStore := newProviderCallsStore(t)
	seedProviderCallsFixture(t, pcStore, evalStore)

	rows, err := pcStore.AggregateProviderCalls(context.Background(), api.ProviderCallAggregateOpts{
		Namespace: pcNamespaceDefault,
		AgentName: pcAgentChatbot,
		GroupBy:   []api.ProviderCallAggregateGroupBy{api.ProviderCallAggregateGroupByAgent},
		Metric:    api.ProviderCallAggregateMetricP95DurationMs,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	// chatbot durations: [80, 100, 200] sorted; p95 lies between 100 and 200.
	assert.GreaterOrEqual(t, rows[0].Value, 100.0)
	assert.LessOrEqual(t, rows[0].Value, 200.0)
}

func TestAggregateProviderCalls_FilterByProviderAndModel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pcStore, evalStore := newProviderCallsStore(t)
	seedProviderCallsFixture(t, pcStore, evalStore)

	rows, err := pcStore.AggregateProviderCalls(context.Background(), api.ProviderCallAggregateOpts{
		Namespace: pcNamespaceDefault,
		Provider:  pcProviderOpenAI,
		Model:     pcModelGPT4,
		GroupBy:   []api.ProviderCallAggregateGroupBy{api.ProviderCallAggregateGroupByModel},
		Metric:    api.ProviderCallAggregateMetricCount,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, pcModelGPT4, rows[0].Key)
	assert.InDelta(t, 2, rows[0].Value, 0.001)
}

func TestAggregateProviderCalls_FilterTimeRange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pcStore, evalStore := newProviderCallsStore(t)
	seedProviderCallsFixture(t, pcStore, evalStore)

	// Cover day1 (the two openai calls) but exclude day2: [day1 00:00, day2 00:00).
	day1Start := pcFixtureDay1.Truncate(24 * time.Hour)
	day1End := day1Start.AddDate(0, 0, 1)
	rows, err := pcStore.AggregateProviderCalls(context.Background(), api.ProviderCallAggregateOpts{
		Namespace: pcNamespaceDefault,
		From:      day1Start,
		To:        day1End,
		GroupBy:   []api.ProviderCallAggregateGroupBy{api.ProviderCallAggregateGroupByProvider},
		Metric:    api.ProviderCallAggregateMetricCount,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, pcProviderOpenAI, rows[0].Key)
	assert.InDelta(t, 2, rows[0].Value, 0.001)
}

func TestAggregateProviderCalls_NamespaceIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pcStore, evalStore := newProviderCallsStore(t)
	seedProviderCallsFixture(t, pcStore, evalStore)

	rows, err := pcStore.AggregateProviderCalls(context.Background(), api.ProviderCallAggregateOpts{
		Namespace: "other-namespace",
		GroupBy:   []api.ProviderCallAggregateGroupBy{api.ProviderCallAggregateGroupByProvider},
		Metric:    api.ProviderCallAggregateMetricCount,
	})
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestAggregateProviderCalls_MissingNamespace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pcStore, _ := newProviderCallsStore(t)
	_, err := pcStore.AggregateProviderCalls(context.Background(), api.ProviderCallAggregateOpts{
		GroupBy: []api.ProviderCallAggregateGroupBy{api.ProviderCallAggregateGroupByProvider},
		Metric:  api.ProviderCallAggregateMetricCount,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace is required")
}

func TestAggregateProviderCalls_InvalidGroupBy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pcStore, _ := newProviderCallsStore(t)
	_, err := pcStore.AggregateProviderCalls(context.Background(), api.ProviderCallAggregateOpts{
		Namespace: pcNamespaceDefault,
		GroupBy:   []api.ProviderCallAggregateGroupBy{"not-a-groupby"},
		Metric:    api.ProviderCallAggregateMetricCount,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid groupBy")
}

func TestAggregateProviderCalls_CompoundProviderModelAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pcStore, evalStore := newProviderCallsStore(t)
	seedProviderCallsFixture(t, pcStore, evalStore)

	rows, err := pcStore.AggregateProviderCalls(context.Background(), api.ProviderCallAggregateOpts{
		Namespace: pcNamespaceDefault,
		GroupBy: []api.ProviderCallAggregateGroupBy{
			api.ProviderCallAggregateGroupByProvider,
			api.ProviderCallAggregateGroupByModel,
			api.ProviderCallAggregateGroupByAgent,
		},
		Metric: api.ProviderCallAggregateMetricSumCostUSD,
	})
	require.NoError(t, err)
	// 3 distinct (provider,model,agent) combinations in the fixture.
	require.Len(t, rows, 3)
	byKey := map[string]*api.ProviderCallAggregateRow{}
	for _, r := range rows {
		byKey[r.Key] = r
	}
	// chatbot · openai gpt-4: 0.01 + 0.02 across 2 calls.
	row := byKey[pcProviderOpenAI+"|"+pcModelGPT4+"|"+pcAgentChatbot]
	require.NotNil(t, row)
	assert.InDelta(t, 0.03, row.Value, 0.0001)
	assert.Equal(t, int64(2), row.Count)
	// support · anthropic sonnet: single 0.05 call.
	assert.InDelta(t, 0.05, byKey[pcProviderAnthropic+"|"+pcModelSonnet+"|"+pcAgentSupport].Value, 0.0001)
}

func TestAggregateProviderCalls_CompoundTimeDayProvider(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pcStore, evalStore := newProviderCallsStore(t)
	seedProviderCallsFixture(t, pcStore, evalStore)

	rows, err := pcStore.AggregateProviderCalls(context.Background(), api.ProviderCallAggregateOpts{
		Namespace: pcNamespaceDefault,
		GroupBy: []api.ProviderCallAggregateGroupBy{
			api.ProviderCallAggregateGroupByTimeDay,
			api.ProviderCallAggregateGroupByProvider,
		},
		Metric: api.ProviderCallAggregateMetricSumCostUSD,
	})
	require.NoError(t, err)
	// day1: openai (2 calls); day2: openai + anthropic -> 3 composite buckets.
	require.Len(t, rows, 3)
	// Composite keys are "<YYYY-MM-DD>|<provider>" and sort ASC by key (time present).
	for _, r := range rows {
		assert.Regexp(t, `^\d{4}-\d{2}-\d{2}\|(openai|anthropic)$`, r.Key)
	}
	assert.Equal(t, pcFixtureDay1.Format("2006-01-02")+"|"+pcProviderOpenAI, rows[0].Key)
	assert.InDelta(t, 0.03, rows[0].Value, 0.0001)
}

func TestAggregateProviderCalls_EmptyGroupBy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pcStore, _ := newProviderCallsStore(t)
	_, err := pcStore.AggregateProviderCalls(context.Background(), api.ProviderCallAggregateOpts{
		Namespace: pcNamespaceDefault,
		GroupBy:   []api.ProviderCallAggregateGroupBy{},
		Metric:    api.ProviderCallAggregateMetricCount,
	})
	require.Error(t, err)
}

func TestAggregateProviderCalls_InvalidMetric(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pcStore, _ := newProviderCallsStore(t)
	_, err := pcStore.AggregateProviderCalls(context.Background(), api.ProviderCallAggregateOpts{
		Namespace: pcNamespaceDefault,
		GroupBy:   []api.ProviderCallAggregateGroupBy{api.ProviderCallAggregateGroupByProvider},
		Metric:    "not-a-metric",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid metric")
}

// --- helpers --------------------------------------------------------------

func TestClampProviderCallAggregateLimit(t *testing.T) {
	t.Run("zero returns default", func(t *testing.T) {
		assert.Equal(t, api.DefaultProviderCallAggregateLimit, clampProviderCallAggregateLimit(0))
	})
	t.Run("negative returns default", func(t *testing.T) {
		assert.Equal(t, api.DefaultProviderCallAggregateLimit, clampProviderCallAggregateLimit(-5))
	})
	t.Run("within range passes through", func(t *testing.T) {
		assert.Equal(t, 42, clampProviderCallAggregateLimit(42))
	})
	t.Run("above max clamps to max", func(t *testing.T) {
		assert.Equal(t, api.MaxProviderCallAggregateLimit,
			clampProviderCallAggregateLimit(api.MaxProviderCallAggregateLimit+1000))
	})
}

func TestBuildProviderCallAggregateFilters(t *testing.T) {
	t.Run("namespace only", func(t *testing.T) {
		qb := buildProviderCallAggregateFilters(api.ProviderCallAggregateOpts{Namespace: "ns"})
		assert.Equal(t, []any{"ns"}, qb.Args())
		assert.Contains(t, qb.Where(), "pc.namespace=$1")
	})
	t.Run("all filters set", func(t *testing.T) {
		from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
		qb := buildProviderCallAggregateFilters(api.ProviderCallAggregateOpts{
			Namespace: "ns",
			AgentName: pcAgentChatbot,
			Provider:  pcProviderOpenAI,
			Model:     pcModelGPT4,
			From:      from,
			To:        to,
		})
		args := qb.Args()
		assert.Contains(t, args, "ns")
		assert.Contains(t, args, pcAgentChatbot)
		assert.Contains(t, args, pcProviderOpenAI)
		assert.Contains(t, args, pcModelGPT4)
		assert.Contains(t, args, from)
		assert.Contains(t, args, to)
	})
}

// --- ProviderCallsDiscovery ------------------------------------------------

func TestProviderCallsDiscovery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pcStore, evalStore := newProviderCallsStore(t)
	seedProviderCallsFixture(t, pcStore, evalStore)

	res, err := pcStore.ProviderCallsDiscovery(context.Background(), pcNamespaceDefault)
	require.NoError(t, err)
	require.NotNil(t, res)
	// Providers sorted alphabetically: anthropic, openai.
	assert.Equal(t, []string{pcProviderAnthropic, pcProviderOpenAI}, res.Providers)
	// Provider CRD names sorted: anthropic-main, openai-cheap, openai-primary.
	assert.Equal(t, []string{pcProviderNameAnthropicMain, pcProviderNameOpenAICheap, pcProviderNameOpenAIPrimary}, res.ProviderNames)
	// Models sorted: claude-3-5-sonnet, gpt-4, gpt-4o-mini.
	assert.Equal(t, []string{pcModelSonnet, pcModelGPT4, pcModelGPT4Mini}, res.Models)
}

func TestProviderCallsDiscovery_NamespaceIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pcStore, evalStore := newProviderCallsStore(t)
	seedProviderCallsFixture(t, pcStore, evalStore)

	res, err := pcStore.ProviderCallsDiscovery(context.Background(), "other-namespace")
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Empty(t, res.Providers)
	assert.Empty(t, res.ProviderNames)
	assert.Empty(t, res.Models)
}

func TestProviderCallsDiscovery_MissingNamespace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pcStore, _ := newProviderCallsStore(t)
	_, err := pcStore.ProviderCallsDiscovery(context.Background(), "")
	require.Error(t, err)
}

func TestProviderCallsDiscovery_SkipsEmptyProviderAndModel(t *testing.T) {
	// Insert a row with empty model — should not appear in distinct results.
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pcStore, evalStore := newProviderCallsStore(t)
	sess := "33333333-3333-3333-3333-333333333333"
	seedSessionWithAgent(t, evalStore, sess, pcNamespaceDefault, pcAgentChatbot)
	insertProviderCall(t, pcStore, pcRow{
		sessionID: sess, namespace: pcNamespaceDefault, agentName: pcAgentChatbot,
		provider: pcProviderOpenAI, model: "",
		inputTokens: 1, outputTokens: 1, costUSD: 0, durationMs: 1,
		createdAt: time.Now().UTC(),
	})

	res, err := pcStore.ProviderCallsDiscovery(context.Background(), pcNamespaceDefault)
	require.NoError(t, err)
	require.NotNil(t, res)
	for _, m := range res.Models {
		assert.NotEmpty(t, m, "empty model should be filtered out")
	}
}
