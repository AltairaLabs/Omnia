//go:build e2e
// +build e2e

/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/altairalabs/omnia/test/utils"
)

// Self-contained namespace + Postgres + memory-api + stub function
// server. Runs after the Manager Ordered suite tears down
// omnia-system, mirroring memory_e2e_test.go's isolation pattern.
const (
	consE2ENamespace      = "consolidation-e2e"
	consE2EPostgresApp    = "consolidation-e2e-postgres"
	consE2EApiApp         = "consolidation-e2e-api"
	consE2EStubApp        = "consolidation-fn-stub"
	consE2EStubImage      = "consolidation-fn-stub:e2e"
	consE2EWorkspaceName  = "consolidation-e2e-ws"
	consE2EPolicyName     = "consolidation-e2e-policy"
	consE2EStubAxis       = "staleObservations"
	consE2EMemoryAPIAddr  = "consolidation-e2e-api.consolidation-e2e.svc.cluster.local:8080"
	consE2EStubFQDN       = "consolidation-fn-stub.consolidation-e2e.svc.cluster.local"
	consE2EConsolidateInt = "10s"
)

// consE2EWorkspaceUID is populated in BeforeAll by reading the actual
// metadata.uid the API server assigned to the Workspace CR. Kubernetes
// generates uids on create and ignores any user-supplied value, so we
// can't define this as a const. The worker's WorkspaceLister returns
// the real uid; the seed SQL + assertions must use that same value.
var consE2EWorkspaceUID string

