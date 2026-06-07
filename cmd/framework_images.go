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

package main

import (
	"fmt"
	"strings"
)

// frameworkImagesFlag is a repeatable --framework-image flag. Each value is
// "type=repo:tag"; a value with no "=" is the legacy bare form and maps to the
// "promptkit" framework for back-compat. Split is on the FIRST "=" so the
// "repo:tag" colon is preserved.
type frameworkImagesFlag struct {
	m map[string]string
}

const promptkitFrameworkKey = "promptkit"

func (f *frameworkImagesFlag) String() string {
	if f == nil || len(f.m) == 0 {
		return ""
	}
	parts := make([]string, 0, len(f.m))
	for k, v := range f.m {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

func (f *frameworkImagesFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("framework-image value is empty")
	}
	if f.m == nil {
		f.m = map[string]string{}
	}
	key := promptkitFrameworkKey
	img := value
	if i := strings.Index(value, "="); i >= 0 {
		key = strings.TrimSpace(value[:i])
		img = strings.TrimSpace(value[i+1:])
		if key == "" {
			key = promptkitFrameworkKey
		}
	}
	if img == "" {
		return fmt.Errorf("framework-image %q has no image", value)
	}
	f.m[key] = img
	return nil
}

// images returns the accumulated type->image map (never nil).
func (f *frameworkImagesFlag) images() map[string]string {
	if f.m == nil {
		return map[string]string{}
	}
	return f.m
}
