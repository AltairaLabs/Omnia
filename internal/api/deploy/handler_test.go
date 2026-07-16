package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/altairalabs/omnia/internal/api/authz"
)

// testIdentityContext attaches a RequestIdentity to ctx, matching what the
// authz middleware would have stashed for a request scoped to namespace.
func testIdentityContext(ctx context.Context, namespace string) context.Context {
	id := &authz.RequestIdentity{
		VerifiedIdentity: &authz.VerifiedIdentity{Subject: "u@x.io"},
		Namespace:        namespace,
	}
	return authz.ContextWithIdentity(ctx, id)
}

func testHandler(t *testing.T, c client.Client) *Handler {
	t.Helper()
	return NewHandler(NewApplier(c, logr.Discard()), logr.Discard())
}

// TestHandler_MissingIdentity exercises the defensive 500 branch reached only
// if a request bypasses the authz middleware (it always sets an identity).
func TestHandler_MissingIdentity(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).Build()
	h := testHandler(t, c)

	r := httptest.NewRequest("POST", "/deployments", bytes.NewReader([]byte("{}")))
	w := httptest.NewRecorder()
	h.Deploy(w, r)

	if w.Code != 500 {
		t.Errorf("code = %d, want 500", w.Code)
	}
}

// TestHandler_InvalidJSON exercises the malformed-body 400 branch.
func TestHandler_InvalidJSON(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).Build()
	h := testHandler(t, c)

	ctx := testIdentityContext(context.Background(), "ns")
	r := httptest.NewRequest("POST", "/deployments", strings.NewReader("{not json")).WithContext(ctx)
	w := httptest.NewRecorder()
	h.Deploy(w, r)

	if w.Code != 400 {
		t.Errorf("code = %d, want 400", w.Code)
	}
}

// TestHandler_ApplyFailure exercises the 207 (multi-status) branch: the
// intent decodes and validates cleanly, but the underlying apply fails on one
// resource.
func TestHandler_ApplyFailure(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithInterceptorFuncs(interceptor.Funcs{
		Create: func(ctx context.Context, cli client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*corev1.ConfigMap); ok {
				return errBoom
			}
			return cli.Create(ctx, obj, opts...)
		},
	}).Build()
	h := testHandler(t, c)

	body, _ := json.Marshal(testIntent())
	ctx := testIdentityContext(context.Background(), "ns")
	r := httptest.NewRequest("POST", "/deployments", bytes.NewReader(body)).WithContext(ctx)
	w := httptest.NewRecorder()
	h.Deploy(w, r)

	if w.Code != 207 {
		t.Fatalf("code = %d, want 207", w.Code)
	}
	var out DeployResult
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Succeeded {
		t.Errorf("result.Succeeded = true, want false: %+v", out)
	}
}
