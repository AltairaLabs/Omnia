package checks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	eev1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/doctor"
)

// mockEnterpriseLicenseServer returns a test server that serves an enterprise license
// at /api/v1/license, mimicking the arena controller.
func mockEnterpriseLicenseServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tier":"enterprise"}`))
	}))
}

// newPrivacyCheckerForServer creates a PrivacyChecker pointing at the given test server
// with a mock enterprise license server.
func newPrivacyCheckerForServer(srv *httptest.Server, arenaSrv *httptest.Server) *PrivacyChecker {
	return NewPrivacyChecker(srv.URL, "", testWorkspace, arenaSrv.URL)
}

// privacySaveBody captures a decoded save request body for assertions.
type privacySaveBody struct {
	Type    string                 `json:"type"`
	Content string                 `json:"content"`
	Scope   map[string]interface{} `json:"scope"`
}

// mockPrivacyServer builds a minimal httptest.Server for privacy check tests.
// handlers map path strings to handler funcs; the save endpoint dispatches on method.
type mockPrivacyServer struct {
	saveHandler        http.HandlerFunc
	searchHandler      http.HandlerFunc
	batchDeleteHandler http.HandlerFunc
	auditHandler       http.HandlerFunc
}

func (m *mockPrivacyServer) serve(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	saveH := m.saveHandler
	if saveH == nil {
		saveH = func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"memory":{"id":"priv-test-id"}}`))
		}
	}

	searchH := m.searchHandler
	if searchH == nil {
		searchH = func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"memories":[],"total":0}`))
		}
	}

	mux.HandleFunc(privacyMemoriesPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			saveH(w, r)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc(privacyMemorySearchPath, searchH)

	if m.batchDeleteHandler != nil {
		mux.HandleFunc(privacyBatchDeletePath, m.batchDeleteHandler)
	}

	if m.auditHandler != nil {
		mux.HandleFunc("/api/v1/audit/memories", m.auditHandler)
	}

	return httptest.NewServer(mux)
}

// --- MemoryPIIRedaction ---

func TestCheckPIIRedaction_Pass(t *testing.T) {
	arenaSrv := mockEnterpriseLicenseServer(t)
	defer arenaSrv.Close()

	// Save succeeds; search returns content without SSN.
	srv := (&mockPrivacyServer{
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"memories":[{"id":"m1","content":"patient ssn is [REDACTED]"}],"total":1}`))
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv, arenaSrv).checkPIIRedaction(t.Context())
	assert.Equal(t, doctor.StatusPass, result.Status)
	assert.Contains(t, result.Detail, "redacted")
}

func TestCheckPIIRedaction_Fail_SSNPresent(t *testing.T) {
	arenaSrv := mockEnterpriseLicenseServer(t)
	defer arenaSrv.Close()

	// Search returns content still containing the SSN.
	srv := (&mockPrivacyServer{
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			body, _ := json.Marshal(map[string]interface{}{
				"memories": []map[string]interface{}{
					{"id": "m1", "content": "patient ssn is " + privacyTestSSN},
				},
				"total": 1,
			})
			_, _ = w.Write(body)
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv, arenaSrv).checkPIIRedaction(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "unredacted")
}

func TestCheckPIIRedaction_Fail_SaveError(t *testing.T) {
	arenaSrv := mockEnterpriseLicenseServer(t)
	defer arenaSrv.Close()

	srv := (&mockPrivacyServer{
		saveHandler: func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "internal error", http.StatusInternalServerError)
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv, arenaSrv).checkPIIRedaction(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

func TestCheckPIIRedaction_Skip_NoWorkspace(t *testing.T) {
	c := NewPrivacyChecker("http://localhost:8080", "", "", "")
	result := c.checkPIIRedaction(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
	assert.Contains(t, result.Detail, "workspace UID not resolved")
}

func TestCheckPIIRedaction_Skip_NoEnterprise(t *testing.T) {
	c := NewPrivacyChecker("http://localhost:8080", "", testWorkspace, "")
	result := c.checkPIIRedaction(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
	assert.Contains(t, result.Detail, "enterprise not configured")
}

func TestCheckPIIRedaction_Skip_SearchUnavailable(t *testing.T) {
	arenaSrv := mockEnterpriseLicenseServer(t)
	defer arenaSrv.Close()

	// Save succeeds but search returns 500 — should skip (search unavailable).
	srv := (&mockPrivacyServer{
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "search down", http.StatusInternalServerError)
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv, arenaSrv).checkPIIRedaction(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
	assert.Contains(t, result.Detail, "unavailable")
}

func TestCheckPIIRedaction_SendsSSNInContent(t *testing.T) {
	arenaSrv := mockEnterpriseLicenseServer(t)
	defer arenaSrv.Close()

	var captured privacySaveBody
	srv := (&mockPrivacyServer{
		saveHandler: func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"memory":{"id":"x"}}`))
		},
	}).serve(t)
	defer srv.Close()

	_ = newPrivacyCheckerForServer(srv, arenaSrv).checkPIIRedaction(t.Context())
	assert.Contains(t, captured.Content, privacyTestSSN)
}