var _ = Describe("Memory consolidation worker", Ordered, Label("consolidation"), func() {
	BeforeAll(func() {
		By("creating the consolidation-e2e namespace")
		_, _ = utils.Run(exec.Command("kubectl", "create", "ns", consE2ENamespace))

		By("building the consolidation-fn-stub image")
		buildCmd := exec.Command("docker", "build",
			"-t", consE2EStubImage,
			"test/e2e/fixtures/consolidation-fn-stub")
		_, err := utils.Run(buildCmd)
		Expect(err).NotTo(HaveOccurred(), "docker build consolidation-fn-stub failed")

		By("loading the stub image into kind")
		kindCluster := os.Getenv("KIND_CLUSTER")
		if kindCluster == "" {
			kindCluster = "kind"
		}
		loadCmd := exec.Command("kind", "load", "docker-image", consE2EStubImage, "--name", kindCluster)
		_, err = utils.Run(loadCmd)
		Expect(err).NotTo(HaveOccurred(), "kind load consolidation-fn-stub failed")

		By("deploying Postgres + omnia_memory DB init")
		postgresManifest := `
apiVersion: v1
kind: Secret
metadata:
  name: consolidation-e2e-postgres-conn
  namespace: consolidation-e2e
type: Opaque
stringData:
  # POSTGRES_CONN is the canonical key memory-api reads (it env-binds
  # POSTGRES_CONN directly) and the key the Workspace CRD's
  # spec.services[].memory.database.secretRef expects.
  POSTGRES_CONN: "postgres://omnia:omnia@consolidation-e2e-postgres.consolidation-e2e.svc.cluster.local:5432/omnia_memory?sslmode=disable"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: consolidation-e2e-postgres-init
  namespace: consolidation-e2e
data:
  init.sql: |
    CREATE DATABASE omnia_memory OWNER omnia;
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: consolidation-e2e-postgres
  namespace: consolidation-e2e
spec:
  replicas: 1
  selector:
    matchLabels:
      app: consolidation-e2e-postgres
  template:
    metadata:
      labels:
        app: consolidation-e2e-postgres
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 70
        fsGroup: 70
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: postgres
        image: pgvector/pgvector:pg17
        ports:
        - containerPort: 5432
        env:
        - name: POSTGRES_USER
          value: omnia
        - name: POSTGRES_PASSWORD
          value: omnia
        - name: POSTGRES_DB
          value: omnia
        - name: PGDATA
          value: /tmp/pgdata
        volumeMounts:
        - name: init
          mountPath: /docker-entrypoint-initdb.d
          readOnly: true
        readinessProbe:
          exec:
            command: ["pg_isready", "-U", "omnia", "-d", "omnia_memory"]
          initialDelaySeconds: 5
          periodSeconds: 5
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
      volumes:
      - name: init
        configMap:
          name: consolidation-e2e-postgres-init
---
apiVersion: v1
kind: Service
metadata:
  name: consolidation-e2e-postgres
  namespace: consolidation-e2e
spec:
  selector:
    app: consolidation-e2e-postgres
  ports:
  - port: 5432
    targetPort: 5432
`
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(postgresManifest)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy postgres")

		By("waiting for postgres to be ready")
		Eventually(func(g Gomega) {
			out, err := utils.Run(exec.Command("kubectl", "get", "pods", "-n", consE2ENamespace,
				"-l", "app="+consE2EPostgresApp,
				"-o", "jsonpath={.items[0].status.conditions[?(@.type=='Ready')].status}"))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(out).To(Equal("True"))
		}, 4*time.Minute, time.Second).Should(Succeed())

		By("deploying the consolidation-fn-stub Service")
		stubManifest := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %[1]s
  template:
    metadata:
      labels:
        app: %[1]s
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: stub
        image: %[3]s
        imagePullPolicy: Never
        ports:
        - containerPort: 8080
        readinessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 1
          periodSeconds: 2
        securityContext:
          readOnlyRootFilesystem: true
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
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
  - port: 8080
    targetPort: 8080
`, consE2EStubApp, consE2ENamespace, consE2EStubImage)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(stubManifest)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy stub")

		By("ensuring Omnia CRDs are installed (needed for MemoryPolicy + Workspace)")
		Expect(ensureManagerDeployed()).To(Succeed())

		By("deploying memory-api with --enterprise and --consolidation-interval=10s")
		memoryApiManifest := fmt.Sprintf(`
apiVersion: v1
kind: ServiceAccount
metadata:
  name: consolidation-e2e-api
  namespace: consolidation-e2e
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: consolidation-e2e-api
rules:
# Consolidation worker: MemoryPolicy lister + Workspace lister.
# Privacy middleware watcher (--enterprise mode): SessionPrivacyPolicy
# + Workspace + AgentRuntime (all three are listed during initial sync;
# missing any one fails the watcher at startup with HTTP 403).
- apiGroups: ["omnia.altairalabs.ai"]
  resources: ["workspaces", "memorypolicies", "sessionprivacypolicies", "agentruntimes"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: consolidation-e2e-api
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: consolidation-e2e-api
subjects:
- kind: ServiceAccount
  name: consolidation-e2e-api
  namespace: consolidation-e2e
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: consolidation-e2e-api
  namespace: consolidation-e2e
spec:
  replicas: 1
  selector:
    matchLabels:
      app: consolidation-e2e-api
  template:
    metadata:
      labels:
        app: consolidation-e2e-api
    spec:
      serviceAccountName: consolidation-e2e-api
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: memory-api
        image: %[1]s
        args:
        - --enterprise=true
        - --consolidation-interval=%[2]s
        # memory-api is per-workspace: the operator injects --workspace in prod,
        # so the consolidation + privacy-watcher paths Get their OWN Workspace by
        # name rather than listing the cluster (#1899). This standalone manifest
        # must set it too, or ForPolicy short-circuits to zero workspaces.
        - --workspace=%[3]s
        ports:
        - name: api
          containerPort: 8080
        - name: health
          containerPort: 8081
        env:
        - name: POSTGRES_CONN
          valueFrom:
            secretKeyRef:
              name: consolidation-e2e-postgres-conn
              key: POSTGRES_CONN
        readinessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 5
        securityContext:
          readOnlyRootFilesystem: true
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
---
apiVersion: v1
kind: Service
metadata:
  name: consolidation-e2e-api
  namespace: consolidation-e2e
spec:
  selector:
    app: consolidation-e2e-api
  ports:
  - port: 8080
    targetPort: 8080
`, memoryApiImage, consE2EConsolidateInt, consE2EWorkspaceName)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(memoryApiManifest)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy memory-api")

		By("waiting for memory-api to be ready")
		Eventually(func(g Gomega) {
			out, err := utils.Run(exec.Command("kubectl", "get", "pods", "-n", consE2ENamespace,
				"-l", "app="+consE2EApiApp,
				"-o", "jsonpath={.items[0].status.conditions[?(@.type=='Ready')].status}"))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(out).To(Equal("True"))
		}, 4*time.Minute, time.Second).Should(Succeed())

		By("applying Workspace + MemoryPolicy CRs")
		crManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: MemoryPolicy
metadata:
  name: %[1]s
spec:
  tiers:
    user: { mode: "Decay" }
  consolidation:
    # Every-minute cadence so the staleObservations axis becomes due
    # within the test's 3-minute Eventually window. Per-axis scheduling
    # (#1152) gates each axis on its cron; the default "0 2 * * *" would
    # never fire during the test.
    schedule: "* * * * *"
    functionRefs:
      staleObservations:
        name: %[2]s
        namespace: %[3]s
    candidateLimits:
      maxBucketsPerPass: 10
      maxPerBucket: 50
    safetyGates:
      requirePIIRedaction: false
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Workspace
metadata:
  name: %[4]s
spec:
  displayName: Consolidation E2E
  namespace:
    name: %[3]s
  services:
  - name: default
    mode: external
    memory:
      database:
        secretRef:
          name: consolidation-e2e-postgres-conn
      policyRef:
        name: %[1]s
    external:
      sessionURL: "http://unused.consolidation-e2e.svc.cluster.local:8080"
      memoryURL: "http://%[5]s"
`, consE2EPolicyName, consE2EStubApp, consE2ENamespace, consE2EWorkspaceName, consE2EMemoryAPIAddr)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(crManifest)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply MemoryPolicy + Workspace")

		By("reading the Workspace's server-assigned metadata.uid")
		// metadata.uid is generated by the Kubernetes API server on
		// create; any user-supplied value is ignored. The worker's
		// WorkspaceLister returns this uid, so the seed SQL must use
		// the same value (otherwise the pre-filter SELECT matches 0
		// rows and the worker never calls the stub).
		Eventually(func(g Gomega) {
			out, err := utils.Run(exec.Command("kubectl", "get", "workspace", consE2EWorkspaceName,
				"-o", "jsonpath={.metadata.uid}"))
			g.Expect(err).NotTo(HaveOccurred())
			uid := strings.TrimSpace(out)
			g.Expect(uid).NotTo(BeEmpty(), "Workspace UID not yet populated")
			consE2EWorkspaceUID = uid
		}, 30*time.Second, time.Second).Should(Succeed())
		_, _ = fmt.Fprintf(GinkgoWriter, "Workspace UID: %s\n", consE2EWorkspaceUID)
	})

	AfterAll(func() {
		if CurrentSpecReport().Failed() {
			_, _ = fmt.Fprintf(GinkgoWriter, "\n=== spec failed — leaving %s intact ===\n", consE2ENamespace)
			memLogs := exec.Command("kubectl", "logs", "-n", consE2ENamespace,
				"-l", "app="+consE2EApiApp, "--tail=200")
			if logs, err := utils.Run(memLogs); err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "memory-api logs:\n%s\n", logs)
			}
			stubLogs := exec.Command("kubectl", "logs", "-n", consE2ENamespace,
				"-l", "app="+consE2EStubApp, "--tail=200")
			if logs, err := utils.Run(stubLogs); err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "stub logs:\n%s\n", logs)
			}
			return
		}
		By("cleaning up")
		_, _ = utils.Run(exec.Command("kubectl", "delete", "ns", consE2ENamespace,
			"--ignore-not-found", "--force", "--grace-period=0", "--timeout=60s"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "clusterrolebinding", "consolidation-e2e-api", "--ignore-not-found"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "clusterrole", "consolidation-e2e-api", "--ignore-not-found"))
	})

	It("runs a pass, writes a summary with lineage, and emits an audit row", func() {
		By("seeding stale observations directly via SQL")
		// Worker's buildPreFilterOptions hardcodes MinGroupSize: 5, so we
		// need at least 5 observations sharing the same (user, agent, kind,
		// name) tuple before the pre-filter surfaces the bucket. Seed 6.
		seedSQL := fmt.Sprintf(`
INSERT INTO memory_entities (id, workspace_id, virtual_user_id, agent_id, name, kind)
VALUES
  ('11111111-1111-1111-1111-111111111111', '%[1]s', 'e2e-user', NULL, 'favorite_color', 'fact')
ON CONFLICT (id) DO NOTHING;
INSERT INTO memory_observations (entity_id, content, observed_at, mutability)
VALUES
  ('11111111-1111-1111-1111-111111111111', 'blue',     NOW() - INTERVAL '70 days', 'mutable'),
  ('11111111-1111-1111-1111-111111111111', 'azure',    NOW() - INTERVAL '65 days', 'mutable'),
  ('11111111-1111-1111-1111-111111111111', 'navy',     NOW() - INTERVAL '60 days', 'mutable'),
  ('11111111-1111-1111-1111-111111111111', 'cobalt',   NOW() - INTERVAL '55 days', 'mutable'),
  ('11111111-1111-1111-1111-111111111111', 'cerulean', NOW() - INTERVAL '50 days', 'mutable'),
  ('11111111-1111-1111-1111-111111111111', 'sapphire', NOW() - INTERVAL '45 days', 'mutable');
`, consE2EWorkspaceUID)
		_, err := utils.Run(exec.Command("kubectl", "exec", "-n", consE2ENamespace,
			"deployment/"+consE2EPostgresApp, "--",
			"psql", "-U", "omnia", "-d", "omnia_memory", "-c", seedSQL))
		Expect(err).NotTo(HaveOccurred(), "seed failed")

		By("waiting for the worker to materialize an ai_summary row")
		Eventually(func(g Gomega) {
			out, err := utils.Run(exec.Command("kubectl", "exec", "-n", consE2ENamespace,
				"deployment/"+consE2EPostgresApp, "--",
				"psql", "-U", "omnia", "-d", "omnia_memory", "-tAc",
				`SELECT COUNT(*) FROM memory_observations
                  WHERE source_type='ai_summary'
                  AND promoted_by_pack IS NOT NULL`))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(out)).NotTo(Equal("0"), "no ai_summary rows yet")
		}, 3*time.Minute, 5*time.Second).Should(Succeed())

		By("verifying lineage columns are populated on the summary")
		out, err := utils.Run(exec.Command("kubectl", "exec", "-n", consE2ENamespace,
			"deployment/"+consE2EPostgresApp, "--",
			"psql", "-U", "omnia", "-d", "omnia_memory", "-tAc",
			`SELECT promoted_by_pack, array_length(promoted_from_ids, 1)
              FROM memory_observations
              WHERE source_type='ai_summary'
              ORDER BY promoted_at DESC LIMIT 1`))
		Expect(err).NotTo(HaveOccurred())
		row := strings.TrimSpace(out)
		Expect(row).To(ContainSubstring(consE2EStubApp), "promoted_by_pack should be the stub name")
		Expect(row).NotTo(ContainSubstring("|0"), "promoted_from_ids should have entries")

		By("verifying a consolidation audit row landed")
		Eventually(func(g Gomega) {
			out, err := utils.Run(exec.Command("kubectl", "exec", "-n", consE2ENamespace,
				"deployment/"+consE2EPostgresApp, "--",
				"psql", "-U", "omnia", "-d", "omnia_memory", "-tAc",
				`SELECT COUNT(*) FROM audit_log
                  WHERE event_type='memory.consolidation.create_summary'`))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(out)).NotTo(Equal("0"), "no consolidation audit row")
		}, 2*time.Minute, 5*time.Second).Should(Succeed())

		By("verifying the audit row's consolidation_run_id metadata references the workspace UID")
		auditOut, err := utils.Run(exec.Command("kubectl", "exec", "-n", consE2ENamespace,
			"deployment/"+consE2EPostgresApp, "--",
			"psql", "-U", "omnia", "-d", "omnia_memory", "-tAc",
			`SELECT metadata->>'consolidation_run_id'
              FROM audit_log
              WHERE event_type='memory.consolidation.create_summary'
              ORDER BY timestamp DESC LIMIT 1`))
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.TrimSpace(auditOut)).To(HavePrefix(consE2EWorkspaceUID),
			"consolidation_run_id should start with the workspace UID")
	})
})
