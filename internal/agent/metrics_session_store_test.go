package agent

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

// TestSetSessionStoreMode verifies the omnia_agent_session_store gauge marks the
// active mode 1 and resets the others to 0, so an in-memory fallback is
// observable/alertable and a previous mode doesn't leave a stale "1" (#1223).
func TestSetSessionStoreMode(t *testing.T) {
	m := &Metrics{
		SessionStore: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "omnia_agent_session_store",
		}, []string{"mode"}),
	}

	m.SetSessionStoreMode(SessionStoreModeNone)
	assert.Equal(t, 1.0, testutil.ToFloat64(m.SessionStore.WithLabelValues(SessionStoreModeNone)))
	assert.Equal(t, 0.0, testutil.ToFloat64(m.SessionStore.WithLabelValues(SessionStoreModeHTTPClient)))

	// Flipping to httpclient must reset the none-mode gauge back to 0.
	m.SetSessionStoreMode(SessionStoreModeHTTPClient)
	assert.Equal(t, 1.0, testutil.ToFloat64(m.SessionStore.WithLabelValues(SessionStoreModeHTTPClient)))
	assert.Equal(t, 0.0, testutil.ToFloat64(m.SessionStore.WithLabelValues(SessionStoreModeNone)))
}
