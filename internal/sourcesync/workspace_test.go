/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package sourcesync

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const wsTestFile = "f.txt"

var _ = Describe("WorkspaceFetcher", func() {
	var (
		ctx    context.Context
		srcDir string
	)

	BeforeEach(func() {
		ctx = context.Background()
		srcDir = GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(srcDir, "pack.json"), []byte(`{"name":"p"}`), 0600)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(srcDir, "scenarios"), 0755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(srcDir, "scenarios", "a.yaml"), []byte("x: 1"), 0600)).To(Succeed())
	})

	It("returns an in-place artifact marked Preserve without moving the source", func() {
		f := NewWorkspaceFetcher(srcDir)
		Expect(f.Type()).To(Equal("workspace"))

		artifact, err := f.Fetch(ctx, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(artifact.Path).To(Equal(srcDir))
		Expect(artifact.Preserve).To(BeTrue())
		Expect(artifact.Checksum).NotTo(BeEmpty())
		Expect(artifact.Revision).To(Equal(artifact.Checksum))

		// Source dir is untouched.
		_, statErr := os.Stat(filepath.Join(srcDir, "pack.json"))
		Expect(statErr).NotTo(HaveOccurred())
	})

	It("changes its revision when content changes", func() {
		f := NewWorkspaceFetcher(srcDir)
		rev1, err := f.LatestRevision(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(os.WriteFile(filepath.Join(srcDir, "pack.json"), []byte(`{"name":"changed"}`), 0600)).To(Succeed())
		rev2, err := f.LatestRevision(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(rev2).NotTo(Equal(rev1))
	})

	It("errors when the source path is missing", func() {
		f := NewWorkspaceFetcher(filepath.Join(srcDir, "does-not-exist"))
		_, err := f.Fetch(ctx, "")
		Expect(err).To(HaveOccurred())
	})

	It("errors when the source path is a file, not a directory", func() {
		f := NewWorkspaceFetcher(filepath.Join(srcDir, "pack.json"))
		_, err := f.LatestRevision(ctx)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("storeVersion preserve mode", func() {
	It("copies the source into the version dir and leaves the source in place", func() {
		src := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(src, wsTestFile), []byte("hello"), 0600)).To(Succeed())
		versionDir := filepath.Join(GinkgoT().TempDir(), "versions", "v1")

		Expect(storeVersion(src, versionDir, true)).To(Succeed())

		// Version dir has the content...
		copied, err := os.ReadFile(filepath.Join(versionDir, wsTestFile))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(copied)).To(Equal("hello"))

		// ...and the source is preserved (not moved away).
		orig, err := os.ReadFile(filepath.Join(src, wsTestFile))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(orig)).To(Equal("hello"))
	})

	It("lands content in the version dir when preserve is false (move mode)", func() {
		src := GinkgoT().TempDir()
		inner := filepath.Join(src, "content")
		Expect(os.MkdirAll(inner, 0755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(inner, wsTestFile), []byte("bye"), 0600)).To(Succeed())
		versionDir := filepath.Join(GinkgoT().TempDir(), "versions", "v1")

		// Move mode (the throwaway-temp-dir path) — the controller is
		// responsible for cleaning up the source afterward, so we only assert
		// the content is versioned, not whether the source survived.
		Expect(storeVersion(inner, versionDir, false)).To(Succeed())

		copied, err := os.ReadFile(filepath.Join(versionDir, wsTestFile))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(copied)).To(Equal("bye"))
	})
})
