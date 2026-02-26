/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/altairalabs/omnia/ee/pkg/arena/fetcher"
)

var _ = Describe("FilesystemSyncer", func() {
	var (
		ctx     context.Context
		tmpDir  string
		syncer  *FilesystemSyncer
		srcDir  string
		srcFile string
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tmpDir, err = os.MkdirTemp("", "fssync-test-*")
		Expect(err).NotTo(HaveOccurred())

		syncer = &FilesystemSyncer{
			WorkspaceContentPath: tmpDir,
			MaxVersionsPerSource: 3,
		}

		// Create artifact source directory with a file
		srcDir, err = os.MkdirTemp("", "fssync-src-*")
		Expect(err).NotTo(HaveOccurred())
		srcFile = filepath.Join(srcDir, "test.txt")
		Expect(os.WriteFile(srcFile, []byte("hello world"), 0644)).To(Succeed())
	})

	AfterEach(func() {
		_ = os.RemoveAll(tmpDir)
		_ = os.RemoveAll(srcDir)
	})

	Describe("SyncToFilesystem", func() {
		It("should sync a new artifact to the filesystem", func() {
			artifact := &fetcher.Artifact{
				Path:     srcDir,
				Checksum: "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
				Revision: "v1.0.0",
			}

			contentPath, version, err := syncer.SyncToFilesystem(ctx, SyncParams{
				WorkspaceName: "ws1",
				Namespace:     "ns1",
				TargetPath:    "arena/my-source",
				Artifact:      artifact,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(version).To(Equal("abcdef123456"))
			Expect(contentPath).To(Equal("arena/my-source/.arena/versions/abcdef123456"))

			// Verify HEAD file
			headContent, err := os.ReadFile(filepath.Join(tmpDir, "ws1", "ns1", "arena/my-source", ".arena", "HEAD"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(headContent)).To(Equal("abcdef123456"))
		})

		It("should skip sync when version already exists", func() {
			artifact := &fetcher.Artifact{
				Path:     srcDir,
				Checksum: "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
				Revision: "v1.0.0",
			}

			// First sync
			contentPath1, version1, err := syncer.SyncToFilesystem(ctx, SyncParams{
				WorkspaceName: "ws1",
				Namespace:     "ns1",
				TargetPath:    "arena/my-source",
				Artifact:      artifact,
			})
			Expect(err).NotTo(HaveOccurred())

			// Create new source dir for second sync (original was moved)
			srcDir2, err := os.MkdirTemp("", "fssync-src2-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(srcDir2) }()
			Expect(os.WriteFile(filepath.Join(srcDir2, "test.txt"), []byte("hello world"), 0644)).To(Succeed())

			artifact2 := &fetcher.Artifact{
				Path:     srcDir2,
				Checksum: "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
				Revision: "v1.0.0",
			}

			// Second sync with same checksum
			contentPath2, version2, err := syncer.SyncToFilesystem(ctx, SyncParams{
				WorkspaceName: "ws1",
				Namespace:     "ns1",
				TargetPath:    "arena/my-source",
				Artifact:      artifact2,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(contentPath2).To(Equal(contentPath1))
			Expect(version2).To(Equal(version1))
		})

		It("should handle nil StorageManager", func() {
			syncer.StorageManager = nil
			artifact := &fetcher.Artifact{
				Path:     srcDir,
				Checksum: "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
				Revision: "v1.0.0",
			}

			_, _, err := syncer.SyncToFilesystem(ctx, SyncParams{
				WorkspaceName: "ws1",
				Namespace:     "ns1",
				TargetPath:    "arena/my-source",
				Artifact:      artifact,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("UpdateHEAD", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "head-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		_ = os.RemoveAll(tmpDir)
	})

	It("should create HEAD file with version content", func() {
		err := UpdateHEAD(tmpDir, "v1.0.0")
		Expect(err).NotTo(HaveOccurred())

		content, err := os.ReadFile(filepath.Join(tmpDir, ".arena", "HEAD"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(Equal("v1.0.0"))
	})

	It("should overwrite existing HEAD file", func() {
		Expect(UpdateHEAD(tmpDir, "v1.0.0")).To(Succeed())
		Expect(UpdateHEAD(tmpDir, "v2.0.0")).To(Succeed())

		content, err := os.ReadFile(filepath.Join(tmpDir, ".arena", "HEAD"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(Equal("v2.0.0"))
	})

	It("should create .arena directory if it does not exist", func() {
		err := UpdateHEAD(tmpDir, "v1.0.0")
		Expect(err).NotTo(HaveOccurred())

		info, err := os.Stat(filepath.Join(tmpDir, ".arena"))
		Expect(err).NotTo(HaveOccurred())
		Expect(info.IsDir()).To(BeTrue())
	})
})

var _ = Describe("GCOldVersions", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "gc-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		_ = os.RemoveAll(tmpDir)
	})

	It("should not error when versions directory does not exist", func() {
		err := GCOldVersions(tmpDir, 3)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should not remove versions when under the limit", func() {
		versionsDir := filepath.Join(tmpDir, ".arena", "versions")
		Expect(os.MkdirAll(versionsDir, 0755)).To(Succeed())

		for i := 0; i < 3; i++ {
			Expect(os.MkdirAll(filepath.Join(versionsDir, "v"+string(rune('a'+i))), 0755)).To(Succeed())
		}

		err := GCOldVersions(tmpDir, 3)
		Expect(err).NotTo(HaveOccurred())

		entries, err := os.ReadDir(versionsDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(3))
	})

	It("should remove oldest versions when over the limit", func() {
		versionsDir := filepath.Join(tmpDir, ".arena", "versions")
		Expect(os.MkdirAll(versionsDir, 0755)).To(Succeed())

		// Create 5 version dirs with staggered mod times
		versions := []string{"oldest", "old", "mid", "new", "newest"}
		for i, name := range versions {
			dir := filepath.Join(versionsDir, name)
			Expect(os.MkdirAll(dir, 0755)).To(Succeed())
			// Set modification times with 1-second intervals
			modTime := time.Now().Add(time.Duration(i-5) * time.Second)
			Expect(os.Chtimes(dir, modTime, modTime)).To(Succeed())
		}

		err := GCOldVersions(tmpDir, 3)
		Expect(err).NotTo(HaveOccurred())

		entries, err := os.ReadDir(versionsDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(3))

		// Verify oldest two were removed
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		Expect(names).NotTo(ContainElement("oldest"))
		Expect(names).NotTo(ContainElement("old"))
		Expect(names).To(ContainElement("mid"))
		Expect(names).To(ContainElement("new"))
		Expect(names).To(ContainElement("newest"))
	})

	It("should default to 10 when maxVersions is 0", func() {
		versionsDir := filepath.Join(tmpDir, ".arena", "versions")
		Expect(os.MkdirAll(versionsDir, 0755)).To(Succeed())

		// Create 11 versions
		for i := 0; i < 11; i++ {
			dir := filepath.Join(versionsDir, "v"+string(rune('a'+i)))
			Expect(os.MkdirAll(dir, 0755)).To(Succeed())
			modTime := time.Now().Add(time.Duration(i-11) * time.Second)
			Expect(os.Chtimes(dir, modTime, modTime)).To(Succeed())
		}

		err := GCOldVersions(tmpDir, 0) // Should default to 10
		Expect(err).NotTo(HaveOccurred())

		entries, err := os.ReadDir(versionsDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(10))
	})

	It("should skip non-directory entries", func() {
		versionsDir := filepath.Join(tmpDir, ".arena", "versions")
		Expect(os.MkdirAll(versionsDir, 0755)).To(Succeed())

		// Create directories and a file
		for i := 0; i < 4; i++ {
			dir := filepath.Join(versionsDir, "v"+string(rune('a'+i)))
			Expect(os.MkdirAll(dir, 0755)).To(Succeed())
			modTime := time.Now().Add(time.Duration(i-4) * time.Second)
			Expect(os.Chtimes(dir, modTime, modTime)).To(Succeed())
		}
		Expect(os.WriteFile(filepath.Join(versionsDir, "not-a-dir.txt"), []byte("data"), 0644)).To(Succeed())

		err := GCOldVersions(tmpDir, 3)
		Expect(err).NotTo(HaveOccurred())

		entries, err := os.ReadDir(versionsDir)
		Expect(err).NotTo(HaveOccurred())
		// 3 dirs remaining + 1 file = 4 total entries
		Expect(entries).To(HaveLen(4))
	})
})

var _ = Describe("copyDirectory", func() {
	var srcDir string
	var dstDir string

	BeforeEach(func() {
		var err error
		srcDir, err = os.MkdirTemp("", "copy-src-*")
		Expect(err).NotTo(HaveOccurred())
		dstDir, err = os.MkdirTemp("", "copy-dst-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		_ = os.RemoveAll(srcDir)
		_ = os.RemoveAll(dstDir)
	})

	It("should copy files and subdirectories", func() {
		// Create source structure
		Expect(os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("content2"), 0644)).To(Succeed())

		err := copyDirectory(srcDir, dstDir)
		Expect(err).NotTo(HaveOccurred())

		// Verify copied files
		content1, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content1)).To(Equal("content1"))

		content2, err := os.ReadFile(filepath.Join(dstDir, "subdir", "file2.txt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content2)).To(Equal("content2"))
	})

	It("should return error for non-existent source", func() {
		err := copyDirectory("/nonexistent/path", dstDir)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("calculateVersion", func() {
	It("should extract version from sha256 checksum", func() {
		artifact := &fetcher.Artifact{
			Checksum: "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		}

		version, err := calculateVersion(artifact)
		Expect(err).NotTo(HaveOccurred())
		Expect(version).To(Equal("abcdef123456"))
	})

	It("should calculate hash when checksum is empty", func() {
		tmpDir, err := os.MkdirTemp("", "calc-version-*")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(tmpDir) }()

		Expect(os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("test data"), 0644)).To(Succeed())

		artifact := &fetcher.Artifact{
			Path:     tmpDir,
			Checksum: "",
		}

		version, err := calculateVersion(artifact)
		Expect(err).NotTo(HaveOccurred())
		Expect(version).To(HaveLen(12))
	})

	It("should calculate hash when checksum has wrong prefix", func() {
		tmpDir, err := os.MkdirTemp("", "calc-version-*")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(tmpDir) }()

		Expect(os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("test data"), 0644)).To(Succeed())

		artifact := &fetcher.Artifact{
			Path:     tmpDir,
			Checksum: "md5:abcdef",
		}

		version, err := calculateVersion(artifact)
		Expect(err).NotTo(HaveOccurred())
		Expect(version).To(HaveLen(12))
	})

	It("should calculate hash when checksum hash part is too short", func() {
		tmpDir, err := os.MkdirTemp("", "calc-version-*")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(tmpDir) }()

		Expect(os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("test data"), 0644)).To(Succeed())

		artifact := &fetcher.Artifact{
			Path:     tmpDir,
			Checksum: "sha256:short",
		}

		version, err := calculateVersion(artifact)
		Expect(err).NotTo(HaveOccurred())
		Expect(version).To(HaveLen(12))
	})
})

var _ = Describe("storeVersion", func() {
	It("should move artifact to version directory", func() {
		srcDir, err := os.MkdirTemp("", "store-src-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(filepath.Join(srcDir, "test.txt"), []byte("data"), 0644)).To(Succeed())

		dstDir := filepath.Join(os.TempDir(), "store-dst-test-"+time.Now().Format("20060102150405"))
		defer func() { _ = os.RemoveAll(dstDir) }()

		err = storeVersion(srcDir, dstDir)
		Expect(err).NotTo(HaveOccurred())

		// Verify content was stored
		entries, err := os.ReadDir(dstDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).NotTo(BeEmpty())
	})
})

var _ = Describe("collectVersionInfos", func() {
	It("should collect directory entries only", func() {
		tmpDir, err := os.MkdirTemp("", "collect-test-*")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(tmpDir) }()

		Expect(os.MkdirAll(filepath.Join(tmpDir, "dir1"), 0755)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(tmpDir, "dir2"), 0755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("data"), 0644)).To(Succeed())

		entries, err := os.ReadDir(tmpDir)
		Expect(err).NotTo(HaveOccurred())

		versions := collectVersionInfos(entries)
		Expect(versions).To(HaveLen(2))
		names := []string{versions[0].name, versions[1].name}
		Expect(names).To(ContainElement("dir1"))
		Expect(names).To(ContainElement("dir2"))
	})
})
