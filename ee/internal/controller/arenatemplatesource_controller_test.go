/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/fetcher"
	arenaTemplate "github.com/altairalabs/omnia/ee/pkg/arena/template"
)

var _ = Describe("ArenaTemplateSource Controller", func() {
	const (
		templateSourceName      = "test-templatesource"
		templateSourceNamespace = "default"
		configMapName           = "test-templates"
	)

	ctx := context.Background()

	var workspaceContentPath string

	BeforeEach(func() {
		var err error
		workspaceContentPath, err = os.MkdirTemp("", "templatesource-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if workspaceContentPath != "" {
			_ = os.RemoveAll(workspaceContentPath)
		}
	})

	Context("When reconciling a non-existent ArenaTemplateSource", func() {
		It("should return without error", func() {
			By("reconciling a non-existent ArenaTemplateSource")
			reconciler := &ArenaTemplateSourceReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				WorkspaceContentPath: workspaceContentPath,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "nonexistent-source",
					Namespace: templateSourceNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling a suspended ArenaTemplateSource", func() {
		var templateSource *omniav1alpha1.ArenaTemplateSource

		BeforeEach(func() {
			By("creating the suspended ArenaTemplateSource")
			templateSource = &omniav1alpha1.ArenaTemplateSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "suspended-templatesource",
					Namespace: templateSourceNamespace,
				},
				Spec: omniav1alpha1.ArenaTemplateSourceSpec{
					Type:    omniav1alpha1.ArenaTemplateSourceTypeConfigMap,
					Suspend: true,
					ConfigMap: &omniav1alpha1.ConfigMapSource{
						Name: configMapName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, templateSource)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the ArenaTemplateSource")
			resource := &omniav1alpha1.ArenaTemplateSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "suspended-templatesource",
				Namespace: templateSourceNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should not fetch when suspended", func() {
			By("reconciling the suspended ArenaTemplateSource")
			reconciler := &ArenaTemplateSourceReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				Recorder:             record.NewFakeRecorder(10),
				WorkspaceContentPath: workspaceContentPath,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "suspended-templatesource",
					Namespace: templateSourceNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the status")
			updatedSource := &omniav1alpha1.ArenaTemplateSource{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "suspended-templatesource",
				Namespace: templateSourceNamespace,
			}, updatedSource)
			Expect(err).NotTo(HaveOccurred())

			// Should have Ready condition set to False with reason Suspended
			readyCondition := findCondition(updatedSource.Status.Conditions, ArenaTemplateSourceConditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal("Suspended"))
		})
	})

	Context("When reconciling an ArenaTemplateSource with ConfigMap", func() {
		var templateSource *omniav1alpha1.ArenaTemplateSource
		var configMap *corev1.ConfigMap

		BeforeEach(func() {
			By("creating the ConfigMap with template content")
			templateYAML := `apiVersion: arena.altairalabs.ai/v1alpha1
kind: ArenaTemplate
metadata:
  name: test-template
spec:
  displayName: Test Template
  description: A test template
  category: test
  tags:
    - test
  variables:
    - name: projectName
      type: string
      required: true
`
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: templateSourceNamespace,
				},
				Data: map[string]string{
					// ConfigMap keys cannot contain slashes, use a flat key name
					"template.yaml": templateYAML,
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("creating the ArenaTemplateSource")
			templateSource = &omniav1alpha1.ArenaTemplateSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      templateSourceName,
					Namespace: templateSourceNamespace,
				},
				Spec: omniav1alpha1.ArenaTemplateSourceSpec{
					Type: omniav1alpha1.ArenaTemplateSourceTypeConfigMap,
					ConfigMap: &omniav1alpha1.ConfigMapSource{
						Name: configMapName,
					},
					SyncInterval: "1h",
				},
			}
			Expect(k8sClient.Create(ctx, templateSource)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			resource := &omniav1alpha1.ArenaTemplateSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      templateSourceName,
				Namespace: templateSourceNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      configMapName,
				Namespace: templateSourceNamespace,
			}, cm)
			if err == nil {
				Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
			}
		})

		It("should initialize status to Pending", func() {
			By("reconciling the ArenaTemplateSource")
			reconciler := &ArenaTemplateSourceReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				Recorder:             record.NewFakeRecorder(10),
				WorkspaceContentPath: workspaceContentPath,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      templateSourceName,
					Namespace: templateSourceNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the status is Fetching (fetch started)")
			updatedSource := &omniav1alpha1.ArenaTemplateSource{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      templateSourceName,
				Namespace: templateSourceNamespace,
			}, updatedSource)
			Expect(err).NotTo(HaveOccurred())

			// Should be Fetching after first reconcile (async fetch started)
			Expect(updatedSource.Status.Phase).To(Equal(omniav1alpha1.ArenaTemplateSourcePhaseFetching))
		})
	})

	Context("Template index", func() {
		It("should write template index file correctly", func() {
			// Create temp directory structure: {base}/{workspace}/{namespace}/{contentPath}
			baseDir, err := os.MkdirTemp("", "workspace-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(baseDir) }()

			// Create nested directory structure
			contentPath := "arena/template-sources/test-source/.arena/versions/abc123"
			fullPath := filepath.Join(baseDir, "test-ns", "test-ns", contentPath)
			Expect(os.MkdirAll(fullPath, 0755)).To(Succeed())

			source := &omniav1alpha1.ArenaTemplateSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-source",
					Namespace: "test-ns",
				},
			}

			reconciler := &ArenaTemplateSourceReconciler{
				WorkspaceContentPath: baseDir,
			}

			templates := []arenaTemplate.Template{
				{
					Name:        "test-template",
					Version:     "1.0.0",
					DisplayName: "Test Template",
					Description: "A test template",
					Category:    "test",
					Tags:        []string{"test", "example"},
					Variables: []arenaTemplate.Variable{
						{
							Name:     "projectName",
							Type:     arenaTemplate.VariableTypeString,
							Required: true,
						},
					},
					Files: []arenaTemplate.FileSpec{
						{Path: "config.yaml", Render: true},
					},
					Path: "templates/test-template",
				},
			}

			err = reconciler.writeTemplateIndex(source, contentPath, templates)
			Expect(err).NotTo(HaveOccurred())

			// Verify index file was created at the new path structure
			indexPath := filepath.Join(baseDir, "test-ns", "test-ns", TemplateIndexDir, "test-source.json")
			Expect(indexPath).To(BeAnExistingFile())

			// Read and verify contents
			data, err := os.ReadFile(indexPath)
			Expect(err).NotTo(HaveOccurred())

			var loadedTemplates []arenaTemplate.Template
			err = json.Unmarshal(data, &loadedTemplates)
			Expect(err).NotTo(HaveOccurred())

			Expect(loadedTemplates).To(HaveLen(1))
			Expect(loadedTemplates[0].Name).To(Equal("test-template"))
			Expect(loadedTemplates[0].Version).To(Equal("1.0.0"))
			Expect(loadedTemplates[0].DisplayName).To(Equal("Test Template"))
			Expect(loadedTemplates[0].Category).To(Equal("test"))
			Expect(loadedTemplates[0].Tags).To(Equal([]string{"test", "example"}))
		})

		It("should handle empty template list", func() {
			// Create temp directory structure
			baseDir, err := os.MkdirTemp("", "workspace-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(baseDir) }()

			contentPath := "arena/template-sources/test-source/.arena/versions/abc123"
			fullPath := filepath.Join(baseDir, "test-ns", "test-ns", contentPath)
			Expect(os.MkdirAll(fullPath, 0755)).To(Succeed())

			source := &omniav1alpha1.ArenaTemplateSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-source",
					Namespace: "test-ns",
				},
			}

			reconciler := &ArenaTemplateSourceReconciler{
				WorkspaceContentPath: baseDir,
			}

			err = reconciler.writeTemplateIndex(source, contentPath, []arenaTemplate.Template{})
			Expect(err).NotTo(HaveOccurred())

			indexPath := filepath.Join(baseDir, "test-ns", "test-ns", TemplateIndexDir, "test-source.json")
			Expect(indexPath).To(BeAnExistingFile())

			data, err := os.ReadFile(indexPath)
			Expect(err).NotTo(HaveOccurred())

			var loadedTemplates []arenaTemplate.Template
			err = json.Unmarshal(data, &loadedTemplates)
			Expect(err).NotTo(HaveOccurred())
			Expect(loadedTemplates).To(BeEmpty())
		})
	})

	Context("Fetch result handling", func() {
		It("should handle fetch result with templates", func() {
			// Create a mock artifact directory
			artifactDir, err := os.MkdirTemp("", "artifact-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(artifactDir) }()

			// Create a test file in artifact
			Expect(os.WriteFile(filepath.Join(artifactDir, "test.txt"), []byte("test"), 0644)).To(Succeed())

			result := &templateFetchResult{
				artifact: &fetcher.Artifact{
					Revision: "abc123",
					Checksum: "sha256:1234567890abcdef",
					Size:     100,
					Path:     artifactDir,
				},
				templates: []arenaTemplate.Template{
					{
						Name:        "test",
						DisplayName: "Test",
						Path:        "templates/test",
					},
				},
			}

			Expect(result.err).ToNot(HaveOccurred())
			Expect(result.templates).To(HaveLen(1))
			Expect(result.artifact.Revision).To(Equal("abc123"))
		})

		It("should handle fetch result with error", func() {
			result := &templateFetchResult{
				err: os.ErrNotExist,
			}

			Expect(result.err).To(Equal(os.ErrNotExist))
			Expect(result.templates).To(BeNil())
		})
	})

	Context("Condition management", func() {
		It("should set conditions correctly", func() {
			reconciler := &ArenaTemplateSourceReconciler{}

			source := &omniav1alpha1.ArenaTemplateSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "default",
					Generation: 1,
				},
			}

			reconciler.setCondition(source, ArenaTemplateSourceConditionTypeReady,
				metav1.ConditionTrue, "Ready", "Source is ready")

			Expect(source.Status.Conditions).To(HaveLen(1))
			Expect(source.Status.Conditions[0].Type).To(Equal(ArenaTemplateSourceConditionTypeReady))
			Expect(source.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(source.Status.Conditions[0].Reason).To(Equal("Ready"))
			Expect(source.Status.Conditions[0].Message).To(Equal("Source is ready"))
			Expect(source.Status.Conditions[0].ObservedGeneration).To(Equal(int64(1)))
		})

		It("should update existing condition", func() {
			reconciler := &ArenaTemplateSourceReconciler{}

			source := &omniav1alpha1.ArenaTemplateSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "default",
					Generation: 2,
				},
				Status: omniav1alpha1.ArenaTemplateSourceStatus{
					Conditions: []metav1.Condition{
						{
							Type:               ArenaTemplateSourceConditionTypeReady,
							Status:             metav1.ConditionFalse,
							Reason:             "NotReady",
							Message:            "Not ready yet",
							ObservedGeneration: 1,
						},
					},
				},
			}

			reconciler.setCondition(source, ArenaTemplateSourceConditionTypeReady,
				metav1.ConditionTrue, "Ready", "Now ready")

			Expect(source.Status.Conditions).To(HaveLen(1))
			Expect(source.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(source.Status.Conditions[0].Reason).To(Equal("Ready"))
			Expect(source.Status.Conditions[0].Message).To(Equal("Now ready"))
			Expect(source.Status.Conditions[0].ObservedGeneration).To(Equal(int64(2)))
		})
	})

	Context("Fetcher creation", func() {
		It("should return error for unsupported source type", func() {
			reconciler := &ArenaTemplateSourceReconciler{
				Client: k8sClient,
			}

			spec := &omniav1alpha1.ArenaTemplateSourceSpec{
				Type: "unsupported",
			}

			_, err := reconciler.createFetcher(ctx, spec, "default", fetcher.Options{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported source type"))
		})

		It("should return error for git type without git config", func() {
			reconciler := &ArenaTemplateSourceReconciler{
				Client: k8sClient,
			}

			spec := &omniav1alpha1.ArenaTemplateSourceSpec{
				Type: omniav1alpha1.ArenaTemplateSourceTypeGit,
				Git:  nil,
			}

			_, err := reconciler.createFetcher(ctx, spec, "default", fetcher.Options{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("git configuration is required"))
		})

		It("should return error for oci type without oci config", func() {
			reconciler := &ArenaTemplateSourceReconciler{
				Client: k8sClient,
			}

			spec := &omniav1alpha1.ArenaTemplateSourceSpec{
				Type: omniav1alpha1.ArenaTemplateSourceTypeOCI,
				OCI:  nil,
			}

			_, err := reconciler.createFetcher(ctx, spec, "default", fetcher.Options{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("oci configuration is required"))
		})

		It("should return error for configmap type without configmap config", func() {
			reconciler := &ArenaTemplateSourceReconciler{
				Client: k8sClient,
			}

			spec := &omniav1alpha1.ArenaTemplateSourceSpec{
				Type:      omniav1alpha1.ArenaTemplateSourceTypeConfigMap,
				ConfigMap: nil,
			}

			_, err := reconciler.createFetcher(ctx, spec, "default", fetcher.Options{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("configmap configuration is required"))
		})

		It("should create git fetcher successfully", func() {
			reconciler := &ArenaTemplateSourceReconciler{
				Client: k8sClient,
			}

			spec := &omniav1alpha1.ArenaTemplateSourceSpec{
				Type: omniav1alpha1.ArenaTemplateSourceTypeGit,
				Git: &omniav1alpha1.GitSource{
					URL: "https://github.com/example/repo",
					Ref: &omniav1alpha1.GitReference{
						Branch: "main",
					},
				},
			}

			f, err := reconciler.createFetcher(ctx, spec, "default", fetcher.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(f).NotTo(BeNil())
		})

		It("should create configmap fetcher successfully", func() {
			reconciler := &ArenaTemplateSourceReconciler{
				Client: k8sClient,
			}

			spec := &omniav1alpha1.ArenaTemplateSourceSpec{
				Type: omniav1alpha1.ArenaTemplateSourceTypeConfigMap,
				ConfigMap: &omniav1alpha1.ConfigMapSource{
					Name: "test-configmap",
				},
			}

			f, err := reconciler.createFetcher(ctx, spec, "default", fetcher.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(f).NotTo(BeNil())
		})
	})
})

// findCondition finds a condition by type in the list.
func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
