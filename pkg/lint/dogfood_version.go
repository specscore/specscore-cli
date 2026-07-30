package lint

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// dogfoodVersionChecker warns when a `.github/workflows/*.yml` file pins
// `SPECSCORE_VERSION` to a release older than the running CLI's own
// version. The scenario it catches: a convention change ships in a new
// CLI release, but CI still installs the old CLI and silently uses the
// old convention — or fails noisily on new artifacts the old CLI does
// not understand.
//
// The check is line-scoped, single-pass, and has no autofix: bumping
// the pin is a deliberate human action per the standard
// `# bump intentionally via PR` convention in dogfood workflows.
type dogfoodVersionChecker struct {
	cliVersion  string // the running CLI's own semver, or "dev"/"" to disable
	projectRoot string
}

func newDogfoodVersionChecker(cliVersion string, projectRoots ...string) checker {
	var projectRoot string
	if len(projectRoots) > 0 {
		projectRoot = projectRoots[0]
	}
	return &dogfoodVersionChecker{cliVersion: cliVersion, projectRoot: projectRoot}
}

func (c *dogfoodVersionChecker) name() string     { return "dogfood-version-bump" }
func (c *dogfoodVersionChecker) severity() string { return "warning" }

// pinPattern matches lines like `  SPECSCORE_VERSION: v0.3.0  # comment`,
// `SPECSCORE_VERSION: "0.3.0"`, etc. Group 1 is the bare semver
// (no `v` prefix, no quotes).
var pinPattern = regexp.MustCompile(`SPECSCORE_VERSION:\s*"?v?(\d+\.\d+\.\d+)"?`)

// parseSemverFn is the injectable semver parser; tests may replace it.
var parseSemverFn = parseSemver

func (c *dogfoodVersionChecker) check(specRoot string) ([]Violation, error) {
	cli, ok := parseSemver(c.cliVersion)
	if !ok {
		// Dev build or unparseable version — rule is silently disabled.
		return nil, nil
	}

	projectRoot := lintProjectRoot(c.projectRoot, specRoot)
	workflowsDir := filepath.Join(projectRoot, ".github", "workflows")

	entries, err := os.ReadDir(workflowsDir)
	if err != nil {
		// No workflows directory — nothing to check.
		return nil, nil
	}

	var violations []Violation
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		path := filepath.Join(workflowsDir, name)
		f, openErr := os.Open(path)
		if openErr != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			// Cheap prefix-check before the regex to keep large workflows
			// fast — the env var name has to appear literally.
			if !strings.Contains(line, "SPECSCORE_VERSION:") {
				continue
			}
			m := pinPattern.FindStringSubmatch(line)
			if m == nil {
				// Present but unparseable (e.g. `${{ inputs.version }}`,
				// `latest`, `main`). Per REQ:dogfood-version-bump-skips-
				// when-pin-unparseable, silently skip.
				continue
			}
			pinned, ok := parseSemverFn(m[1])
			if !ok {
				continue
			}
			if compareSemver(pinned, cli) < 0 {
				rel, _ := filepath.Rel(projectRoot, path)
				violations = append(violations, Violation{
					File:     rel,
					Line:     lineNum,
					Severity: "warning",
					Rule:     "dogfood-version-bump",
					Message: fmt.Sprintf(
						"Pinned SPECSCORE_VERSION v%s is older than the running CLI version v%s; bump the pin to match",
						formatSemver(pinned), formatSemver(cli),
					),
				})
			}
		}
		_ = f.Close()
	}

	return violations, nil
}

// semver is a parsed, comparable semantic version. Only the
// major.minor.patch triple is captured; pre-release and build metadata
// suffixes (e.g. `-rc.1`) are stripped during parsing and ignored for
// comparison, which is sufficient for the bump-detection use case.
type semver struct {
	major, minor, patch int
}

func parseSemver(s string) (semver, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	// Drop pre-release / build suffix if present.
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	var sv semver
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, false
		}
		switch i {
		case 0:
			sv.major = n
		case 1:
			sv.minor = n
		case 2:
			sv.patch = n
		}
	}
	return sv, true
}

// compareSemver returns -1 if a < b, 0 if equal, 1 if a > b.
func compareSemver(a, b semver) int {
	if a.major != b.major {
		if a.major < b.major {
			return -1
		}
		return 1
	}
	if a.minor != b.minor {
		if a.minor < b.minor {
			return -1
		}
		return 1
	}
	if a.patch != b.patch {
		if a.patch < b.patch {
			return -1
		}
		return 1
	}
	return 0
}

func formatSemver(s semver) string {
	return fmt.Sprintf("%d.%d.%d", s.major, s.minor, s.patch)
}
