// Package config resolves SpecScore configuration from layered sources.
//
// Configuration is read from three layers, in decreasing specificity:
//
//	specscore.local.yaml  (repo root, uncommitted)  -- most specific
//	specscore.yaml        (repo root, committed)
//	~/.specscore.yaml     (user home)               -- least specific
//
// The most specific layer wins per key; mapping nodes are deep-merged, while
// scalars and sequences are replaced wholesale. An explicit null in a more
// specific layer clears a value set by a less specific one.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Layer file names.
const (
	ProjectFile = "specscore.yaml"
	LocalFile   = "specscore.local.yaml"
	HomeFile    = ".specscore.yaml"
)

// Resolved is the merged configuration plus the per-key source layer.
type Resolved struct {
	// Values is the deep-merged configuration tree.
	Values map[string]any
	// Origin maps a dotted leaf key path to the layer it resolved from:
	// one of "local", "project", or "home".
	Origin map[string]string
}

// ResolveDir resolves layered config for a repo root and a home directory.
// Missing layer files are treated as empty layers (not errors); a malformed
// layer file is a hard error.
func ResolveDir(repoRoot, homeDir string) (Resolved, error) {
	// Least specific first so more specific layers overwrite.
	layers := []struct{ name, path string }{
		{"home", filepath.Join(homeDir, HomeFile)},
		{"project", filepath.Join(repoRoot, ProjectFile)},
		{"local", filepath.Join(repoRoot, LocalFile)},
	}

	values := map[string]any{}
	origin := map[string]string{}
	for _, l := range layers {
		m, err := loadLayer(l.path)
		if err != nil {
			return Resolved{}, err
		}
		if m == nil {
			continue
		}
		mergeInto(values, origin, m, l.name, "")
	}
	return Resolved{Values: values, Origin: origin}, nil
}

// loadLayer reads and parses one layer file. A missing file yields (nil, nil);
// an empty/comment-only file yields (nil, nil); a malformed file yields an error.
func loadLayer(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return m, nil
}

// mergeInto merges src into dst, recording origins keyed by dotted path.
func mergeInto(dst map[string]any, origin map[string]string, src map[string]any, layer, prefix string) {
	for k, v := range src {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}

		if v == nil {
			// Explicit null clears a value from a less specific layer.
			delete(dst, k)
			clearOrigins(origin, path)
			continue
		}

		if srcMap, ok := v.(map[string]any); ok {
			dstMap, ok := dst[k].(map[string]any)
			if !ok {
				// Replacing a scalar (or absent) with a map: drop stale origin.
				dstMap = map[string]any{}
				dst[k] = dstMap
				delete(origin, path)
			}
			mergeInto(dstMap, origin, srcMap, layer, path)
			continue
		}

		// Scalar or sequence: replace wholesale.
		dst[k] = v
		clearOrigins(origin, path)
		origin[path] = layer
	}
}

// clearOrigins removes the origin for path and any origins nested under it.
func clearOrigins(origin map[string]string, path string) {
	delete(origin, path)
	nested := path + "."
	for key := range origin {
		if len(key) > len(nested) && key[:len(nested)] == nested {
			delete(origin, key)
		}
	}
}
