package cli

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// specFileState is one regular file's bytes and permissions.
type specFileState struct {
	content []byte
	mode    fs.FileMode
}

// readSpecFiles records every regular file under specRoot, so a failed
// `feature change-status` can put back not only the Feature's own Status line
// but every file its `spec lint --fix` pass touched (e.g. a synced index row).
func readSpecFiles(specRoot string) (map[string]specFileState, error) {
	files := map[string]specFileState{}
	err := filepath.WalkDir(specRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		b, err := os.ReadFile(p)
		info, infoErr := d.Info()
		if err = errors.Join(err, infoErr); err != nil {
			return err
		}
		rel, _ := filepath.Rel(specRoot, p)
		files[rel] = specFileState{content: b, mode: info.Mode().Perm()}
		return nil
	})
	return files, err
}

// restoreSpecFiles makes the regular files under specRoot match before:
// files that appeared are removed, changed files are rewritten, and files
// that disappeared are recreated.
func restoreSpecFiles(specRoot string, before map[string]specFileState) error {
	seen := map[string]bool{}
	err := filepath.WalkDir(specRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		rel, _ := filepath.Rel(specRoot, p)
		seen[rel] = true
		orig, existed := before[rel]
		if !existed {
			return os.Remove(p)
		}
		cur, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if bytes.Equal(cur, orig.content) {
			return nil
		}
		return os.WriteFile(p, orig.content, orig.mode)
	})
	if err != nil {
		return err
	}
	for rel, orig := range before {
		if seen[rel] {
			continue
		}
		p := filepath.Join(specRoot, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, orig.content, orig.mode); err != nil {
			return err
		}
	}
	return nil
}
