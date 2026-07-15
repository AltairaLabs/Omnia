/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"os"
	"path/filepath"
	"time"

	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/license"
	"github.com/altairalabs/omnia/internal/sourcesync"
)

// fakeFetcher is an injected Fetcher that writes a pack.json to a fresh temp
// dir on every Fetch. A fresh dir each call keeps the fake robust against the
// reconciler's defer os.RemoveAll(artifact.Path) cleanup.
type fakeFetcher struct {
	rev  string
	pack string // pack.json contents
}

func (f *fakeFetcher) LatestRevision(context.Context) (string, error) { return f.rev, nil }

func (f *fakeFetcher) Fetch(_ context.Context, _ string) (*sourcesync.Artifact, error) {
	dir, err := os.MkdirTemp("", "pps-fake-*")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, packJSONKey), []byte(f.pack), 0o600); err != nil {
		return nil, err
	}
	return &sourcesync.Artifact{Path: dir, Revision: f.rev}, nil
}

func (f *fakeFetcher) Type() string { return "git" }

// pinnedLicenseClaims mirrors the private wire contract of the license
// package's JWT claims (lid/tier/customer/features/limits) so this package —
// which cannot import the unexported licenseClaims type — can mint tokens the
// real Validator accepts.
type pinnedLicenseClaims struct {
	jwt.RegisteredClaims
	LicenseID string           `json:"lid"`
	Tier      string           `json:"tier"`
	Customer  string           `json:"customer"`
	Features  license.Features `json:"features"`
	Limits    license.Limits   `json:"limits"`
}

// newEnterpriseTokenDenyingGit signs an enterprise-tier license (no GitSource
// feature) with a freshly generated RSA key pair, returning the token and the
// matching public key so a Validator can be built to verify it.
func newEnterpriseTokenDenyingGit() (string, *rsa.PublicKey) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	claims := &pinnedLicenseClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		LicenseID: "test-no-git-source",
		Tier:      "enterprise",
		Customer:  "Test Corp",
		Features:  license.Features{}, // GitSource left false — denies git sources
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenString, err := token.SignedString(privateKey)
	Expect(err).NotTo(HaveOccurred())

	return tokenString, &privateKey.PublicKey
}

