//go:build e2e

/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"os/exec"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/api/authz"
	"github.com/altairalabs/omnia/internal/api/deploy"
	"github.com/altairalabs/omnia/pkg/facade/auth"
	"github.com/altairalabs/omnia/test/utils"
)

// Deploy-intent E2E identifiers. The workspace grants the test identity the
// editor role via a directGrant so the deploy API's authz middleware
// (>= editor for POST) admits the minted token.
const (
	deployWorkspace   = "deploy-e2e-ws"
	deployWorkspaceNS = "deploy-e2e-agents"
	deployIdentity    = "deploy-e2e@omnia.test"
	deployPackName    = "deploy-e2e-pack"
	deployPackVersion = "1.0.0"
	deployAgentName   = "deploy-e2e-agent"

	deployCurlPod    = "deploy-e2e-curl"
	deployJWKSObject = "deploy-e2e-jwks"
	deployAPIService = "deploy-api-e2e"

	// Trigger-mode fixture (version-bump canary spec). Isolated from the
	// materialize spec's pack/agent so the two deploys don't collide.
	deployTriggerPackName  = "deploy-e2e-trigger-pack"
	deployTriggerAgentName = "deploy-e2e-trigger-agent"
	deployTriggerProvider  = "deploy-e2e-trigger-provider"
	deployTriggerChannel   = "stable"
	deployTriggerVersionV1 = "1.0.0"
	deployTriggerVersionV2 = "1.1.0"

	// deployJWKSURL is served by the in-cluster nginx pod (public half of the
	// test RSA key). The manager verifies identity tokens against it.
	deployJWKSURL = "http://deploy-e2e-jwks.omnia-system.svc.cluster.local/keys"
)

// deployRunKid is a per-run-unique JWKS key id. Using a fresh kid each run
// forces the manager's JWKS resolver (which caches keys by kid indefinitely)
// to refetch the current public key rather than validate against a stale one
// left cached by a prior E2E_SKIP_CLEANUP run.
var deployRunKid string

// deployAPIURL is the in-cluster deploy API endpoint for the test workspace,
// reachable from the curl helper pod via the deploy-api-e2e Service.
func deployAPIURL() string {
	return fmt.Sprintf(
		"http://%s.%s.svc.cluster.local:8085/api/v1/workspaces/%s/deployments",
		deployAPIService, namespace, deployWorkspace)
}

// generateDeployKey returns a fresh 2048-bit RSA key used to sign identity
// tokens and (public half) to serve via the in-cluster JWKS endpoint.
func generateDeployKey() *rsa.PrivateKey {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())
	return key
}

// jwksJSON builds a single-key JWKS document for pub under kid, matching the
// operator's JWKS resolver on-the-wire shape (RSA n/e as base64url).
func jwksJSON(pub *rsa.PublicKey, kid string) string {
	doc := map[string]any{
		"keys": []map[string]string{{
			"kty": "RSA",
			"use": "sig",
			"alg": "RS256",
			"kid": kid,
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}},
	}
	b, err := json.Marshal(doc)
	Expect(err).NotTo(HaveOccurred())
	return string(b)
}

// mintDeployToken signs an RS256 identity JWT scoped to workspace for the test
// identity (deployIdentity), mirroring the dashboard's content-API token shape
// (authz.IdentityClaims).
func mintDeployToken(key *rsa.PrivateKey, kid, workspace string) string {
	now := time.Now()
	claims := authz.IdentityClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    authz.IssuerDashboard,
			Audience:  jwt.ClaimStrings{authz.AudienceContentAPI},
			Subject:   deployIdentity,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Second)),
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		},
		Identity:  deployIdentity,
		Workspace: workspace,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	Expect(err).NotTo(HaveOccurred())
	return signed
}

