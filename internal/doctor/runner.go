package doctor

import (
	"context"
	"fmt"
	"sync"
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
	checks     []Check
	sequential map[string]string // category → group: categories in the same group run sequentially
}

func NewRunner() *Runner { return &Runner{sequential: make(map[string]string)} }

// SequentialGroup declares that the given categories must run sequentially
// in registration order (they share state). Categories in different groups
// or ungrouped categories run in parallel.
func (r *Runner) SequentialGroup(group string, categories ...string) {
	for _, cat := range categories {
		r.sequential[cat] = group
	}
}

func (r *Runner) Register(checks ...Check) {
	r.checks = append(r.checks, checks...)
}

// Run executes checks grouped by category. Categories run in parallel;
// checks within a category run sequentially (they may have ordering deps).
// Results are streamed to the channel as they complete. The channel is
// closed when all categories finish. Returns the full RunResult.
func (r *Runner) Run(ctx context.Context, results chan<- TestResult) *RunResult {
	defer close(results)

	run := &RunResult{
		ID:        fmt.Sprintf("%d", time.Now().UnixMilli()%100000),
		Status:    StatusRunning,
		StartedAt: time.Now(),
	}

	// Group checks by category, preserving registration order within each.
	type categoryEntry struct {
		name   string
		order  int
		checks []Check
	}
	var categories []categoryEntry
	catIndex := make(map[string]int)
	for _, check := range r.checks {
		idx, ok := catIndex[check.Category]
		if !ok {
			idx = len(categories)
			catIndex[check.Category] = idx
			categories = append(categories, categoryEntry{name: check.Category, order: idx})
		}
		categories[idx].checks = append(categories[idx].checks, check)
	}

	// Build execution groups: sequential groups share a goroutine,
	// ungrouped categories each get their own goroutine.
	type execGroup struct {
		entries []categoryEntry
	}
	groupMap := make(map[string]*execGroup)
	var groups []*execGroup
	for _, entry := range categories {
		groupName, isSeq := r.sequential[entry.name]
		if isSeq {
			g, exists := groupMap[groupName]
			if !exists {
				g = &execGroup{}
				groupMap[groupName] = g
				groups = append(groups, g)
			}
			g.entries = append(g.entries, entry)
		} else {
			groups = append(groups, &execGroup{entries: []categoryEntry{entry}})
		}
	}

	// Run groups concurrently; categories within a group run sequentially.
	type catResult struct {
		order int
		cat   CategoryResult
	}
	catResults := make(chan catResult, len(categories))

	var wg sync.WaitGroup
	for _, group := range groups {
		wg.Add(1)
		go func(entries []categoryEntry) {
			defer wg.Done()
			for _, entry := range entries {
				cr := CategoryResult{Name: entry.name}
				for _, check := range entry.checks {
					if ctx.Err() != nil {
						break
					}

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
					cr.Tests = append(cr.Tests, result)
				}
				catResults <- catResult{order: entry.order, cat: cr}
			}
		}(group.entries)
	}

	wg.Wait()
	close(catResults)

	// Reassemble categories in registration order.
	ordered := make([]CategoryResult, len(categories))
	for cr := range catResults {
		ordered[cr.order] = cr.cat
	}

	for _, cat := range ordered {
		run.Categories = append(run.Categories, cat)
		for _, t := range cat.Tests {
			switch t.Status {
			case StatusPass:
				run.Summary.Passed++
			case StatusFail:
				run.Summary.Failed++
			case StatusSkip:
				run.Summary.Skipped++
			}
			run.Summary.Total++
		}
	}

	run.Duration = time.Since(run.StartedAt)
	if run.Summary.Failed > 0 {
		run.Status = StatusFail
	} else {
		run.Status = StatusPass
	}

	return run
}
