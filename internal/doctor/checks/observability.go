package checks

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/altairalabs/omnia/internal/doctor"
)

const (
	metricsTimeout  = 5 * time.Second
	metricsCategory = "Observability"
	metricsPath     = "/metrics"
)

// ObservabilityChecks returns checks for Prometheus metrics endpoints.
// metricsURLs maps service name to its metrics URL (e.g., "SessionAPI" → "http://session-api:9090")
func ObservabilityChecks(metricsURLs map[string]string) []doctor.Check {
	checks := make([]doctor.Check, 0, len(metricsURLs))
	for name, url := range metricsURLs {
		checks = append(checks, metricsCheck(name, url))
	}
	return checks
}

func metricsCheck(name, url string) doctor.Check {
	return doctor.Check{
		Name:     name + " metrics",
		Category: metricsCategory,
		Run:      runMetricsCheck(name, url),
	}
}

func runMetricsCheck(name, url string) func(ctx context.Context) doctor.TestResult {
	return func(ctx context.Context) doctor.TestResult {
		if url == "" {
			return doctor.TestResult{Status: doctor.StatusSkip, Detail: "no metrics URL configured"}
		}

		prefix := serviceMetricsPrefix(name)
		body, err := fetchMetrics(ctx, url)
		if err != nil {
			return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error()}
		}

		count := countMetricLines(body, prefix)
		if count == 0 {
			return doctor.TestResult{
				Status: doctor.StatusFail,
				Error:  fmt.Sprintf("no metrics with prefix %q found", prefix),
			}
		}

		return doctor.TestResult{
			Status: doctor.StatusPass,
			Detail: fmt.Sprintf("%d metrics found", count),
		}
	}
}

// serviceMetricsPrefix converts a service name like "SessionAPI" into "omnia_session_api_".
// It handles both camelCase transitions (lower→upper) and acronym-to-word transitions (upper run → lower).
func serviceMetricsPrefix(name string) string {
	runes := []rune(name)
	var sb strings.Builder
	sb.WriteString("omnia_")
	for i, r := range runes {
		upper := unicode.IsUpper(r)
		if i > 0 && upper {
			prev := runes[i-1]
			next := rune(0)
			if i+1 < len(runes) {
				next = runes[i+1]
			}
			// Insert underscore when transitioning from lower→upper,
			// or at the start of a new word within an all-caps run (e.g., "APIFoo" → "api_foo").
			if unicode.IsLower(prev) || (unicode.IsUpper(prev) && next != 0 && unicode.IsLower(next)) {
				sb.WriteByte('_')
			}
		}
		sb.WriteRune(unicode.ToLower(r))
	}
	sb.WriteByte('_')
	return sb.String()
}

// fetchMetrics performs a GET request and returns the response body as a string.
func fetchMetrics(ctx context.Context, url string) (string, error) {
	client := &http.Client{Timeout: metricsTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+metricsPath, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch metrics: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	return string(data), nil
}

// countMetricLines counts lines in body that start with prefix and are not comments.
func countMetricLines(body, prefix string) int {
	count := 0
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, prefix) {
			count++
		}
	}
	return count
}