// deployIntentBody returns the JSON body for a minimal valid DeployIntent: one
// pack (opaque content) and one pinned-mode agent, no external provider or tool
// dependency — enough to exercise the create path end-to-end.
func deployIntentBody() string {
	intent := deploy.DeployIntent{
		APIVersion: deploy.APIVersionV1,
		Pack: deploy.PackIntent{
			Name:    deployPackName,
			Version: deployPackVersion,
			Content: `{"id":"deploy-e2e-pack","version":"1.0.0",` +
				`"prompts":{"default":{"id":"default","name":"Default",` +
				`"version":"1.0.0","system_template":"You are a helpful assistant."}}}`,
		},
		Agents: []deploy.AgentIntent{{
			Name:      deployAgentName,
			Providers: []deploy.ProviderBind{{Name: "primary", Ref: "deploy-e2e-provider"}},
		}},
	}
	b, err := json.Marshal(intent)
	Expect(err).NotTo(HaveOccurred())
	return string(b)
}

// deployApplyStdin applies a YAML manifest passed on stdin.
func deployApplyStdin(manifest string) error {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	_, err := utils.Run(cmd)
	return err
}

// deployRecreateJWKS re-applies the JWKS ConfigMap + Service and recreates the
// nginx pod so it remounts the current (fresh-key) ConfigMap. Deleting the pod
// first is what forces the new content to be served — a plain re-apply is a
// no-op for an unchanged pod spec and would keep serving the stale volume.
func deployRecreateJWKS(jwks string) {
	_, _ = utils.Run(exec.Command("kubectl", "delete", "pod", deployJWKSObject,
		"-n", namespace, "--ignore-not-found", "--timeout=60s"))
	Expect(deployApplyStdin(deployJWKSManifest(jwks))).To(Succeed())
	deployWaitPodRunning(deployJWKSObject)
}

// deployWaitPodRunning blocks until the named pod in omnia-system is Running.
func deployWaitPodRunning(pod string) {
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "pod", pod, "-n", namespace,
			"-o", "jsonpath={.status.phase}")
		out, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(out).To(Equal("Running"))
	}, 2*time.Minute, 3*time.Second).Should(Succeed())
}

// deployObjectExists asserts (via Eventually) that a namespaced object exists.
func deployObjectExists(kind, name string) {
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", kind, name, "-n", deployWorkspaceNS)
		_, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
	}, 2*time.Minute, 3*time.Second).Should(Succeed())
}

// curlDeploy runs curl from the helper pod and returns "<body>\n<http_code>".
func curlDeploy(extraArgs ...string) (string, error) {
	base := make([]string, 0, 9+len(extraArgs))
	base = append(base, "exec", deployCurlPod, "-n", namespace, "--",
		"curl", "-s", "-w", "\n%{http_code}")
	cmd := exec.Command("kubectl", append(base, extraArgs...)...)
	return utils.Run(cmd)
}

// splitCurlResponse splits curl's "<body>\n<http_code>" output; the last line
// is the status code and everything before it is the response body.
func splitCurlResponse(out string) (body, status string) {
	trimmed := strings.TrimRight(out, "\n")
	i := strings.LastIndex(trimmed, "\n")
	if i < 0 {
		return "", trimmed
	}
	return trimmed[:i], trimmed[i+1:]
}

