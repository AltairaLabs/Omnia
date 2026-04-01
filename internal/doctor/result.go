package doctor

import "time"

type Status string

const (
	StatusRunning Status = "running"
	StatusPass    Status = "pass"
	StatusFail    Status = "fail"
	StatusSkip    Status = "skip"
)

type TestResult struct {
	Name     string        `json:"name"`
	Category string        `json:"category"`
	Status   Status        `json:"status"`
	Duration time.Duration `json:"duration"`
	Detail   string        `json:"detail"`
	Error    string        `json:"error,omitempty"`
}

type CategoryResult struct {
	Name  string       `json:"name"`
	Tests []TestResult `json:"tests"`
}

type RunResult struct {
	ID         string           `json:"id"`
	Status     Status           `json:"status"`
	StartedAt  time.Time        `json:"startedAt"`
	Duration   time.Duration    `json:"duration"`
	Summary    RunSummary       `json:"summary"`
	Categories []CategoryResult `json:"categories"`
}

type RunSummary struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}