// --- MemoryOptOutRespected ---

func TestCheckOptOutRespected_Pass(t *testing.T) {
	arenaSrv := mockEnterpriseLicenseServer(t)
	defer arenaSrv.Close()

	// Mock session-api accepts opt-out, mock memory-api rejects save with 204.
	sessionSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer sessionSrv.Close()

	memorySrv := (&mockPrivacyServer{
		saveHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	}).serve(t)
	defer memorySrv.Close()

	c := NewPrivacyChecker(memorySrv.URL, sessionSrv.URL, testWorkspace, arenaSrv.URL)
	result := c.checkOptOutRespected(t.Context())
	assert.Equal(t, doctor.StatusPass, result.Status)
	assert.Contains(t, result.Detail, "204")
}

func TestCheckOptOutRespected_Fail_SavedDespiteOptOut(t *testing.T) {
	arenaSrv := mockEnterpriseLicenseServer(t)
	defer arenaSrv.Close()

	// Session-api accepts opt-out, but memory-api saves anyway (middleware broken).
	sessionSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer sessionSrv.Close()

	memorySrv := (&mockPrivacyServer{}).serve(t)
	defer memorySrv.Close()

	c := NewPrivacyChecker(memorySrv.URL, sessionSrv.URL, testWorkspace, arenaSrv.URL)
	result := c.checkOptOutRespected(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "despite")
}

func TestCheckOptOutRespected_Skip_SessionAPIUnreachable(t *testing.T) {
	arenaSrv := mockEnterpriseLicenseServer(t)
	defer arenaSrv.Close()

	memorySrv := (&mockPrivacyServer{}).serve(t)
	defer memorySrv.Close()

	c := NewPrivacyChecker(memorySrv.URL, "https://127.0.0.1:1", testWorkspace, arenaSrv.URL)
	result := c.checkOptOutRespected(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
}

func TestCheckOptOutRespected_Skip_NoWorkspace(t *testing.T) {
	c := NewPrivacyChecker("https://localhost:8080", "", "", "")
	result := c.checkOptOutRespected(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
}

// --- MemoryDeletionCascade ---

func TestCheckDeletionCascade_Pass(t *testing.T) {
	arenaSrv := mockEnterpriseLicenseServer(t)
	defer arenaSrv.Close()

	srv := (&mockPrivacyServer{
		batchDeleteHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"deleted":1}`))
		},
		// search after delete returns empty.
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"memories":[],"total":0}`))
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv, arenaSrv).checkDeletionCascade(t.Context())
	assert.Equal(t, doctor.StatusPass, result.Status)
	assert.Contains(t, result.Detail, "absent")
}

