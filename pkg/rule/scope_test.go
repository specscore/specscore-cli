package rule

import "testing"

func TestParseScope(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    Scope
		wantErr bool
	}{
		{name: "bare fleet", raw: "fleet", want: Scope{Kind: ScopeFleet, Raw: "fleet"}},
		{name: "fleet is trimmed", raw: "  fleet  ", want: Scope{Kind: ScopeFleet, Raw: "fleet"}},
		{name: "product", raw: "product:sneat", want: Scope{Kind: ScopeProduct, Value: "sneat", Raw: "product:sneat"}},
		{name: "product with dots", raw: "product:sneat.money", want: Scope{Kind: ScopeProduct, Value: "sneat.money", Raw: "product:sneat.money"}},
		{name: "repo", raw: "repo:specscore/specscore-cli", want: Scope{Kind: ScopeRepo, Value: "specscore/specscore-cli", Raw: "repo:specscore/specscore-cli"}},
		{name: "path glob", raw: "path:**/*.go", want: Scope{Kind: ScopePath, Value: "**/*.go", Raw: "path:**/*.go"}},

		{name: "empty", raw: "   ", wantErr: true},
		{name: "bare word is never guessed as a product", raw: "sneat", wantErr: true},
		{name: "unknown kind", raw: "team:platform", wantErr: true},
		{name: "missing value", raw: "product:", wantErr: true},
		{name: "fleet takes no value", raw: "fleet:everything", wantErr: true},
		{name: "product uppercase rejected", raw: "product:Sneat", wantErr: true},
		{name: "repo without owner", raw: "repo:specscore-cli", wantErr: true},
		{name: "repo with three segments", raw: "repo:a/b/c", wantErr: true},
		{name: "invalid glob", raw: "path:[unclosed", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseScope(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseScope(%q) = %+v, want error", tc.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseScope(%q): %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("ParseScope(%q) = %+v, want %+v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestScopeString(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"fleet", "fleet"},
		{"product:sneat", "product:sneat"},
		{"repo:owner/name", "repo:owner/name"},
		{"path:pkg/**", "path:pkg/**"},
	}
	for _, tc := range cases {
		s, err := ParseScope(tc.raw)
		if err != nil {
			t.Fatalf("ParseScope(%q): %v", tc.raw, err)
		}
		if got := s.String(); got != tc.want {
			t.Errorf("Scope(%q).String() = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestScopeMatches(t *testing.T) {
	cases := []struct {
		name  string
		scope string
		path  string
		want  bool
	}{
		{name: "fleet matches anything", scope: "fleet", path: "any/path.txt", want: true},
		{name: "fleet matches empty path", scope: "fleet", path: "", want: true},

		{name: "glob matches relative", scope: "path:**/*.go", path: "internal/cli/rule.go", want: true},
		{name: "glob matches absolute via suffix", scope: "path:pkg/**", path: "/home/me/projects/org/repo/pkg/rule/parse.go", want: true},
		{name: "glob rejects other extension", scope: "path:**/*.go", path: "docs/readme.md", want: false},
		{name: "anchored glob matches exact", scope: "path:go.mod", path: "go.mod", want: true},
		{name: "anchored glob rejects nested", scope: "path:go.mod", path: "sub/go.mod", want: true},

		{name: "repo matches owner+repo pair", scope: "repo:specscore/specscore-cli", path: "/home/ai/projects/specscore/specscore-cli/pkg/x.go", want: true},
		{name: "repo matches bare repo segment", scope: "repo:specscore/specscore-cli", path: "specscore-cli/pkg/x.go", want: true},
		{name: "repo rejects unrelated path", scope: "repo:specscore/specscore-cli", path: "other/repo/pkg/x.go", want: false},

		{name: "product matches whole segment", scope: "product:sneat", path: "projects/sneat/app.ts", want: true},
		{name: "product does not match a partial segment", scope: "product:sneat", path: "projects/sneat-co/apps/app.ts", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := ParseScope(tc.scope)
			if err != nil {
				t.Fatalf("ParseScope(%q): %v", tc.scope, err)
			}
			if got := s.Matches(tc.path); got != tc.want {
				t.Fatalf("Scope(%q).Matches(%q) = %v, want %v", tc.scope, tc.path, got, tc.want)
			}
		})
	}
}

func TestScopesMatchAnyScopeWins(t *testing.T) {
	scopes, err := ParseScopes([]string{"path:docs/**", "path:**/*.go"})
	if err != nil {
		t.Fatalf("ParseScopes: %v", err)
	}
	if !ScopesMatch(scopes, "pkg/rule/parse.go") {
		t.Error("ScopesMatch should match on the second scope")
	}
	if ScopesMatch(scopes, "Makefile") {
		t.Error("ScopesMatch should not match a path no scope covers")
	}
}

func TestParseScopesReportsFirstError(t *testing.T) {
	if _, err := ParseScopes([]string{"fleet", "team:platform"}); err == nil {
		t.Fatal("ParseScopes should reject an invalid entry")
	}
}
