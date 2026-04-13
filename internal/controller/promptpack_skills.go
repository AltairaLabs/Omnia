/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// SkillManifestEntry is one row in the PromptPack skill manifest written to
// the workspace PVC. The runtime reads the manifest at startup and calls
// sdk.WithSkillsDir(...) once per entry.
type SkillManifestEntry struct {
	// MountAs is the directory name the runtime exposes the skill under,
	// e.g. "billing/refund-processing".
	MountAs string `json:"mount_as"`
	// ContentPath is the path under the workspace content PVC where the
	// skill's directory (containing SKILL.md) lives.
	ContentPath string `json:"content_path"`
	// Name is the skill's frontmatter name (helpful in logs).
	Name string `json:"name"`
}

// SkillManifestConfig carries the PromptPack.spec.skillsConfig block to the
// runtime.
type SkillManifestConfig struct {
	MaxActive int32  `json:"max_active,omitempty"`
	Selector  string `json:"selector,omitempty"`
}

// SkillManifest is serialised as JSON at
//
//	<workspace-pvc>/manifests/<promptpack-name>.json
//
// The runtime container mounts the workspace PVC and reads this file to
// configure PromptKit.
type SkillManifest struct {
	Version string               `json:"version"`
	Skills  []SkillManifestEntry `json:"skills"`
	Config  *SkillManifestConfig `json:"config,omitempty"`
}

// PromptPackSkillResolution captures everything the PromptPack reconciler
// needs from a spec.skills resolution pass — manifest content, the union of
// declared allowed-tools (for cross-validation against the pack's tool set),
// and a categorised error list.
type PromptPackSkillResolution struct {
	Manifest            *SkillManifest
	AllowedToolsBySkill map[string][]string

	// LookupErrors are SkillSource lookup failures (missing CRD, no synced
	// artifact, etc.). Drive the SkillsResolved condition.
	LookupErrors []error
	// CollisionErrors are skill-name collisions across resolved entries.
	// Drive the SkillsValid condition.
	CollisionErrors []error
}

// ResolvePromptPackSkills walks PromptPack.spec.skills, looks up each
// referenced SkillSource in the pack's namespace, applies the per-ref
// Include filter, and returns a manifest plus categorised diagnostics.
func ResolvePromptPackSkills(
	ctx context.Context,
	c client.Reader,
	pack *corev1alpha1.PromptPack,
	workspaceContentRoot string,
) PromptPackSkillResolution {
	res := PromptPackSkillResolution{
		Manifest:            &SkillManifest{},
		AllowedToolsBySkill: map[string][]string{},
	}
	if len(pack.Spec.Skills) == 0 {
		res.Manifest.Version = hashManifest(res.Manifest)
		return res
	}

	workspaceName := GetWorkspaceForNamespace(ctx, c, pack.Namespace)
	seenNames := map[string]string{} // name -> source

	for _, ref := range pack.Spec.Skills {
		entries, allowed, collisions, lookupErr := resolveOneSkillRef(
			ctx, c, pack.Namespace, workspaceContentRoot, workspaceName, ref, seenNames)
		if lookupErr != nil {
			res.LookupErrors = append(res.LookupErrors, lookupErr)
			continue
		}
		res.Manifest.Skills = append(res.Manifest.Skills, entries...)
		for name, tools := range allowed {
			res.AllowedToolsBySkill[name] = tools
		}
		res.CollisionErrors = append(res.CollisionErrors, collisions...)
	}

	sort.Slice(res.Manifest.Skills, func(i, j int) bool {
		return res.Manifest.Skills[i].MountAs < res.Manifest.Skills[j].MountAs
	})
	if pack.Spec.SkillsConfig != nil {
		cfg := &SkillManifestConfig{Selector: string(pack.Spec.SkillsConfig.Selector)}
		if pack.Spec.SkillsConfig.MaxActive != nil {
			cfg.MaxActive = *pack.Spec.SkillsConfig.MaxActive
		}
		res.Manifest.Config = cfg
	}
	res.Manifest.Version = hashManifest(res.Manifest)
	return res
}