// deployJWKSManifest builds the ConfigMap + nginx pod + Service that serve the
// public JWKS document. nginx-unprivileged listens on 8080 as a non-root user
// so it satisfies the restricted PodSecurity policy on omnia-system.
func deployJWKSManifest(jwks string) string {
	return fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: %[1]s
  namespace: %[2]s
data:
  keys: '%[3]s'
---
apiVersion: v1
kind: Pod
metadata:
  name: %[1]s
  namespace: %[2]s
  labels:
    app: %[1]s
spec:
  securityContext:
    runAsNonRoot: true
    seccompProfile:
      type: RuntimeDefault
  containers:
  - name: nginx
    image: nginxinc/nginx-unprivileged:alpine
    ports:
    - containerPort: 8080
    volumeMounts:
    - name: jwks
      mountPath: /usr/share/nginx/html
    securityContext:
      runAsUser: 101
      allowPrivilegeEscalation: false
      capabilities:
        drop: ["ALL"]
  volumes:
  - name: jwks
    configMap:
      name: %[1]s
---
apiVersion: v1
kind: Service
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  selector:
    app: %[1]s
  ports:
  - port: 80
    targetPort: 8080
`, deployJWKSObject, namespace, jwks)
}

// deployAPIServiceManifest builds the ClusterIP Service that fronts the
// manager's deploy API on :8085 (kustomize exposes no such Service).
func deployAPIServiceManifest() string {
	return fmt.Sprintf(`
apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: %s
spec:
  selector:
    control-plane: controller-manager
    app.kubernetes.io/name: omnia
  ports:
  - port: 8085
    targetPort: 8085
`, deployAPIService, namespace)
}

// deployWorkspaceManifest builds the workspace namespace + a Workspace CR
// granting deployIdentity the editor role via a directGrant.
func deployWorkspaceManifest() string {
	return fmt.Sprintf(`
apiVersion: v1
kind: Namespace
metadata:
  name: %[2]s
  labels:
    pod-security.kubernetes.io/enforce: restricted
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Workspace
metadata:
  name: %[1]s
spec:
  displayName: "Deploy E2E Workspace"
  namespace:
    name: %[2]s
  directGrants:
  - user: %[3]s
    role: editor
`, deployWorkspace, deployWorkspaceNS, deployIdentity)
}

// deployCurlPodManifest builds the curl helper pod with a full restricted
// securityContext (omnia-system enforces restricted PodSecurity).
func deployCurlPodManifest() string {
	return fmt.Sprintf(`
apiVersion: v1
kind: Pod
metadata:
  name: %s
  namespace: %s
spec:
  restartPolicy: Never
  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
    seccompProfile:
      type: RuntimeDefault
  containers:
  - name: curl
    image: curlimages/curl:8.5.0
    command: ["sleep", "3600"]
    securityContext:
      allowPrivilegeEscalation: false
      capabilities:
        drop: ["ALL"]
`, deployCurlPod, namespace)
}

// managerArgsPatch builds a strategic-merge patch that sets the manager
// container args to the suite's canonical base set plus any extra flags. A
// full-args merge (not a JSON append) keeps it idempotent across
// E2E_SKIP_CLEANUP re-runs — a duplicated flag would crash the manager at
// startup. The base set mirrors ensureManagerDeployed (e2e_test.go) so a
// restore returns the shared manager to exactly the args a sibling suite
// expects.
func managerArgsPatch(extra ...string) string {
	args := make([]string, 0, 8+len(extra))
	args = append(args,
		"--metrics-bind-address=:8443", "--leader-elect",
		"--health-probe-bind-address=:8081",
		"--facade-image="+facadeImageRef,
		"--framework-image="+runtimeImageRef,
		"--session-api-image="+sessionApiImage,
		"--memory-api-image="+memoryApiImage,
		"--workspace-reader-rbac=true",
	)
	args = append(args, extra...)
	patch := map[string]any{"spec": map[string]any{"template": map[string]any{
		"spec": map[string]any{"containers": []map[string]any{
			{"name": "manager", "args": args},
		}},
	}}}
	b, err := json.Marshal(patch)
	Expect(err).NotTo(HaveOccurred())
	return string(b)
}

// applyManagerArgsPatch strategic-merges patch onto the manager Deployment and
// waits for the resulting rollout to complete.
func applyManagerArgsPatch(patch string) error {
	cmd := exec.Command("kubectl", "patch", "deployment", "omnia-controller-manager",
		"-n", namespace, "--type=strategic", "-p", patch)
	if _, err := utils.Run(cmd); err != nil {
		return err
	}
	cmd = exec.Command("kubectl", "rollout", "status",
		"deployment/omnia-controller-manager", "-n", namespace, "--timeout=120s")
	_, err := utils.Run(cmd)
	return err
}

// patchManagerDeployArgs enables the deploy API on the manager (bind address +
// JWKS URL) on top of the canonical base args.
func patchManagerDeployArgs() error {
	return applyManagerArgsPatch(managerArgsPatch(
		"--deploy-api-bind-address=:8085",
		"--mgmt-plane-jwks-url="+deployJWKSURL))
}

// restoreManagerBaseArgs reverts the manager to the canonical base args (no
// deploy API flags), undoing patchManagerDeployArgs so a non-skipCleanup run
// leaves the shared manager unmutated for sibling suites.
func restoreManagerBaseArgs() error {
	return applyManagerArgsPatch(managerArgsPatch())
}

// deployContentConfigMapName mirrors deploy.contentConfigMapName (unexported):
// the pack's object name suffixed with "-content".
func deployContentConfigMapName() string {
	return omniav1alpha1.PromptPackObjectName(deployPackName, deployPackVersion) + "-content"
}

// i32 returns a pointer to v (for the *int32 setWeight fields).
func i32(v int32) *int32 { return &v }

// deployTriggerPackContent builds a schema-valid pack.json for the given
// version. The shape mirrors the known-good rollout E2E fixture (includes the
// template_engine block) so the PromptPack reaches the Active phase and the
// agent's stable pin comes up — a prerequisite for the version trigger.
func deployTriggerPackContent(version string) string {
	return fmt.Sprintf(`{"id":%[1]q,"name":%[1]q,"version":%[2]q,`+
		`"template_engine":{"version":"v1","syntax":"{{variable}}"},`+
		`"prompts":{"default":{"id":"default","name":"default","version":%[2]q,`+
		`"system_template":"You are version %[2]s."}}}`, deployTriggerPackName, version)
}

// deployTriggerIntentBody returns a DeployIntent for a single version-triggered
// (spec.rollout.trigger) agent at the given pack version. The rollout steps
// hold a canary at 20% behind an indefinite-enough pause so the candidate ref
// stays observable rather than promoting straight through.
func deployTriggerIntentBody(version string) string {
	intent := deploy.DeployIntent{
		APIVersion: deploy.APIVersionV1,
		Pack: deploy.PackIntent{
			Name:    deployTriggerPackName,
			Version: version,
			Content: deployTriggerPackContent(version),
		},
		Agents: []deploy.AgentIntent{{
			Name:      deployTriggerAgentName,
			Providers: []deploy.ProviderBind{{Name: "default", Ref: deployTriggerProvider}},
			Rollout: &deploy.RolloutIntent{
				Trigger: &deploy.RolloutTriggerIntent{PromptPackChannel: deployTriggerChannel},
				Steps: []deploy.RolloutStepIntent{
					{SetWeight: i32(20)},
					{PauseDuration: "10m"},
					{SetWeight: i32(100)},
				},
			},
		}},
	}
	b, err := json.Marshal(intent)
	Expect(err).NotTo(HaveOccurred())
	return string(b)
}

// deployTriggerProviderManifest builds a dummy secret + mock Provider the
// trigger-mode agent binds to. Mock needs no real key; the secret mirrors the
// rollout E2E fixture so the Provider reconciles to Ready and the agent's
// references resolve.
func deployTriggerProviderManifest() string {
	return fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %[1]s
  namespace: %[2]s
type: Opaque
stringData:
  api-key: mock-key
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  type: mock
  credential:
    secretRef:
      name: %[1]s
`, deployTriggerProvider, deployWorkspaceNS)
}