func TestCheckDeletionCascade_Skip_BatchEndpointNotFound(t *testing.T) {
	arenaSrv := mockEnterpriseLicenseServer(t)
	defer arenaSrv.Close()

	srv := (&mockPrivacyServer{
		batchDeleteHandler: func(w http.ResponseWriter, _ *http.Request) {
			http.NotFound(w, nil)
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv, arenaSrv).checkDeletionCascade(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
	assert.Contains(t, result.Detail, "batch delete endpoint not available")
}

func TestCheckDeletionCascade_Fail_MemoryStillPresent(t *testing.T) {
	arenaSrv := mockEnterpriseLicenseServer(t)
	defer arenaSrv.Close()

	srv := (&mockPrivacyServer{
		batchDeleteHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"deleted":0}`))
		},
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"memories":[{"id":"m1","content":"deletion cascade test"}],"total":1}`))
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv, arenaSrv).checkDeletionCascade(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "still present")
}

func TestCheckDeletionCascade_Fail_SaveFails(t *testing.T) {
	arenaSrv := mockEnterpriseLicenseServer(t)
	defer arenaSrv.Close()

	srv := (&mockPrivacyServer{
		saveHandler: func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "save error", http.StatusInternalServerError)
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv, arenaSrv).checkDeletionCascade(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "save failed")
}

func TestCheckDeletionCascade_Fail_BatchDeleteError(t *testing.T) {
	arenaSrv := mockEnterpriseLicenseServer(t)
	defer arenaSrv.Close()

	srv := (&mockPrivacyServer{
		batchDeleteHandler: func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "delete error", http.StatusInternalServerError)
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv, arenaSrv).checkDeletionCascade(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

func TestCheckDeletionCascade_Skip_NoWorkspace(t *testing.T) {
	c := NewPrivacyChecker("http://localhost:8080", "", "", "")
	result := c.checkDeletionCascade(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
}

// --- AuditLogWritten ---

func TestCheckAuditLogWritten_Pass(t *testing.T) {
	arenaSrv := mockEnterpriseLicenseServer(t)
	defer arenaSrv.Close()

	srv := (&mockPrivacyServer{
		auditHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			// Return entry with memory_id matching the test save response ("priv-test-id").
			_, _ = w.Write([]byte(`{"entries":[{"eventType":"memory_created","memory_id":"priv-test-id"}],"total":1,"hasMore":false}`))
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv, arenaSrv).checkAuditLogWritten(t.Context())
	assert.Equal(t, doctor.StatusPass, result.Status)
	assert.Contains(t, result.Detail, "priv-test-id")
}

func TestCheckAuditLogWritten_Fail_NoEvents(t *testing.T) {
	arenaSrv := mockEnterpriseLicenseServer(t)
	defer arenaSrv.Close()

	srv := (&mockPrivacyServer{
		auditHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"entries":[],"total":0,"hasMore":false}`))
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv, arenaSrv).checkAuditLogWritten(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "no audit event found")
}

func TestCheckAuditLogWritten_Skip_EndpointNotAvailable(t *testing.T) {
	arenaSrv := mockEnterpriseLicenseServer(t)
	defer arenaSrv.Close()

	// No audit handler registered → 404 → skip.
	srv := (&mockPrivacyServer{}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv, arenaSrv).checkAuditLogWritten(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
	assert.Contains(t, result.Detail, "audit endpoint not available")
}

func TestCheckAuditLogWritten_Skip_NoWorkspace(t *testing.T) {
	c := NewPrivacyChecker("http://localhost:8080", "", "", "")
	result := c.checkAuditLogWritten(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
}

// --- Checks() registration ---

func TestPrivacyChecker_Checks_ReturnsFive(t *testing.T) {
	c := NewPrivacyChecker("http://localhost:8080", "", "ws1", "")
	cs := c.Checks()
	require.Len(t, cs, 5)
	names := make([]string, len(cs))
	for i, ch := range cs {
		names[i] = ch.Name
	}
	assert.Equal(t, []string{
		"MemoryPIIRedaction",
		"MemoryOptOutRespected",
		"MemoryDeletionCascade",
		"AuditLogWritten",
		"SessionEncryptionAtRest",
	}, names)
	for _, ch := range cs {
		assert.Equal(t, privacyCategory, ch.Category)
	}
}

// --- SessionEncryptionAtRest ---

// newPrivacyEncryptionScheme creates a runtime.Scheme with both core Omnia and EE types.
func newPrivacyEncryptionScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(s))
	require.NoError(t, eev1alpha1.AddToScheme(s))
	return s
}

// newPrivacyCheckerWithK8s builds a PrivacyChecker backed by a fake k8s client
// pre-populated with the given objects.
func newPrivacyCheckerWithK8s(t *testing.T, objs ...runtime.Object) *PrivacyChecker {
	t.Helper()
	s := newPrivacyEncryptionScheme(t)
	builder := fake.NewClientBuilder().WithScheme(s)
	if len(objs) > 0 {
		builder = builder.WithRuntimeObjects(objs...)
	}
	return NewPrivacyChecker("", "", "", "").WithK8sClient(builder.Build())
}

// TestCheckSessionEncryption_Skip_NoK8sClient verifies skip when no k8s client is set.
func TestCheckSessionEncryption_Skip_NoK8sClient(t *testing.T) {
	c := NewPrivacyChecker("", "", "", "")
	result := c.checkSessionEncryption(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
	assert.Contains(t, result.Detail, "k8s client not available")
}

// TestCheckSessionEncryption_Skip_NoWorkspaces verifies skip when no Workspace CRDs exist.
func TestCheckSessionEncryption_Skip_NoWorkspaces(t *testing.T) {
	c := newPrivacyCheckerWithK8s(t)
	result := c.checkSessionEncryption(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
	assert.Contains(t, result.Detail, "no Workspaces found")
}

// TestCheckSessionEncryption_Skip_NoPrivacyPolicyRef verifies skip (info) when a workspace
// has a service group but no privacyPolicyRef — sessions are in plaintext, which is valid.
func TestCheckSessionEncryption_Skip_NoPrivacyPolicyRef(t *testing.T) {
	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: "WS 1",
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "ws-ns-1"},
			Services: []omniav1alpha1.WorkspaceServiceGroup{
				{Name: "default", Mode: omniav1alpha1.ServiceModeManaged},
			},
		},
	}
	c := newPrivacyCheckerWithK8s(t, ws)
	result := c.checkSessionEncryption(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
	assert.Contains(t, result.Detail, "plaintext")
}

// TestCheckSessionEncryption_Skip_EncryptionDisabledInPolicy verifies skip (info) when a
// service group references a policy but that policy has encryption.enabled=false.
func TestCheckSessionEncryption_Skip_EncryptionDisabledInPolicy(t *testing.T) {
	policy := &eev1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-encrypt-policy",
			Namespace: "ws-ns-1",
		},
		Spec: eev1alpha1.SessionPrivacyPolicySpec{
			Recording:  eev1alpha1.RecordingConfig{Enabled: true},
			Encryption: &eev1alpha1.EncryptionConfig{Enabled: false},
		},
	}
	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: "WS 1",
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "ws-ns-1"},
			Services: []omniav1alpha1.WorkspaceServiceGroup{
				{
					Name: "default",
					Mode: omniav1alpha1.ServiceModeManaged,
					PrivacyPolicyRef: &corev1.LocalObjectReference{
						Name: "no-encrypt-policy",
					},
				},
			},
		},
	}
	c := newPrivacyCheckerWithK8s(t, policy, ws)
	result := c.checkSessionEncryption(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
	assert.Contains(t, result.Detail, "plaintext")
}

// TestCheckSessionEncryption_Pass_EncryptionEnabled verifies pass when a service group
// references a policy with encryption enabled, reporting workspace/group/kmsProvider/keyID.
func TestCheckSessionEncryption_Pass_EncryptionEnabled(t *testing.T) {
	policy := &eev1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "encrypt-policy",
			Namespace: "ws-ns-1",
		},
		Spec: eev1alpha1.SessionPrivacyPolicySpec{
			Recording: eev1alpha1.RecordingConfig{Enabled: true},
			Encryption: &eev1alpha1.EncryptionConfig{
				Enabled:     true,
				KMSProvider: eev1alpha1.KMSProviderAWSKMS,
				KeyID:       "arn:aws:kms:us-east-1:123456789:key/abc-def",
			},
		},
	}
	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-prod"},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: "Production",
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "ws-ns-1"},
			Services: []omniav1alpha1.WorkspaceServiceGroup{
				{
					Name: "payments",
					Mode: omniav1alpha1.ServiceModeManaged,
					PrivacyPolicyRef: &corev1.LocalObjectReference{
						Name: "encrypt-policy",
					},
				},
			},
		},
	}
	c := newPrivacyCheckerWithK8s(t, policy, ws)
	result := c.checkSessionEncryption(t.Context())
	assert.Equal(t, doctor.StatusPass, result.Status)
	assert.Contains(t, result.Detail, "ws-prod")
	assert.Contains(t, result.Detail, "payments")
	assert.Contains(t, result.Detail, "aws-kms")
}

// TestCheckSessionEncryption_Fail_MissingPolicy verifies error when a privacyPolicyRef
// names a policy that does not exist in the cluster.
func TestCheckSessionEncryption_Fail_MissingPolicy(t *testing.T) {
	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: "WS 1",
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "ws-ns-1"},
			Services: []omniav1alpha1.WorkspaceServiceGroup{
				{
					Name: "default",
					Mode: omniav1alpha1.ServiceModeManaged,
					PrivacyPolicyRef: &corev1.LocalObjectReference{
						Name: "ghost-policy",
					},
				},
			},
		},
	}
	c := newPrivacyCheckerWithK8s(t, ws)
	result := c.checkSessionEncryption(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "ghost-policy")
	assert.Contains(t, result.Detail, "not found")
}
