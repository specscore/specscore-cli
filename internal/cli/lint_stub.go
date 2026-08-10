package cli

import "github.com/specscore/specscore-cli/pkg/lint"

// lintLintFn is a seam for injecting lint failures in tests. Production
// code calls lintLintFn; tests replace it with a closure that returns a
// canned error, then restore it via t.Cleanup.
var lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
	return lint.Lint(opts)
}

var lessonIndexUpsertFn = lint.UpsertLessonIndexRow

// lintMigrateFn is the seam for the frontmatter migration backfill; tests
// replace it to exercise the migrate verb's error path.
var lintMigrateFn = lint.Migrate
