//go:build e2e
// +build e2e

/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/altairalabs/omnia/test/utils"
)

// memoryE2ENamespace isolates this spec's postgres + memory-api so it can run
// after the Manager Ordered tears down omnia-system + test-agents in its
// AfterAll. The spec is fully self-contained: it deploys its own infra and
// cleans it up regardless of where in the suite Ginkgo schedules it.
const (
	memoryE2ENamespace      = "memory-e2e"
	memoryE2EPostgresApp    = "memory-e2e-postgres"
	memoryE2EApiApp         = "memory-e2e-api"
	memoryE2EApiServiceFQDN = "memory-e2e-api.memory-e2e.svc.cluster.local:8080"
	memoryE2ETestPod        = "memory-tier-test"
	// Synthetic workspace UID. memory-api in standalone mode (no --workspace
	// flag) doesn't validate against a real Workspace CRD; it accepts any
	// string as the value of the ?workspace= query param.
	memoryE2EWorkspaceUID = "00000000-0000-0000-0000-00000017e2ed"
)

var _ = Describe("Memory API tier", Ordered, func() {
	BeforeAll(func() {
		By("creating the memory-e2e namespace")
		cmd := exec.Command("kubectl", "create", "ns", memoryE2ENamespace)
		_, _ = utils.Run(cmd) // ignore AlreadyExists

		By("deploying postgres + omnia_memory db init for memory-api")
		postgresManifest := `
apiVersion: v1
kind: Secret
metadata:
  name: memory-e2e-postgres-conn
  namespace: memory-e2e
type: Opaque
stringData:
  connection-string: "postgres://omnia:omnia@memory-e2e-postgres.memory-e2e.svc.cluster.local:5432/omnia_memory?sslmode=disable"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: memory-e2e-postgres-init
  namespace: memory-e2e
data:
  # Postgres official image runs everything in /docker-entrypoint-initdb.d on
  # first boot. memory-api expects its own database (separate schema_migrations
  # table) per internal/memory/postgres/migrator.go.
  init.sql: |
    CREATE DATABASE omnia_memory OWNER omnia;
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: memory-e2e-postgres
  namespace: memory-e2e
spec:
  replicas: 1
  selector:
    matchLabels:
      app: memory-e2e-postgres
  template:
    metadata:
      labels:
        app: memory-e2e-postgres
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
          name: memory-e2e-postgres-init
---
apiVersion: v1
kind: Service
metadata:
  name: memory-e2e-postgres
  namespace: memory-e2e
spec:
  selector:
    app: memory-e2e-postgres
  ports:
  - port: 5432
    targetPort: 5432
`
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(postgresManifest)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy postgres")

		By("waiting for postgres to be ready")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pods", "-n", memoryE2ENamespace,
				"-l", "app="+memoryE2EPostgresApp,
				"-o", "jsonpath={.items[0].status.conditions[?(@.type=='Ready')].status}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("True"))
		}, 4*time.Minute, time.Second).Should(Succeed())

		By("deploying memory-api")
		memoryApiManifest := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: memory-e2e-api
  namespace: memory-e2e
spec:
  replicas: 1
  selector:
    matchLabels:
      app: memory-e2e-api
  template:
    metadata:
      labels:
        app: memory-e2e-api
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: memory-api
        image: %s
        ports:
        - name: api
          containerPort: 8080
        - name: health
          containerPort: 8081
        env:
        - name: POSTGRES_CONN
          valueFrom:
            secretKeyRef:
              name: memory-e2e-postgres-conn
              key: connection-string
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
  name: memory-e2e-api
  namespace: memory-e2e
spec:
  selector:
    app: memory-e2e-api
  ports:
  - port: 8080
    targetPort: 8080
`, memoryApiImage)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(memoryApiManifest)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy memory-api")

		By("waiting for memory-api to be ready")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pods", "-n", memoryE2ENamespace,
				"-l", "app="+memoryE2EApiApp,
				"-o", "jsonpath={.items[0].status.conditions[?(@.type=='Ready')].status}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("True"))
		}, 4*time.Minute, time.Second).Should(Succeed())

		By("seeding analytics:aggregate consent for the consenting user")
		// Memory-api ran its migrations on startup → user_privacy_preferences
		// table exists. We pre-seed a row for the consenting user; the
		// non-consenting user has no row, so the consent join filters their
		// memories from the user-tier aggregate by construction.
		seedSQL := `
INSERT INTO user_privacy_preferences (user_id, consent_grants)
VALUES ('e2e-user-consenting', ARRAY['analytics:aggregate'])
ON CONFLICT (user_id) DO UPDATE SET consent_grants = EXCLUDED.consent_grants;
`
		cmd = exec.Command("kubectl", "exec", "-n", memoryE2ENamespace,
			"deployment/memory-e2e-postgres", "--",
			"psql", "-U", "omnia", "-d", "omnia_memory", "-c", seedSQL)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to seed analytics:aggregate consent")
	})

	AfterAll(func() {
		if CurrentSpecReport().Failed() {
			_, _ = fmt.Fprintf(GinkgoWriter, "\n=== spec failed — leaving %s namespace intact for diagnostics ===\n",
				memoryE2ENamespace)
			memApiLogs := exec.Command("kubectl", "logs", "-n", memoryE2ENamespace,
				"-l", "app="+memoryE2EApiApp, "--tail=200")
			if logs, err := utils.Run(memApiLogs); err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "memory-api logs:\n%s\n", logs)
			}
			testLogs := exec.Command("kubectl", "logs", memoryE2ETestPod,
				"-n", memoryE2ENamespace)
			if logs, err := utils.Run(testLogs); err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "test pod logs:\n%s\n", logs)
			}
			return
		}

		By("cleaning up memory-e2e namespace")
		cmd := exec.Command("kubectl", "delete", "ns", memoryE2ENamespace,
			"--ignore-not-found", "--force", "--grace-period=0", "--timeout=60s")
		_, _ = utils.Run(cmd)
	})

	It("derives tier on list responses, aggregates by tier, and respects analytics:aggregate consent", func() {
		By("deploying the python memory-tier test pod")
		testPodManifest := fmt.Sprintf(`
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
  - name: test
    image: python:3.13-slim
    env:
    - name: WORKSPACE_UID
      value: %q
    - name: MEMORY_API
      value: "http://%s"
    securityContext:
      readOnlyRootFilesystem: false
      allowPrivilegeEscalation: false
      capabilities:
        drop: ["ALL"]
    command: ["python3", "-c"]
    args:
    - |
      import json
      import os
      import sys
      import urllib.request
      import urllib.error

      MEMORY_API = os.environ["MEMORY_API"]
      WORKSPACE_UID = os.environ["WORKSPACE_UID"]
      # agent_id is a UUID column on memory_entities, so AGENT_ID must be a
      # valid UUID string. virtual_user_id is plain TEXT so the user IDs
      # below stay as readable strings.
      AGENT_ID = "00000000-0000-0000-0000-0000000a9e87"
      CONSENTING_USER = "e2e-user-consenting"
      NON_CONSENTING_USER = "e2e-user-no-consent"

      # Each tier endpoint has its own request shape:
      #   /api/v1/institutional/memories — flat workspace_id
      #   /api/v1/agent-memories         — flat workspace_id + agent_id
      #   /api/v1/memories               — nested scope map
      INSTITUTIONAL = {
          "workspace_id": WORKSPACE_UID,
          "type": "fact",
          "content": "Institutional memory: company refund policy is 30 days.",
          "confidence": 0.95,
      }
      AGENT = {
          "workspace_id": WORKSPACE_UID,
          "agent_id": AGENT_ID,
          "type": "fact",
          "content": "Agent memory: customers asking about overages prefer credits.",
          "confidence": 0.85,
      }
      USER_CONSENTING = {
          "type": "preference",
          "content": "User memory: this user prefers dark mode.",
          "confidence": 0.9,
          "scope": {"workspace_id": WORKSPACE_UID, "user_id": CONSENTING_USER},
      }
      USER_NO_CONSENT = {
          "type": "preference",
          "content": "User memory: should be filtered out of aggregate.",
          "confidence": 0.9,
          "scope": {"workspace_id": WORKSPACE_UID, "user_id": NON_CONSENTING_USER},
      }


      def _request(method, path, body=None):
          url = f"{MEMORY_API}{path}"
          data = None
          headers = {"Accept": "application/json"}
          if body is not None:
              data = json.dumps(body).encode("utf-8")
              headers["Content-Type"] = "application/json"
          req = urllib.request.Request(url, data=data, method=method, headers=headers)
          try:
              with urllib.request.urlopen(req, timeout=10) as resp:
                  payload = resp.read().decode("utf-8")
                  return resp.status, payload
          except urllib.error.HTTPError as e:
              return e.code, e.read().decode("utf-8")


      def post(path, body):
          status, payload = _request("POST", path, body)
          if status >= 300:
              print(f"FAIL POST {path}: {status} {payload}", flush=True)
              sys.exit(1)


      def get_json(path):
          status, payload = _request("GET", path)
          if status >= 300:
              print(f"FAIL GET {path}: {status} {payload}", flush=True)
              sys.exit(1)
          return json.loads(payload)


      print("=== Step 1: POST one memory per tier (+ a non-consenting user) ===", flush=True)
      post("/api/v1/institutional/memories", INSTITUTIONAL)
      post("/api/v1/agent-memories", AGENT)
      post("/api/v1/memories", USER_CONSENTING)
      post("/api/v1/memories", USER_NO_CONSENT)

      print("=== Step 2: each list endpoint carries the expected tier ===", flush=True)

      inst_list = get_json(f"/api/v1/institutional/memories?workspace={WORKSPACE_UID}")
      assert inst_list["memories"], f"institutional list empty: {inst_list}"
      for m in inst_list["memories"]:
          assert m.get("tier") == "institutional", \
              f"institutional row missing tier=institutional: {json.dumps(m)}"

      agent_list = get_json(
          f"/api/v1/agent-memories?workspace={WORKSPACE_UID}&agent={AGENT_ID}")
      assert agent_list["memories"], f"agent list empty: {agent_list}"
      for m in agent_list["memories"]:
          assert m.get("tier") == "agent", \
              f"agent row missing tier=agent: {json.dumps(m)}"

      user_list = get_json(
          f"/api/v1/memories?workspace={WORKSPACE_UID}&user_id={CONSENTING_USER}")
      assert user_list["memories"], f"user list empty: {user_list}"
      for m in user_list["memories"]:
          assert m.get("tier") == "user", \
              f"user row missing tier=user: {json.dumps(m)}"

      print("=== Step 3: groupBy=tier aggregates correctly ===", flush=True)
      agg = get_json(
          f"/api/v1/memories/aggregate?workspace={WORKSPACE_UID}&groupBy=tier&metric=count")
      counts = {row["key"]: row["value"] for row in agg}
      print(f"tier counts: {counts}", flush=True)
      assert counts.get("institutional", 0) >= 1, f"missing institutional: {counts}"
      assert counts.get("agent", 0) >= 1, f"missing agent: {counts}"
      # Only the consenting user's row should count toward the user tier.
      # Non-consenting user is filtered by AggregateConsentJoin.
      user_count = counts.get("user", 0)
      assert user_count == 1, \
          f"expected user tier count == 1 (consenting user only), got {user_count} ({counts})"

      print("=== Step 4: category aggregate also responds ===", flush=True)
      cat_agg = get_json(
          f"/api/v1/memories/aggregate?workspace={WORKSPACE_UID}&groupBy=category&metric=count")
      assert cat_agg, f"category aggregate empty: {cat_agg}"

      print("PASS", flush=True)
`, memoryE2ETestPod, memoryE2ENamespace, memoryE2EWorkspaceUID, memoryE2EApiServiceFQDN)

		applyCmd := exec.Command("kubectl", "apply", "-f", "-")
		applyCmd.Stdin = strings.NewReader(testPodManifest)
		_, err := utils.Run(applyCmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create memory-tier-test pod")

		By("waiting for the memory-tier test pod to complete")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pod", memoryE2ETestPod,
				"-n", memoryE2ENamespace, "-o", "jsonpath={.status.phase}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("Succeeded"))
		}, 5*time.Minute, 2*time.Second).Should(Succeed())
	})
})
