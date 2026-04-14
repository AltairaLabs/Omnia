/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// skillSourceEnvtestCounter gives each spec a unique resource suffix so they
// don't collide inside the single envtest API server.
var skillSourceEnvtestCounter uint64

var _ = Describe("SkillSource Controller (envtest)", func() {
	var (
		ctx       context.Context
		namespace string
		workDir   string
		nextName  = func(prefix string) string {
			n := atomic.AddUint64(&skillSourceEnvtestCounter, 1)
			return fmt.Sprintf("%s-%d", prefix, n)
		}
	)

	BeforeEach(func() {
		ctx = context.Background()
		workDir = GinkgoT().TempDir()

		// Each spec gets a fresh namespace — SkillSource is namespace-scoped
		// and the reconciler writes under WorkspaceContentPath/<ns>/...
		namespace = nextName("ss-test")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		})).To(Succeed())
	})

	AfterEach(func() {
		ns := &corev1.Namespace{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, ns); err == nil {
			_ = k8sClient.Delete(ctx, ns)
		}
	})

	Context("CEL validation (API server enforcement)", func() {
		It("rejects type=git without spec.git", func() {
			src := &corev1alpha1.SkillSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nextName("ss"),
					Namespace: namespace,
				},
				Spec: corev1alpha1.SkillSourceSpec{
					Type:     corev1alpha1.SkillSourceTypeGit,
					Interval: "1h",
				},
			}
			err := k8sClient.Create(ctx, src)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("git source requires spec.git"))
		})

		It("rejects type=oci without spec.oci", func() {
			src := &corev1alpha1.SkillSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nextName("ss"),
					Namespace: namespace,
				},
				Spec: corev1alpha1.SkillSourceSpec{
					Type:     corev1alpha1.SkillSourceTypeOCI,
					Interval: "1h",
				},
			}
			err := k8sClient.Create(ctx, src)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("oci source requires spec.oci"))
		})

		It("rejects type=configmap without spec.configMap", func() {
			src := &corev1alpha1.SkillSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nextName("ss"),
					Namespace: namespace,
				},
				Spec: corev1alpha1.SkillSourceSpec{
					Type:     corev1alpha1.SkillSourceTypeConfigMap,
					Interval: "1h",
				},
			}
			err := k8sClient.Create(ctx, src)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("configmap source requires spec.configMap"))
		})

		It("rejects an invalid interval format", func() {
			src := &corev1alpha1.SkillSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nextName("ss"),
					Namespace: namespace,
				},
				Spec: corev1alpha1.SkillSourceSpec{
					Type: corev1alpha1.SkillSourceTypeConfigMap,
					ConfigMap: &corev1alpha1.ConfigMapSource{
						Name: "does-not-matter",
					},
					Interval: "not-a-duration",
				},
			}
			err := k8sClient.Create(ctx, src)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue(),
				"expected 400 Invalid from API server, got: %v", err)
		})

		It("accepts a well-formed configmap source", func() {
			src := &corev1alpha1.SkillSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nextName("ss"),
					Namespace: namespace,
				},
				Spec: corev1alpha1.SkillSourceSpec{
					Type: corev1alpha1.SkillSourceTypeConfigMap,
					ConfigMap: &corev1alpha1.ConfigMapSource{
						Name: "example",
					},
					Interval: "1h",
				},
			}
			Expect(k8sClient.Create(ctx, src)).To(Succeed())
		})
	})

	Context("Reconcile against real API server", func() {
		It("reaches Ready and exposes the right status fields for a configmap source with one SKILL.md", func() {
			cmName := nextName("skills-cm")
			srcName := nextName("ss")

			// ConfigMap-sourced skills: the sync layer decodes "__" back to "/"
			// so "myskill__SKILL.md" becomes file "myskill/SKILL.md".
			Expect(k8sClient.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace},
				Data: map[string]string{
					"myskill__SKILL.md": "---\nname: my-skill\ndescription: demo\n---\n\nbody\n",
				},
			})).To(Succeed())

			src := &corev1alpha1.SkillSource{
				ObjectMeta: metav1.ObjectMeta{Name: srcName, Namespace: namespace},
				Spec: corev1alpha1.SkillSourceSpec{
					Type: corev1alpha1.SkillSourceTypeConfigMap,
					ConfigMap: &corev1alpha1.ConfigMapSource{
						Name: cmName,
					},
					Interval:   "1h",
					Timeout:    "30s",
					TargetPath: "skills/test",
				},
			}
			Expect(k8sClient.Create(ctx, src)).To(Succeed())

			reconciler := &SkillSourceReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				WorkspaceContentPath: workDir,
				MaxVersionsPerSource: 3,
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: srcName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			var updated corev1alpha1.SkillSource
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: srcName, Namespace: namespace,
			}, &updated)).To(Succeed())

			Expect(updated.Status.Phase).To(Equal(corev1alpha1.SkillSourcePhaseReady))
			Expect(updated.Status.SkillCount).To(Equal(int32(1)))
			Expect(updated.Status.Artifact).NotTo(BeNil())
			Expect(updated.Status.Artifact.ContentPath).NotTo(BeEmpty())
			Expect(updated.Status.ObservedGeneration).To(Equal(updated.Generation))

			// Both condition types present and True
			expectSkillSourceCondition(&updated, SkillSourceConditionSourceAvailable, metav1.ConditionTrue)
			expectSkillSourceCondition(&updated, SkillSourceConditionContentValid, metav1.ConditionTrue)

			// Reconciler should have produced the synced content under WorkspaceContentPath
			synced := filepath.Join(workDir, namespace, namespace,
				updated.Status.Artifact.ContentPath, "myskill", "SKILL.md")
			_, statErr := os.Stat(synced)
			Expect(statErr).NotTo(HaveOccurred(),
				"expected synced SKILL.md at %s", synced)
		})

		It("re-reconciles after spec generation change and updates observedGeneration", func() {
			cmName := nextName("skills-cm")
			srcName := nextName("ss")

			Expect(k8sClient.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace},
				Data: map[string]string{
					"a__SKILL.md": "---\nname: a\ndescription: d\n---\n",
				},
			})).To(Succeed())

			src := &corev1alpha1.SkillSource{
				ObjectMeta: metav1.ObjectMeta{Name: srcName, Namespace: namespace},
				Spec: corev1alpha1.SkillSourceSpec{
					Type:      corev1alpha1.SkillSourceTypeConfigMap,
					ConfigMap: &corev1alpha1.ConfigMapSource{Name: cmName},
					Interval:  "1h",
				},
			}
			Expect(k8sClient.Create(ctx, src)).To(Succeed())

			reconciler := &SkillSourceReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				WorkspaceContentPath: workDir,
			}
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{Name: srcName, Namespace: namespace},
			}
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			var first corev1alpha1.SkillSource
			Expect(k8sClient.Get(ctx, req.NamespacedName, &first)).To(Succeed())
			gen1 := first.Generation

			// Mutate spec — flip targetPath. Generation must advance and
			// observedGeneration must catch up on the next reconcile.
			first.Spec.TargetPath = "skills/changed"
			Expect(k8sClient.Update(ctx, &first)).To(Succeed())

			var afterUpdate corev1alpha1.SkillSource
			Expect(k8sClient.Get(ctx, req.NamespacedName, &afterUpdate)).To(Succeed())
			Expect(afterUpdate.Generation).To(BeNumerically(">", gen1),
				"spec change should advance generation")

			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			var final corev1alpha1.SkillSource
			Expect(k8sClient.Get(ctx, req.NamespacedName, &final)).To(Succeed())
			Expect(final.Status.ObservedGeneration).To(Equal(final.Generation))
		})

		It("reports Error phase when the referenced ConfigMap does not exist", func() {
			srcName := nextName("ss")
			src := &corev1alpha1.SkillSource{
				ObjectMeta: metav1.ObjectMeta{Name: srcName, Namespace: namespace},
				Spec: corev1alpha1.SkillSourceSpec{
					Type:      corev1alpha1.SkillSourceTypeConfigMap,
					ConfigMap: &corev1alpha1.ConfigMapSource{Name: "nope"},
					Interval:  "1h",
				},
			}
			Expect(k8sClient.Create(ctx, src)).To(Succeed())

			reconciler := &SkillSourceReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				WorkspaceContentPath: workDir,
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: srcName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			var updated corev1alpha1.SkillSource
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: srcName, Namespace: namespace,
			}, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(corev1alpha1.SkillSourcePhaseError))
			expectSkillSourceCondition(&updated, SkillSourceConditionSourceAvailable, metav1.ConditionFalse)
		})

		It("is a no-op when spec.suspend is true", func() {
			srcName := nextName("ss")
			src := &corev1alpha1.SkillSource{
				ObjectMeta: metav1.ObjectMeta{Name: srcName, Namespace: namespace},
				Spec: corev1alpha1.SkillSourceSpec{
					Type:      corev1alpha1.SkillSourceTypeConfigMap,
					ConfigMap: &corev1alpha1.ConfigMapSource{Name: "nope"},
					Interval:  "1h",
					Suspend:   true,
				},
			}
			Expect(k8sClient.Create(ctx, src)).To(Succeed())

			reconciler := &SkillSourceReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				WorkspaceContentPath: workDir,
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: srcName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			var updated corev1alpha1.SkillSource
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: srcName, Namespace: namespace,
			}, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(BeEmpty(),
				"suspended source should not mutate status")
		})
	})
})

func expectSkillSourceCondition(src *corev1alpha1.SkillSource, condType string, want metav1.ConditionStatus) {
	GinkgoHelper()
	for _, c := range src.Status.Conditions {
		if c.Type == condType {
			Expect(c.Status).To(Equal(want),
				"condition %q status mismatch (reason=%s message=%s)",
				condType, c.Reason, c.Message)
			return
		}
	}
	Fail(fmt.Sprintf("condition %q not present", condType))
}
