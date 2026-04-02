package checks

import (
	"context"
	"fmt"
	"net"
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

// OperatorAPICheck returns a check that verifies the operator REST API responds.
// It GETs /api/v1/workspaces and expects HTTP 200.
func OperatorAPICheck(baseURL string) doctor.Check {
	return probeCheck("OperatorAPI", baseURL, "/api/v1/workspaces", "Healthy")
}

// DashboardCheck returns a check that verifies the dashboard responds.
// It GETs / and expects HTTP 200.
func DashboardCheck(baseURL string) doctor.Check {
	return probeCheck("Dashboard", baseURL, "/", "Responds")
}

// ArenaControllerCheck returns a check that verifies the arena controller health endpoint.
// It GETs /healthz and expects HTTP 200. If the service is not deployed (unreachable),
// the check returns skip instead of fail.
func ArenaControllerCheck(baseURL string) doctor.Check {
	return doctor.Check{
		Name:     "ArenaControllerHealthy",
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

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/healthz", nil)
			if err != nil {
				return doctor.TestResult{Status: doctor.StatusFail, Detail: err.Error()}
			}

			resp, err := client.Do(req)
			if err != nil {
				// If the arena controller is not deployed, skip rather than fail.
				return doctor.TestResult{
					Status: doctor.StatusSkip,
					Detail: "arena controller not deployed",
				}
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode == http.StatusOK {
				return doctor.TestResult{Status: doctor.StatusPass, Detail: "responding"}
			}
			return doctor.TestResult{
				Status: doctor.StatusFail,
				Detail: fmt.Sprintf("HTTP %d", resp.StatusCode),
			}
		},
	}
}

// TCPCheck returns a check that verifies a TCP endpoint is reachable by dialing
// and immediately closing the connection.
func TCPCheck(name, addr string) doctor.Check {
	return doctor.Check{
		Name:     name + "Reachable",
		Category: "Infrastructure",
		Run: func(_ context.Context) doctor.TestResult {
			if addr == "" {
				return doctor.TestResult{
					Status: doctor.StatusSkip,
					Detail: "not configured",
				}
			}

			conn, err := net.DialTimeout("tcp", addr, healthTimeout)
			if err != nil {
				return doctor.TestResult{
					Status: doctor.StatusFail,
					Detail: fmt.Sprintf("unreachable: %v", err),
				}
			}
			_ = conn.Close()
			return doctor.TestResult{
				Status: doctor.StatusPass,
				Detail: "accepting connections",
			}
		},
	}
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