// postDeployIntent POSTs body with an editor token and asserts a 200 + a
// succeeded DeployResult. Retried since the manager's deploy API may still be
// coming up on the first call of a spec.
func postDeployIntent(token, body string) {
	var respBody, status string
	Eventually(func(g Gomega) {
		out, err := curlDeploy("-X", "POST",
			"-H", "Authorization: Bearer "+token,
			"-H", "Content-Type: application/json",
			"-d", body, deployAPIURL())
		g.Expect(err).NotTo(HaveOccurred())
		respBody, status = splitCurlResponse(out)
		g.Expect(status).To(Equal("200"), "body: "+respBody)
	}, 2*time.Minute, 3*time.Second).Should(Succeed())

	var result deploy.DeployResult
	Expect(json.Unmarshal([]byte(respBody), &result)).To(Succeed(), "body: "+respBody)
	Expect(result.Succeeded).To(BeTrue(), "results: "+respBody)
}

// deployTriggerAgentField returns a jsonpath field value from the trigger-mode
// AgentRuntime in the deploy workspace namespace.
func deployTriggerAgentField(jsonpath string) (string, error) {
	cmd := exec.Command("kubectl", "get", "agentruntime", deployTriggerAgentName,
		"-n", deployWorkspaceNS, "-o", "jsonpath="+jsonpath)
	return utils.Run(cmd)
}

