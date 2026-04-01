package checks

import (
	"testing"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResolveWorkspaceUID_Found(t *testing.T) {
	expectedUID := types.UID("abc-123-def")
	objs := []runtime.Object{
		&omniav1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "demo",
				UID:  expectedUID,
			},
			Spec: omniav1alpha1.WorkspaceSpec{
				DisplayName: "Demo",
				Namespace:   omniav1alpha1.NamespaceConfig{Name: "omnia-demo"},
			},
		},
		&omniav1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "other",
				UID:  "other-uid",
			},
			Spec: omniav1alpha1.WorkspaceSpec{
				DisplayName: "Other",
				Namespace:   omniav1alpha1.NamespaceConfig{Name: "omnia-other"},
			},
		},
	}

	s := newTestScheme(t)
	k8s := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objs...).Build()

	uid := ResolveWorkspaceUID(k8s, "omnia-demo", logr.Discard())
	if uid != string(expectedUID) {
		t.Errorf("expected UID %q, got %q", expectedUID, uid)
	}
}

func TestResolveWorkspaceUID_NotFound(t *testing.T) {
	s := newTestScheme(t)
	k8s := fake.NewClientBuilder().WithScheme(s).Build()

	uid := ResolveWorkspaceUID(k8s, "nonexistent", logr.Discard())
	if uid != "" {
		t.Errorf("expected empty UID, got %q", uid)
	}
}

func TestResolveWorkspaceUID_NoWorkspaces(t *testing.T) {
	s := newTestScheme(t)
	k8s := fake.NewClientBuilder().WithScheme(s).Build()

	uid := ResolveWorkspaceUID(k8s, "omnia-demo", logr.Discard())
	if uid != "" {
		t.Errorf("expected empty UID, got %q", uid)
	}
}
