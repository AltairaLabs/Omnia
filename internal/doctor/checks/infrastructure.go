package checks

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/altairalabs/omnia/internal/doctor"
)

const healthTimeout = 5 * time.Second

// InfrastructureChecks returns liveness health checks for all provided services.
func InfrastructureChecks(services map[string]string) []doctor.Check {
	checks := make([]doctor.Check, 0, len(services))
	for name, url := range services {
		checks = append(checks, healthCheck(name, url))
	}
	return checks
}

// OllamaCheck returns a health check that queries the Ollama /api/tags endpoint.
func OllamaCheck(url string) doctor.Check {
	return probeCheck("Ollama", url, "/api/tags", "Healthy")
}

func healthCheck(name, baseURL string) doctor.Check {
	return probeCheck(name, baseURL, "/healthz", "Healthy")
}

func probeCheck(name, baseURL, path, suffix string) doctor.Check {
	return doctor.Check{
		Name:     name + suffix,
		Category: "Infrastructure",
		Run: func(ctx context.Context) doctor.TestResult {
			if baseURL == "" {
				return doctor.TestResult{
					Status: doctor.StatusSkip,
					Detail: "not configured",
				}
			}

			client := &http.Client{Timeout: healthTimeout}
			ctx, cancel := context.WithTimeout(ctx, healthTimeout)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
			if err != nil {
				return doctor.TestResult{Status: doctor.StatusFail, Detail: err.Error()}
			}

			resp, err := client.Do(req)
			if err != nil {
				return doctor.TestResult{Status: doctor.StatusFail, Detail: fmt.Sprintf("unreachable: %v", err)}
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode == http.StatusOK {
				return doctor.TestResult{Status: doctor.StatusPass, Detail: "responding"}
			}
			return doctor.TestResult{Status: doctor.StatusFail, Detail: fmt.Sprintf("HTTP %d", resp.StatusCode)}
		},
	}
}
