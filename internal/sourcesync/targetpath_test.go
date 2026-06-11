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

// Distinct fixture values (not shared with syncer_test.go) keep these literals
// under the goconst duplication threshold.
const (
	guardWS       = "guard-ws"
	guardNS       = "guard-ns"
	guardChecksum = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
)

var _ = Describe("ValidateTargetPath", func() {
	DescribeTable("accepts in-bounds relative paths",
		func(targetPath string) {
			Expect(ValidateTargetPath(targetPath)).To(Succeed())
		},
		Entry("empty (caller defaults it)", ""),
		Entry("simple subdir", "arena/my-source"),
		Entry("skills layout", "skills/anthropic"),
		Entry("dot stays at root", "."),
		Entry("in-bounds dot-dot collapses", "arena/sub/.."),
		Entry("nested", "a/b/c/d"),
	)

	DescribeTable("rejects paths that escape the workspace subtree",
		func(targetPath string) {
			Expect(ValidateTargetPath(targetPath)).NotTo(Succeed())
		},
		Entry("leading dot-dot", "../escape"),
		Entry("bare dot-dot", ".."),
		Entry("absolute", "/etc/passwd"),
		Entry("escape through a subdir", "a/../../b"),
		Entry("deep escape", "arena/../../../../etc"),
	)
})

var _ = Describe("SyncToFilesystem TargetPath guard", func() {
	var (
		ctx    context.Context
		tmpDir string
		srcDir string
		syncer *FilesystemSyncer
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tmpDir, err = os.MkdirTemp("", "fssync-guard-*")
		Expect(err).NotTo(HaveOccurred())
		srcDir, err = os.MkdirTemp("", "fssync-guard-src-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(filepath.Join(srcDir, "x.txt"), []byte("data"), 0644)).To(Succeed())
		syncer = &FilesystemSyncer{WorkspaceContentPath: tmpDir, MaxVersionsPerSource: 3}
	})

	AfterEach(func() {
		_ = os.RemoveAll(tmpDir)
		_ = os.RemoveAll(srcDir)
	})

	It("rejects a traversing TargetPath and writes nothing outside the subtree", func() {
		_, _, err := syncer.SyncToFilesystem(ctx, SyncParams{
			WorkspaceName: guardWS,
			Namespace:     guardNS,
			TargetPath:    "../../escape",
			Artifact: &Artifact{
				Path:     srcDir,
				Checksum: guardChecksum,
				Revision: "v1",
			},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("targetPath"))

		// Nothing should have been materialized under the workspace root.
		entries, readErr := os.ReadDir(filepath.Join(tmpDir, guardWS, guardNS))
		if readErr == nil {
			Expect(entries).To(BeEmpty())
		} else {
			Expect(os.IsNotExist(readErr)).To(BeTrue())
		}
	})
})