func resolveOneSkillRef(
	ctx context.Context,
	c client.Reader,
	namespace, workspaceContentRoot, workspaceName string,
	ref corev1alpha1.SkillRef,
	seenNames map[string]string,
) (entries []SkillManifestEntry, allowed map[string][]string, collisions []error, lookupErr error) {
	allowed = map[string][]string{}

	src := &corev1alpha1.SkillSource{}
	if err := c.Get(ctx, types.NamespacedName{Name: ref.Source, Namespace: namespace}, src); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, allowed, nil, fmt.Errorf("SkillSource %q not found", ref.Source)
		}
		return nil, allowed, nil, fmt.Errorf("SkillSource %q lookup: %w", ref.Source, err)
	}
	if src.Status.Artifact == nil || src.Status.Artifact.ContentPath == "" {
		return nil, allowed, nil, fmt.Errorf("SkillSource %q has no synced artifact yet", ref.Source)
	}

	mountGroup := ref.MountAs
	if mountGroup == "" {
		mountGroup = filepath.Base(src.Status.Artifact.ContentPath)
	}

	syncRoot := filepath.Join(workspaceContentRoot, workspaceName, namespace, src.Status.Artifact.ContentPath)
	resolved, _ := ResolveSkills(syncRoot, nil) // SkillSource already applied its own filter

	includeSet := map[string]struct{}{}
	for _, name := range ref.Include {
		includeSet[name] = struct{}{}
	}

	for _, sk := range resolved {
		if len(includeSet) > 0 {
			if _, ok := includeSet[sk.Name]; !ok {
				continue
			}
		}
		entries = append(entries, SkillManifestEntry{
			MountAs:     filepath.Join(mountGroup, sk.Name),
			ContentPath: filepath.Join(src.Status.Artifact.ContentPath, sk.RelPath),
			Name:        sk.Name,
		})
		if existing, ok := seenNames[sk.Name]; ok && existing != ref.Source {
			collisions = append(collisions, fmt.Errorf(
				"skill name collision: %q appears in both %s and %s", sk.Name, existing, ref.Source))
		}
		seenNames[sk.Name] = ref.Source
		allowed[sk.Name] = sk.AllowedTools
	}
	return entries, allowed, collisions, nil
}

// ValidateSkillTools returns the (skillName:badTool) pairs where a skill's
// allowed-tools names a tool absent from packTools. Empty result = OK.
func ValidateSkillTools(allowedToolsBySkill map[string][]string, packTools map[string]struct{}) []string {
	var bad []string
	for skillName, tools := range allowedToolsBySkill {
		for _, t := range tools {
			if _, ok := packTools[t]; !ok {
				bad = append(bad, fmt.Sprintf("%s:%s", skillName, t))
			}
		}
	}
	sort.Strings(bad)
	return bad
}

// WriteSkillManifest serialises the manifest atomically to
// <root>/manifests/<name>.json. Writes to a temp file and renames.
func WriteSkillManifest(root, name string, manifest *SkillManifest) error {
	dir := filepath.Join(root, "manifests")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir manifests: %w", err)
	}
	target := filepath.Join(dir, name+".json")
	tmp := target + ".tmp"
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write tmp manifest: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("publish manifest: %w", err)
	}
	return nil
}

// ExtractPackTools parses pack JSON and returns the set of declared tool
// names from the top-level "tools" object. Returns an empty set if the pack
// has no tools or the JSON is malformed (callers handle that separately).
func ExtractPackTools(packJSON string) map[string]struct{} {
	var doc struct {
		Tools map[string]any `json:"tools"`
	}
	if err := json.Unmarshal([]byte(packJSON), &doc); err != nil {
		return nil
	}
	out := make(map[string]struct{}, len(doc.Tools))
	for name := range doc.Tools {
		out[name] = struct{}{}
	}
	return out
}

// hashManifest returns a stable SHA256 prefix over the sorted entries + config.
// Same input = same output, which lets PromptPack version bumps detect
// genuine changes in the manifest.
func hashManifest(m *SkillManifest) string {
	h := sha256.New()
	for _, e := range m.Skills {
		_, _ = h.Write([]byte(e.MountAs))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(e.ContentPath))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(e.Name))
		_, _ = h.Write([]byte{0})
	}
	if m.Config != nil {
		_, _ = fmt.Fprintf(h, "ma=%d;sel=%s", m.Config.MaxActive, m.Config.Selector)
	}
	return "v" + hex.EncodeToString(h.Sum(nil))[:12]
}