var _ = Describe("Deploy Intent API", Ordered, Label("deploy"), func() {
	var deployKey *rsa.PrivateKey

	BeforeAll(func() {
		deployKey = generateDeployKey()
		deployRunKid = fmt.Sprintf("e2e-deploy-key-%d", time.Now().UnixNano())

		By("ensuring the controller-manager is deployed")
		Expect(ensureManagerDeployed()).To(Succeed())

		By("serving the public JWKS in-cluster")
		deployRecreateJWKS(jwksJSON(&deployKey.PublicKey, deployRunKid))

		By("enabling the deploy API on the manager (bind addr + JWKS URL)")
		Expect(patchManagerDeployArgs()).To(Succeed())

		By("exposing the deploy API via a ClusterIP Service")
		Expect(deployApplyStdin(deployAPIServiceManifest())).To(Succeed())

		By("creating the workspace + editor grant")
		Expect(deployApplyStdin(deployWorkspaceManifest())).To(Succeed())

		By("creating the curl helper pod")
		Expect(deployApplyStdin(deployCurlPodManifest())).To(Succeed())
		deployWaitPodRunning(deployCurlPod)
	})

	AfterAll(func() {
		if skipCleanup {
			_, _ = fmt.Fprintf(GinkgoWriter,
				"Skipping deploy-intent cleanup (E2E_SKIP_CLEANUP=true)\n")
			return
		}
		By("restoring the manager's canonical base args (undo deploy-API patch)")
		if err := restoreManagerBaseArgs(); err != nil {
			_, _ = fmt.Fprintf(GinkgoWriter, "failed to restore manager base args: %v\n", err)
		}
		for _, res := range []struct{ kind, ns, name string }{
			{"pod", namespace, deployCurlPod},
			{"pod", namespace, deployJWKSObject},
			{"service", namespace, deployJWKSObject},
			{"service", namespace, deployAPIService},
			{"configmap", namespace, deployJWKSObject},
			{"workspace", "", deployWorkspace},
			{"namespace", "", deployWorkspaceNS},
		} {
			args := []string{"delete", res.kind, res.name, "--ignore-not-found", "--timeout=60s"}
			if res.ns != "" {
				args = append(args, "-n", res.ns)
			}
			_, _ = utils.Run(exec.Command("kubectl", args...))
		}
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		out, _ := utils.Run(exec.Command("kubectl", "logs",
			"deployment/omnia-controller-manager", "-n", namespace,
			"--all-containers", "--tail=80"))
		_, _ = fmt.Fprintf(GinkgoWriter, "manager logs:\n%s\n", out)
		out, _ = utils.Run(exec.Command("kubectl", "get",
			"promptpack,agentruntime,configmap", "-n", deployWorkspaceNS))
		_, _ = fmt.Fprintf(GinkgoWriter, "workspace objects:\n%s\n", out)
	})

	It("verifies a freshly minted token against a local verifier (sanity gate)", func() {
		resolver := &auth.StaticKeyResolver{
			Keys: map[string]*rsa.PublicKey{deployRunKid: &deployKey.PublicKey},
		}
		verifier := authz.NewIdentityVerifier(resolver)
		token := mintDeployToken(deployKey, deployRunKid, deployWorkspace)

		id, err := verifier.Verify(context.Background(), token)
		Expect(err).NotTo(HaveOccurred())
		Expect(id.Identity).To(Equal(deployIdentity))
		Expect(id.Workspace).To(Equal(deployWorkspace))
	})

	It("materializes PromptPack, ConfigMap and AgentRuntime from a DeployIntent", func() {
		token := mintDeployToken(deployKey, deployRunKid, deployWorkspace)

		By("posting the DeployIntent with an editor token")
		var body, status string
		Eventually(func(g Gomega) {
			out, err := curlDeploy("-X", "POST",
				"-H", "Authorization: Bearer "+token,
				"-H", "Content-Type: application/json",
				"-d", deployIntentBody(), deployAPIURL())
			g.Expect(err).NotTo(HaveOccurred())
			body, status = splitCurlResponse(out)
			g.Expect(status).To(Equal("200"), "body: "+body)
		}, 2*time.Minute, 3*time.Second).Should(Succeed())

		By("asserting the deploy succeeded")
		var result deploy.DeployResult
		Expect(json.Unmarshal([]byte(body), &result)).To(Succeed(), "body: "+body)
		Expect(result.Succeeded).To(BeTrue(), "results: "+body)

		By("asserting the materialized objects exist")
		deployObjectExists("configmap", deployContentConfigMapName())
		deployObjectExists("promptpack",
			omniav1alpha1.PromptPackObjectName(deployPackName, deployPackVersion))
		deployObjectExists("agentruntime", deployAgentName)
	})

	It("rejects unauthenticated / wrong-workspace tokens", func() {
		By("rejecting a request with no Authorization header (401)")
		out, err := curlDeploy("-X", "POST",
			"-H", "Content-Type: application/json",
			"-d", "{}", deployAPIURL())
		Expect(err).NotTo(HaveOccurred())
		_, status := splitCurlResponse(out)
		Expect(status).To(Equal("401"))

		By("rejecting a token whose workspace claim != path workspace (403)")
		// authorize() (internal/api/authz/workspace_role.go) returns exactly 403
		// for a workspace-claim/path mismatch, which precisely proves path-binding
		// (a bad/expired token would 401 instead).
		wrongToken := mintDeployToken(deployKey, deployRunKid, "some-other-ws")
		out, err = curlDeploy("-X", "POST",
			"-H", "Authorization: Bearer "+wrongToken,
			"-H", "Content-Type: application/json",
			"-d", "{}", deployAPIURL())
		Expect(err).NotTo(HaveOccurred())
		_, status = splitCurlResponse(out)
		Expect(status).To(Equal("403"))
	})

	It("canaries a trigger-mode agent on a version bump", func() {
		token := mintDeployToken(deployKey, deployRunKid, deployWorkspace)

		By("creating the mock provider the trigger-mode agent binds to")
		Expect(deployApplyStdin(deployTriggerProviderManifest())).To(Succeed())

		By("posting the initial trigger-mode DeployIntent at pack version 1.0.0")
		postDeployIntent(token, deployTriggerIntentBody(deployTriggerVersionV1))

		By("asserting the AgentRuntime materialized with the version trigger configured")
		deployObjectExists("agentruntime", deployTriggerAgentName)
		Eventually(func(g Gomega) {
			out, err := deployTriggerAgentField("{.spec.rollout.trigger.promptPackChannel}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(out).To(Equal(deployTriggerChannel))
		}, 2*time.Minute, 3*time.Second).Should(Succeed())

		By("waiting for the stable pin to come up (status.activeVersion == 1.0.0)")
		// The version trigger only fires once a stable version is active — until
		// then resolveTriggerCandidate treats it as a first deploy and canaries
		// nothing (rollout_version_trigger.go).
		Eventually(func(g Gomega) {
			out, err := deployTriggerAgentField("{.status.activeVersion}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(out).To(Equal(deployTriggerVersionV1))
		}, 3*time.Minute, 3*time.Second).Should(Succeed())

		By("posting a second DeployIntent bumping the same agent's pack to 1.1.0")
		postDeployIntent(token, deployTriggerIntentBody(deployTriggerVersionV2))

		By("asserting the 1.1.0 PromptPack object was materialized")
		deployObjectExists("promptpack",
			omniav1alpha1.PromptPackObjectName(deployTriggerPackName, deployTriggerVersionV2))

		By("asserting a canary started: candidate -> 1.1.0 while the stable pin stays 1.0.0")
		// The rollout-aware apply (reconcileAgentRuntimeSpec) preserves the live
		// pin on a trigger-mode agent, and the #1838 controller sets the newer
		// channel version as spec.rollout.candidate — a canary, NOT a hard swap
		// of spec.promptPackRef to 1.1.0.
		Eventually(func(g Gomega) {
			candidate, err := deployTriggerAgentField("{.spec.rollout.candidate.promptPackRef.version}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(candidate).To(Equal(deployTriggerVersionV2), "a canary must start on the 1.1.0 pack")

			pin, err := deployTriggerAgentField("{.spec.promptPackRef.version}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pin).To(Equal(deployTriggerVersionV1), "the stable pin must be preserved, not hard-swapped")
		}, 3*time.Minute, 3*time.Second).Should(Succeed())

		By("asserting the candidate references the same logical pack name")
		candidateName, err := deployTriggerAgentField("{.spec.rollout.candidate.promptPackRef.name}")
		Expect(err).NotTo(HaveOccurred())
		Expect(candidateName).To(Equal(deployTriggerPackName))
	})
})
