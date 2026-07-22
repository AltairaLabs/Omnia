/*
Copyright 2026.

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

package k8s

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// envConfigDir points a component at a directory of YAML manifests to serve in
// place of the Kubernetes API — local-dev mode (no cluster). Unset in prod.
const envConfigDir = "OMNIA_CONFIG_DIR"

// newFileBackedClient builds an in-memory client.Client seeded from every
// *.yaml / *.yml manifest in dir. Each manifest's apiVersion/kind must be
// registered in Scheme(). Backed by controller-runtime's fake client — this is
// a local-dev convenience (see NewClient), not a production store.
func newFileBackedClient(dir string) (client.Client, error) {
	// Fail fast on a misconfigured OMNIA_CONFIG_DIR: a missing path or a file
	// would otherwise glob to nothing and silently seed an empty client, which
	// only surfaces later as a confusing "AgentRuntime not found".
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("config dir %q: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("config dir %q is not a directory", dir)
	}

	scheme := Scheme()
	decoder := serializer.NewCodecFactory(scheme).UniversalDeserializer()

	files, err := manifestFiles(dir)
	if err != nil {
		return nil, err
	}

	var objs []client.Object
	for _, f := range files {
		fileObjs, err := decodeManifestFile(f, decoder)
		if err != nil {
			return nil, err
		}
		objs = append(objs, fileObjs...)
	}

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(), nil
}

// manifestFiles returns every *.yaml / *.yml file in dir.
func manifestFiles(dir string) ([]string, error) {
	var files []string
	for _, pattern := range []string{"*.yaml", "*.yml"} {
		matched, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			return nil, fmt.Errorf("glob %s: %w", pattern, err)
		}
		files = append(files, matched...)
	}
	return files, nil
}

// decodeManifestFile decodes every YAML document in a single manifest file into
// client.Objects using the Omnia scheme's decoder.
func decodeManifestFile(path string, decoder runtime.Decoder) ([]client.Object, error) {
	data, err := os.ReadFile(path) //nolint:gosec // operator/dev-supplied devroot
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var objs []client.Object
	reader := utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(data)))
	for {
		doc, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read yaml doc in %s: %w", path, err)
		}
		if isBlankOrCommentOnly(doc) {
			continue
		}
		obj, _, err := decoder.Decode(doc, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("decode manifest in %s: %w", path, err)
		}
		co, ok := obj.(client.Object)
		if !ok {
			return nil, fmt.Errorf("decoded %T in %s is not a client.Object", obj, path)
		}
		objs = append(objs, co)
	}
	return objs, nil
}

// isBlankOrCommentOnly reports whether a YAML document has no content other than
// blank lines and `#` comments — e.g. the leading comment block before the
// first `---` separator. Such documents are skipped rather than decoded.
func isBlankOrCommentOnly(doc []byte) bool {
	for _, line := range bytes.Split(doc, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 || bytes.HasPrefix(trimmed, []byte("#")) {
			continue
		}
		return false
	}
	return true
}