var _ = Describe("PromptPackSource Controller", func() {
	ctx := context.Background()
	const ns = "default"

	// newReconciler builds a reconciler whose fetcher is the supplied fake.
	newReconciler := func(f sourcesync.Fetcher) *PromptPackSourceReconciler {
		return &PromptPackSourceReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Recorder: record.NewFakeRecorder(10),
			FetcherFor: func(context.Context, *omniav1alpha1.PromptPackSource) (sourcesync.Fetcher, error) {
				return f, nil
			},
		}
	}

	newSource := func(name, packName string, suspend bool) *omniav1alpha1.PromptPackSource {
		return &omniav1alpha1.PromptPackSource{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: omniav1alpha1.PromptPackSourceSpec{
				Type:     omniav1alpha1.PromptPackSourceTypeGit,
				PackName: packName,
				Interval: "5m",
				Suspend:  suspend,
				Git: &corev1alpha1.GitSource{
					URL: "https://github.com/example/repo.git",
				},
			},
		}
	}

	reconcileOnce := func(r *PromptPackSourceReconciler, name string) (reconcile.Result, error) {
		return r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
	}

	Context("When materializing a new version", func() {
		const (
			name     = "pps-materialize"
			packName = "mypack"
		)

		AfterEach(func() {
			src := &omniav1alpha1.PromptPackSource{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, src); err == nil {
				Expect(k8sClient.Delete(ctx, src)).To(Succeed())
			}
		})

		It("creates the PromptPack version-object + backing ConfigMap", func() {
			Expect(k8sClient.Create(ctx, newSource(name, packName, false))).To(Succeed())

			r := newReconciler(&fakeFetcher{rev: "rev1", pack: `{"name":"mypack","version":"1.0.0"}`})
			_, err := reconcileOnce(r, name)
			Expect(err).NotTo(HaveOccurred())

			objName := corev1alpha1.PromptPackObjectName(packName, "1.0.0")

			By("checking the PromptPack version-object")
			pp := &corev1alpha1.PromptPack{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: objName, Namespace: ns}, pp)).To(Succeed())
			Expect(pp.Labels["omnia.altairalabs.ai/promptpack"]).To(Equal(packName))
			Expect(pp.Spec.PackName).To(Equal(packName))
			Expect(pp.Spec.Version).To(Equal("1.0.0"))
			Expect(string(pp.Spec.Source.Type)).To(Equal("configmap"))
			Expect(pp.Spec.Source.ConfigMapRef).NotTo(BeNil())
			Expect(pp.Spec.Source.ConfigMapRef.Name).To(Equal(objName + "-content"))

			By("checking the backing ConfigMap")
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: objName + "-content", Namespace: ns}, cm)).To(Succeed())
			Expect(cm.Data[packJSONKey]).To(Equal(`{"name":"mypack","version":"1.0.0"}`))
			Expect(cm.Labels["omnia.altairalabs.ai/managed-by"]).To(Equal("promptpack"))
			Expect(cm.Labels["omnia.altairalabs.ai/promptpack"]).To(Equal(packName))

			By("checking source status")
			updated := &omniav1alpha1.PromptPackSource{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.PromptPackSourcePhaseReady))
			Expect(updated.Status.LastSyncedVersion).To(Equal("1.0.0"))
		})
	})

	Context("When re-polling with no change (idempotent)", func() {
		const (
			name     = "pps-idempotent"
			packName = "idempack"
		)

		AfterEach(func() {
			src := &omniav1alpha1.PromptPackSource{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, src); err == nil {
				Expect(k8sClient.Delete(ctx, src)).To(Succeed())
			}
		})

		It("does not error and does not duplicate the PromptPack", func() {
			Expect(k8sClient.Create(ctx, newSource(name, packName, false))).To(Succeed())

			r := newReconciler(&fakeFetcher{rev: "rev1", pack: `{"version":"1.0.0"}`})
			_, err := reconcileOnce(r, name)
			Expect(err).NotTo(HaveOccurred())

			By("reconciling again")
			_, err = reconcileOnce(r, name)
			Expect(err).NotTo(HaveOccurred())

			objName := corev1alpha1.PromptPackObjectName(packName, "1.0.0")
			pp := &corev1alpha1.PromptPack{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: objName, Namespace: ns}, pp)).To(Succeed())

			By("verifying only one version-object for this pack exists")
			var packs corev1alpha1.PromptPackList
			Expect(k8sClient.List(ctx, &packs,
				client.InNamespace(ns),
				client.MatchingLabels{"omnia.altairalabs.ai/promptpack": packName})).To(Succeed())
			Expect(packs.Items).To(HaveLen(1))
		})
	})

	Context("When HEAD advances to a new version", func() {
		const (
			name     = "pps-advance"
			packName = "advpack"
		)

		AfterEach(func() {
			src := &omniav1alpha1.PromptPackSource{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, src); err == nil {
				Expect(k8sClient.Delete(ctx, src)).To(Succeed())
			}
		})

		It("materializes the new version and keeps the old one", func() {
			Expect(k8sClient.Create(ctx, newSource(name, packName, false))).To(Succeed())

			r1 := newReconciler(&fakeFetcher{rev: "rev1", pack: `{"version":"1.0.0"}`})
			_, err := reconcileOnce(r1, name)
			Expect(err).NotTo(HaveOccurred())

			By("advancing the fake to 1.1.0")
			r2 := newReconciler(&fakeFetcher{rev: "rev2", pack: `{"version":"1.1.0"}`})
			_, err = reconcileOnce(r2, name)
			Expect(err).NotTo(HaveOccurred())

			old := corev1alpha1.PromptPackObjectName(packName, "1.0.0")
			neu := corev1alpha1.PromptPackObjectName(packName, "1.1.0")
			pp := &corev1alpha1.PromptPack{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: old, Namespace: ns}, pp)).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: neu, Namespace: ns}, pp)).To(Succeed())
		})
	})

	Context("When the source is suspended", func() {
		const (
			name     = "pps-suspended"
			packName = "suspendpack"
		)

		AfterEach(func() {
			src := &omniav1alpha1.PromptPackSource{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, src); err == nil {
				Expect(k8sClient.Delete(ctx, src)).To(Succeed())
			}
		})

		It("is a no-op — no fetch, no objects", func() {
			Expect(k8sClient.Create(ctx, newSource(name, packName, true))).To(Succeed())

			// FetcherFor panics if invoked — proves suspend short-circuits before fetch.
			r := &PromptPackSourceReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
				FetcherFor: func(context.Context, *omniav1alpha1.PromptPackSource) (sourcesync.Fetcher, error) {
					Fail("fetcher must not be built for a suspended source")
					return nil, nil
				},
			}
			res, err := reconcileOnce(r, name)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.RequeueAfter).To(BeZero())

			objName := corev1alpha1.PromptPackObjectName(packName, "1.0.0")
			pp := &corev1alpha1.PromptPack{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: objName, Namespace: ns}, pp)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})

	Context("When the fetched pack.json has no version", func() {
		const (
			name     = "pps-degraded"
			packName = "degpack"
		)

		AfterEach(func() {
			src := &omniav1alpha1.PromptPackSource{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, src); err == nil {
				Expect(k8sClient.Delete(ctx, src)).To(Succeed())
			}
		})

		It("sets Error phase + Ready=False and creates no PromptPack", func() {
			Expect(k8sClient.Create(ctx, newSource(name, packName, false))).To(Succeed())

			r := newReconciler(&fakeFetcher{rev: "rev1", pack: `{"name":"x"}`})
			res, err := reconcileOnce(r, name)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.RequeueAfter).NotTo(BeZero(), "degraded should requeue after interval, not hot-loop")

			updated := &omniav1alpha1.PromptPackSource{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.PromptPackSourcePhaseError))
			cond := meta.FindStatusCondition(updated.Status.Conditions, omniav1alpha1.PromptPackSourceConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))

			By("verifying no PromptPack was created for this pack")
			var packs corev1alpha1.PromptPackList
			Expect(k8sClient.List(ctx, &packs,
				client.InNamespace(ns),
				client.MatchingLabels{"omnia.altairalabs.ai/promptpack": packName})).To(Succeed())
			Expect(packs.Items).To(BeEmpty())
		})
	})

	Context("When the license denies the source type", func() {
		const (
			name     = "pps-license-denied"
			packName = "deniedpack"
		)

		AfterEach(func() {
			src := &omniav1alpha1.PromptPackSource{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, src); err == nil {
				Expect(k8sClient.Delete(ctx, src)).To(Succeed())
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: license.LicenseSecretName, Namespace: ns}, secret); err == nil {
				Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
			}
		})

		It("sets Error phase + Ready=False with LicenseViolation, does not requeue, and creates no PromptPack", func() {
			tokenString, publicKey := newEnterpriseTokenDenyingGit()

			Expect(k8sClient.Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: license.LicenseSecretName, Namespace: ns},
				Data:       map[string][]byte{license.LicenseSecretKey: []byte(tokenString)},
			})).To(Succeed())

			validator, err := license.NewValidator(k8sClient, license.WithPublicKey(publicKey), license.WithNamespace(ns))
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Create(ctx, newSource(name, packName, false))).To(Succeed())

			// FetcherFor panics if invoked — proves the license check short-circuits before fetch.
			r := &PromptPackSourceReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				Recorder:         record.NewFakeRecorder(10),
				LicenseValidator: validator,
				FetcherFor: func(context.Context, *omniav1alpha1.PromptPackSource) (sourcesync.Fetcher, error) {
					Fail("fetcher must not be built when the license denies the source type")
					return nil, nil
				},
			}
			res, err := reconcileOnce(r, name)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.RequeueAfter).To(BeZero(), "license violations must not requeue — a license change, not time, resolves it")

			updated := &omniav1alpha1.PromptPackSource{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.PromptPackSourcePhaseError))
			cond := meta.FindStatusCondition(updated.Status.Conditions, omniav1alpha1.PromptPackSourceConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(reasonLicenseViolation))

			By("verifying no PromptPack was created for this pack")
			var packs corev1alpha1.PromptPackList
			Expect(k8sClient.List(ctx, &packs,
				client.InNamespace(ns),
				client.MatchingLabels{"omnia.altairalabs.ai/promptpack": packName})).To(Succeed())
			Expect(packs.Items).To(BeEmpty())
		})
	})
})
