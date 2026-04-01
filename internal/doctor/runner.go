package doctor

import (
	"context"
	"fmt"
	"time"
)

// Check is a single diagnostic test.
type Check struct {
	Name     string
	Category string
	Run      func(ctx context.Context) TestResult
}

// Runner orchestrates check execution and streams results.
type Runner struct {
	checks []Check
}

func NewRunner() *Runner { return &Runner{} }

func (r *Runner) Register(checks ...Check) {
	r.checks = append(r.checks, checks...)
}

// Run executes all checks sequentially and sends each result to the channel.
// The channel is closed when the run completes. Returns the full RunResult.
func (r *Runner) Run(ctx context.Context, results chan<- TestResult) *RunResult {
	defer close(results)

	run := &RunResult{
		ID:        fmt.Sprintf("%d", time.Now().UnixMilli()%100000),
		Status:    StatusRunning,
		StartedAt: time.Now(),
	}

	categoryMap := make(map[string]*CategoryResult)
	var categoryOrder []string

	for _, check := range r.checks {
		if ctx.Err() != nil {
			break
		}

		// Emit a "running" event so the UI can show a pending indicator.
		results <- TestResult{
			Name:     check.Name,
			Category: check.Category,
			Status:   StatusRunning,
		}

		start := time.Now()
		result := check.Run(ctx)
		result.Name = check.Name
		result.Category = check.Category
		result.Duration = time.Since(start)

		results <- result

		cat, ok := categoryMap[check.Category]
		if !ok {
			cat = &CategoryResult{Name: check.Category}
			categoryMap[check.Category] = cat
			categoryOrder = append(categoryOrder, check.Category)
		}
		cat.Tests = append(cat.Tests, result)

		switch result.Status {
		case StatusPass:
			run.Summary.Passed++
		case StatusFail:
			run.Summary.Failed++
		case StatusSkip:
			run.Summary.Skipped++
		}
		run.Summary.Total++
	}

	for _, name := range categoryOrder {
		run.Categories = append(run.Categories, *categoryMap[name])
	}

	run.Duration = time.Since(run.StartedAt)
	if run.Summary.Failed > 0 {
		run.Status = StatusFail
	} else {
		run.Status = StatusPass
	}

	return run
}
