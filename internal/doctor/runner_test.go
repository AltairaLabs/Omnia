package doctor

import (
	"context"
	"testing"
)

func passCheck(name, category string) Check {
	return Check{
		Name:     name,
		Category: category,
		Run: func(_ context.Context) TestResult {
			return TestResult{Status: StatusPass, Detail: "ok"}
		},
	}
}

func failCheck(name, category string) Check {
	return Check{
		Name:     name,
		Category: category,
		Run: func(_ context.Context) TestResult {
			return TestResult{Status: StatusFail, Error: "something broke"}
		},
	}
}

func skipCheck(name, category string) Check {
	return Check{
		Name:     name,
		Category: category,
		Run: func(_ context.Context) TestResult {
			return TestResult{Status: StatusSkip, Detail: "skipped"}
		},
	}
}

func collectResults(ch <-chan TestResult) []TestResult {
	results := make([]TestResult, 0, cap(ch))
	for r := range ch {
		results = append(results, r)
	}
	return results
}

func TestRunner_AllPass(t *testing.T) {
	r := NewRunner()
	r.Register(
		passCheck("check-a", "cat1"),
		passCheck("check-b", "cat1"),
		passCheck("check-c", "cat2"),
	)

	ch := make(chan TestResult, 10)
	run := r.Run(context.Background(), ch)
	results := collectResults(ch)

	if run.Status != StatusPass {
		t.Errorf("expected StatusPass, got %s", run.Status)
	}
	if run.Summary.Total != 3 {
		t.Errorf("expected Total=3, got %d", run.Summary.Total)
	}
	if run.Summary.Passed != 3 {
		t.Errorf("expected Passed=3, got %d", run.Summary.Passed)
	}
	if run.Summary.Failed != 0 {
		t.Errorf("expected Failed=0, got %d", run.Summary.Failed)
	}
	// Each check emits a "running" event + final result = 6 total
	if len(results) != 6 {
		t.Errorf("expected 6 results from channel (3 running + 3 final), got %d", len(results))
	}
}

func TestRunner_WithFailure(t *testing.T) {
	r := NewRunner()
	r.Register(
		passCheck("check-a", "cat1"),
		failCheck("check-b", "cat1"),
	)

	ch := make(chan TestResult, 10)
	run := r.Run(context.Background(), ch)
	collectResults(ch)

	if run.Status != StatusFail {
		t.Errorf("expected StatusFail, got %s", run.Status)
	}
	if run.Summary.Failed != 1 {
		t.Errorf("expected Failed=1, got %d", run.Summary.Failed)
	}
}

func TestRunner_WithSkip(t *testing.T) {
	r := NewRunner()
	r.Register(
		passCheck("check-a", "cat1"),
		skipCheck("check-b", "cat1"),
	)

	ch := make(chan TestResult, 10)
	run := r.Run(context.Background(), ch)
	collectResults(ch)

	if run.Summary.Skipped != 1 {
		t.Errorf("expected Skipped=1, got %d", run.Summary.Skipped)
	}
}

func TestRunner_CategoryOrder(t *testing.T) {
	r := NewRunner()
	r.Register(
		passCheck("check-a", "zcat"),
		passCheck("check-b", "acat"),
		passCheck("check-c", "zcat"),
	)

	ch := make(chan TestResult, 10)
	run := r.Run(context.Background(), ch)
	collectResults(ch)

	if len(run.Categories) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(run.Categories))
	}
	if run.Categories[0].Name != "zcat" {
		t.Errorf("expected first category 'zcat', got '%s'", run.Categories[0].Name)
	}
	if run.Categories[1].Name != "acat" {
		t.Errorf("expected second category 'acat', got '%s'", run.Categories[1].Name)
	}
	if len(run.Categories[0].Tests) != 2 {
		t.Errorf("expected 2 tests in zcat, got %d", len(run.Categories[0].Tests))
	}
}

func TestRunner_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	r := NewRunner()
	r.Register(
		passCheck("check-a", "cat1"),
		passCheck("check-b", "cat1"),
	)

	ch := make(chan TestResult, 10)
	run := r.Run(ctx, ch)
	results := collectResults(ch)

	// With a pre-cancelled context, no checks should run
	if len(results) != 0 {
		t.Errorf("expected 0 results with cancelled context, got %d", len(results))
	}
	if run.Summary.Total != 0 {
		t.Errorf("expected Total=0 with cancelled context, got %d", run.Summary.Total)
	}
}

func TestRunner_ResultFieldsPopulated(t *testing.T) {
	r := NewRunner()
	r.Register(Check{
		Name:     "my-check",
		Category: "my-cat",
		Run: func(_ context.Context) TestResult {
			return TestResult{Status: StatusPass}
		},
	})

	ch := make(chan TestResult, 10)
	r.Run(context.Background(), ch)
	results := collectResults(ch)

	if len(results) != 2 {
		t.Fatalf("expected 2 results (running + final), got %d", len(results))
	}
	// First event is "running"
	if results[0].Status != StatusRunning {
		t.Errorf("expected first event StatusRunning, got %s", results[0].Status)
	}
	if results[0].Name != "my-check" {
		t.Errorf("expected running event Name='my-check', got '%s'", results[0].Name)
	}
	// Second event is the final result
	res := results[1]
	if res.Name != "my-check" {
		t.Errorf("expected Name='my-check', got '%s'", res.Name)
	}
	if res.Category != "my-cat" {
		t.Errorf("expected Category='my-cat', got '%s'", res.Category)
	}
	if res.Duration == 0 {
		t.Error("expected Duration > 0")
	}
}

func TestNewRunner(t *testing.T) {
	r := NewRunner()
	if r == nil {
		t.Error("expected non-nil runner")
	}
}
