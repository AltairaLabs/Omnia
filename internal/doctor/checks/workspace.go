package checks

import (
	"context"
	"time"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const workspaceLookupTimeout = 5 * time.Second

// ResolveWorkspaceUID finds the Workspace CRD whose spec.namespace.name matches
// the given namespace and returns its Kubernetes UID (as a string).
// Memory-api stores workspace_id as UUID, which maps to the Workspace CR's UID.
func ResolveWorkspaceUID(k8s client.Client, namespace string, log logr.Logger) string {
	ctx, cancel := context.WithTimeout(context.Background(), workspaceLookupTimeout)
	defer cancel()

	var list omniav1alpha1.WorkspaceList
	if err := k8s.List(ctx, &list); err != nil {
		log.V(1).Info("workspace lookup failed", "error", err.Error())
		return ""
	}

	for _, ws := range list.Items {
		if ws.Spec.Namespace.Name == namespace {
			uid := string(ws.UID)
			log.V(1).Info("resolved workspace UID", "namespace", namespace, "uid", uid)
			return uid
		}
	}

	log.V(1).Info("no workspace found for namespace", "namespace", namespace)
	return ""
}
